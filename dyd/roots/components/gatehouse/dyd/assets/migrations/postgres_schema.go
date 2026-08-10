package migrations

import "gatehouse/config"

func postgresMigrations(state config.State) (error, Registry) {
	registry := Registry{
		Init: InitMigration{SQL: `
		CREATE TABLE IF NOT EXISTS gatehouse_schema_migrations (
			installed_rank BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			migration_type TEXT NOT NULL
				CHECK (migration_type IN ('versioned', 'repeatable', 'baseline')),
			migration_index BIGINT NOT NULL
				CHECK (migration_index > 0),
			description TEXT NOT NULL
				CHECK (length(description) > 0),
			checksum BYTEA NOT NULL
				CHECK (octet_length(checksum) = 32),
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE UNIQUE INDEX IF NOT EXISTS gatehouse_schema_migrations_once
		ON gatehouse_schema_migrations (migration_type, migration_index)
		WHERE migration_type IN ('versioned', 'baseline');

		CREATE INDEX IF NOT EXISTS gatehouse_schema_migrations_repeatable_latest
		ON gatehouse_schema_migrations (migration_type, migration_index, installed_rank DESC)
		WHERE migration_type = 'repeatable';
	`},
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_workspaces",
			SQL: `
			CREATE TABLE gatehouse_workspaces (
				id TEXT PRIMARY KEY
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);
		`,
		}, {
			Index:       2,
			Description: "create_principals_and_identities",
			SQL: `
			CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);

			CREATE TABLE gatehouse_identities (
				id TEXT PRIMARY KEY CHECK (position(':' IN id) > 1),
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				verifiers JSONB NOT NULL
					CHECK (jsonb_typeof(verifiers) = 'array')
					CHECK (jsonb_array_length(verifiers) > 0),
				enabled BOOLEAN NOT NULL
			);

			CREATE INDEX gatehouse_identities_by_principal
			ON gatehouse_identities (principal_id);
		`,
		}, {
			Index:       3,
			Description: "create_groups_and_memberships",
			SQL: `
			CREATE TABLE gatehouse_groups (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id)
			);

			CREATE TABLE gatehouse_group_members (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, group_id, principal_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id)
			);

			CREATE INDEX gatehouse_group_members_by_principal
			ON gatehouse_group_members (principal_id);
		`,
		}, {
			Index:       4,
			Description: "create_tools_resources_and_group_grants",
			SQL: `
			CREATE TABLE gatehouse_tools (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id)
			);

			CREATE TABLE gatehouse_resources (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				secret BOOLEAN NOT NULL,
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id)
			);

			CREATE TABLE gatehouse_group_tool_grants (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				tool_id TEXT NOT NULL,
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, group_id, tool_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace_id, tool_id)
					REFERENCES gatehouse_tools (workspace_id, id)
			);

			CREATE TABLE gatehouse_group_resource_grants (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				resource_id TEXT NOT NULL,
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, group_id, resource_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace_id, resource_id)
					REFERENCES gatehouse_resources (workspace_id, id)
			);
		`,
		}, {
			Index:       5,
			Description: "create_keychains",
			SQL: `
			CREATE TABLE gatehouse_keychains (
				id TEXT NOT NULL
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				version BIGINT NOT NULL CHECK (version > 0),
				kek_kdf TEXT NOT NULL CHECK (length(trim(kek_kdf)) > 0),
				key TEXT NOT NULL CHECK (length(trim(key)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (id, version)
			);
		`,
		}, {
			Index:       6,
			Description: "add_identity_revisions",
			SQL: `
			ALTER TABLE gatehouse_identities
			ADD COLUMN revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0);
		`,
		}, {
			Index:       7,
			Description: "create_sessions_and_grants",
			SQL: `
			CREATE TABLE gatehouse_sessions (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL,
				created_by TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace, id)
			);

			CREATE INDEX gatehouse_sessions_by_workspace
			ON gatehouse_sessions (workspace, created_at);

			CREATE TABLE gatehouse_session_principal_grants (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace, session, principal),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id)
			);

			CREATE INDEX gatehouse_session_principal_grants_by_principal
			ON gatehouse_session_principal_grants (principal, workspace, session);

			CREATE TABLE gatehouse_session_group_grants (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				"group" TEXT NOT NULL,
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace, session, "group"),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, "group")
					REFERENCES gatehouse_groups (workspace_id, id)
			);

			CREATE INDEX gatehouse_session_group_grants_by_group
			ON gatehouse_session_group_grants (workspace, "group", session);
		`,
		}},
		Repeatable: []RepeatableMigration{{
			Index:       1,
			Description: "seed_gatehouse_workspace",
			Template: `
			INSERT INTO gatehouse_workspaces (id, name, enabled)
			VALUES ('gatehouse', 'Gatehouse', TRUE)
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
		`,
		}, {
			Index:       2,
			Description: "reconcile_workspaces",
			Template: `
			SELECT 1;
			{{ range .Workspaces }}
			INSERT INTO gatehouse_workspaces (id, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`,
		}, {
			Index:       3,
			Description: "reconcile_principals",
			Template: `
			SELECT 1;
			{{ range .Principals }}
			INSERT INTO gatehouse_principals (id, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`,
		}, {
			Index:       4,
			Description: "reconcile_groups_and_memberships",
			Template: `
			SELECT 1;
			{{ range .Groups }}
			{{ $group := . }}
			INSERT INTO gatehouse_groups (workspace_id, id, name, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ID }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ range .Members }}
			INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled)
			VALUES ({{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.ID }}, {{ sqlLiteral .PrincipalID }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET
				enabled = excluded.enabled;
			{{ end }}
			{{ end }}
		`,
		}, {
			Index:       5,
			Description: "reconcile_tools_and_resources",
			Template: `
			SELECT 1;
			{{ range .Tools }}
			INSERT INTO gatehouse_tools (workspace_id, id, source, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ID }}, {{ sqlLiteral .Source }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, id) DO UPDATE SET
				source = excluded.source,
				enabled = excluded.enabled;
			{{ end }}
			{{ range .Resources }}
			INSERT INTO gatehouse_resources (workspace_id, id, source, secret, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ID }}, {{ sqlLiteral .Source }}, {{ sqlBool .Secret }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, id) DO UPDATE SET
				source = excluded.source,
				secret = excluded.secret,
				enabled = excluded.enabled;
			{{ end }}
		`,
		}, {
			Index:       6,
			Description: "reconcile_group_grants",
			Template: `
			SELECT 1;
			{{ range .Groups }}
			{{ $group := . }}
			{{ range .ToolGrants }}
			INSERT INTO gatehouse_group_tool_grants (workspace_id, group_id, tool_id, enabled)
			VALUES ({{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.ID }}, {{ sqlLiteral .ToolID }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, group_id, tool_id) DO UPDATE SET
				enabled = excluded.enabled;
			{{ end }}
			{{ range .ResourceGrants }}
			INSERT INTO gatehouse_group_resource_grants (workspace_id, group_id, resource_id, enabled)
			VALUES ({{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.ID }}, {{ sqlLiteral .ResourceID }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, group_id, resource_id) DO UPDATE SET
				enabled = excluded.enabled;
			{{ end }}
			{{ end }}
		`,
		}},
	}
	return materializeRegistry(registry, migrationValuesFor(state))
}
