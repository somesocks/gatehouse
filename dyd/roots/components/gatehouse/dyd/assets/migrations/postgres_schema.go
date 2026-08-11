package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func postgresMigrations(state config.State, keyring *keychain.Keyring) (error, Registry) {
	values := migrationValuesFor(state)
	registry := Registry{
		Init: InitMigration{Builder: staticMigrationBuilder(`
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
			applied_at TIMESTAMPTZ NOT NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS gatehouse_schema_migrations_once
		ON gatehouse_schema_migrations (migration_type, migration_index)
		WHERE migration_type IN ('versioned', 'baseline');

		CREATE INDEX IF NOT EXISTS gatehouse_schema_migrations_repeatable_latest
		ON gatehouse_schema_migrations (migration_type, migration_index, installed_rank DESC)
		WHERE migration_type = 'repeatable';
	`)},
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_workspaces",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_workspaces (
				id TEXT PRIMARY KEY
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);
		`),
		}, {
			Index:       2,
			Description: "create_principals_and_identities",
			Builder: staticMigrationBuilder(`
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
		`),
		}, {
			Index:       3,
			Description: "create_groups_and_memberships",
			Builder: staticMigrationBuilder(`
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
		`),
		}, {
			Index:       4,
			Description: "create_tools_resources_and_group_grants",
			Builder: staticMigrationBuilder(`
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
		`),
		}, {
			Index:       5,
			Description: "create_keychains",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_keychains (
				id TEXT NOT NULL
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				version BIGINT NOT NULL CHECK (version > 0),
				kek_kdf TEXT NOT NULL CHECK (length(trim(kek_kdf)) > 0),
				key TEXT NOT NULL CHECK (length(trim(key)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (id, version)
			);
		`),
		}, {
			Index:       6,
			Description: "add_identity_revisions",
			Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_identities
			ADD COLUMN revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0);
		`),
		}, {
			Index:       8,
			Description: "create_sessions_grants_and_events",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_sessions (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL,
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT,
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, id),
				FOREIGN KEY (workspace, author_agent)
					REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				CHECK (
					(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
				)
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

			CREATE TABLE gatehouse_session_events (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(trim(id)) > 0),
				parent TEXT,
				kind TEXT NOT NULL CHECK (length(trim(kind)) > 0),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT,
				payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, session, parent)
					REFERENCES gatehouse_session_events (workspace, session, id),
				FOREIGN KEY (workspace, author_agent)
					REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				CHECK (
					(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
				),
				CHECK (parent IS NULL OR parent <> id)
			);

			CREATE INDEX gatehouse_session_events_by_session_order
			ON gatehouse_session_events (workspace, session, created_at, id);

			CREATE INDEX gatehouse_session_events_roots
			ON gatehouse_session_events (workspace, session, created_at, id)
			WHERE parent IS NULL;

			CREATE INDEX gatehouse_session_events_children
			ON gatehouse_session_events (workspace, session, parent, created_at, id);
		`),
		}, {
			Index:       7,
			Description: "create_agents",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_providers (
				id TEXT PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				revision BIGINT NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('builtin', 'openai-compatible')),
				base_url TEXT CHECK (base_url IS NULL OR length(trim(base_url)) > 0),
				keychain_id TEXT,
				keychain_version BIGINT CHECK (keychain_version IS NULL OR keychain_version > 0),
				api_key TEXT CHECK (api_key IS NULL OR length(trim(api_key)) > 0),
				enabled BOOLEAN NOT NULL,
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'builtin' AND base_url IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND api_key IS NULL)
					OR (protocol = 'openai-compatible' AND base_url IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND api_key IS NOT NULL)
				)
			);

			CREATE TABLE gatehouse_agent_models (
				id TEXT PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				revision BIGINT NOT NULL CHECK (revision > 0),
				provider_id TEXT NOT NULL REFERENCES gatehouse_agent_providers (id),
				model TEXT NOT NULL CHECK (length(trim(model)) > 0),
				parameters JSONB NOT NULL CHECK (jsonb_typeof(parameters) = 'object'),
				enabled BOOLEAN NOT NULL
			);

			CREATE TABLE gatehouse_workspace_agents (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				model_id TEXT NOT NULL REFERENCES gatehouse_agent_models (id),
				priority BIGINT NOT NULL CHECK (priority > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, model_id)
			);
		`),
		}, {
			Index:       9,
			Description: "create_session_event_reply_tasks",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_tasks__session_event_reply (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				event TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, session, event),
				FOREIGN KEY (workspace, session, event)
					REFERENCES gatehouse_session_events (workspace, session, id)
			);
		`),
		}},
		Repeatable: []RepeatableMigration{
			{
				Index:       1,
				Description: "prepare_keychains",
				Builder:     keychainMigrationBuilder(keyring),
			},
			{
				Index:       2,
				Description: "seed_gatehouse_workspace",
				Builder: templateMigrationBuilder(`
			INSERT INTO gatehouse_workspaces (id, name, enabled)
			VALUES ('gatehouse', 'Gatehouse', TRUE)
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
		`, values),
			}, {
				Index:       3,
				Description: "reconcile_workspaces",
				Builder: templateMigrationBuilder(`
			SELECT 1;
			{{ range .Workspaces }}
			INSERT INTO gatehouse_workspaces (id, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`, values),
			}, {
				Index:       4,
				Description: "reconcile_principals",
				Builder: templateMigrationBuilder(`
			SELECT 1;
			{{ range .Principals }}
			INSERT INTO gatehouse_principals (id, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`, values),
			}, {
				Index:       5,
				Description: "reconcile_identities",
				Builder:     identityMigrationBuilder(state.Principals),
			}, {
				Index:       6,
				Description: "reconcile_groups_and_memberships",
				Builder: templateMigrationBuilder(`
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
		`, values),
			}, {
				Index:       7,
				Description: "reconcile_tools_and_resources",
				Builder: templateMigrationBuilder(`
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
		`, values),
			}, {
				Index:       8,
				Description: "reconcile_group_grants",
				Builder: templateMigrationBuilder(`
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
		`, values),
			}, {
				Index:       9,
				Description: "reconcile_agent_providers",
				Builder:     agentProviderMigrationBuilder(state.AgentProviders, keyring),
			}, {
				Index:       10,
				Description: "reconcile_agent_models",
				Builder:     agentModelMigrationBuilder(state.AgentModels),
			}, {
				Index:       11,
				Description: "reconcile_workspace_agents",
				Builder:     workspaceAgentMigrationBuilder(state.WorkspaceAgents),
			}},
	}
	return nil, registry
}
