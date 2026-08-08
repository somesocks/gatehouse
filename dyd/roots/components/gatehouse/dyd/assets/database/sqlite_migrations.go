package database

import "gatehouse/config"

func sqliteMigrations(state config.State) (error, Registry) {
	registry := Registry{
		Init: InitMigration{SQL: `
		CREATE TABLE IF NOT EXISTS gatehouse_schema_migrations (
			installed_rank INTEGER PRIMARY KEY AUTOINCREMENT,
			migration_type TEXT NOT NULL
				CHECK (migration_type IN ('versioned', 'repeatable', 'baseline')),
			migration_index INTEGER NOT NULL
				CHECK (migration_index > 0),
			description TEXT NOT NULL
				CHECK (length(description) > 0),
			checksum BLOB NOT NULL
				CHECK (length(checksum) = 32),
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		) STRICT;

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
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;
		`,
		}, {
			Index:       2,
			Description: "create_principals_and_identities",
			SQL: `
			CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;

			CREATE TABLE gatehouse_identities (
				id TEXT PRIMARY KEY CHECK (instr(id, ':') > 1),
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				verifiers TEXT NOT NULL
					CHECK (json_valid(verifiers))
					CHECK (json_type(verifiers) = 'array')
					CHECK (json_array_length(verifiers) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;

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
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, id)
			) STRICT;

			CREATE TABLE gatehouse_group_members (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, group_id, principal_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;

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
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				ref TEXT NOT NULL CHECK (length(trim(ref)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, id)
			) STRICT;

			CREATE TABLE gatehouse_resources (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				ref TEXT NOT NULL CHECK (length(trim(ref)) > 0),
				secret INTEGER NOT NULL CHECK (secret IN (0, 1)),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, id)
			) STRICT;

			CREATE TABLE gatehouse_group_tool_grants (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				tool_id TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, group_id, tool_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace_id, tool_id)
					REFERENCES gatehouse_tools (workspace_id, id)
			) STRICT;

			CREATE TABLE gatehouse_group_resource_grants (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				resource_id TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, group_id, resource_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace_id, resource_id)
					REFERENCES gatehouse_resources (workspace_id, id)
			) STRICT;
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
			Description: "reconcile_principals_and_identities",
			Template: `
			SELECT 1;
			{{ range .Principals }}
			{{ $principal := . }}
			INSERT INTO gatehouse_principals (id, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ range .Identities }}
			INSERT INTO gatehouse_identities (id, principal_id, verifiers, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral $principal.ID }}, {{ sqlLiteral .Verifiers }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled;
			{{ end }}
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
			INSERT INTO gatehouse_tools (workspace_id, id, ref, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ID }}, {{ sqlLiteral .Ref }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, id) DO UPDATE SET
				ref = excluded.ref,
				enabled = excluded.enabled;
			{{ end }}
			{{ range .Resources }}
			INSERT INTO gatehouse_resources (workspace_id, id, ref, secret, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ID }}, {{ sqlLiteral .Ref }}, {{ sqlBool .Secret }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, id) DO UPDATE SET
				ref = excluded.ref,
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
