package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func sqliteMigrations(state config.State, keyring *keychain.Keyring) (error, Registry) {
	values := migrationValuesFor(state)
	registry := Registry{
		Init: InitMigration{Builder: staticMigrationBuilder(`
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
	`)},
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_workspaces",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_workspaces (
				id TEXT PRIMARY KEY
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;
		`),
		}, {
			Index:       2,
			Description: "create_principals_and_identities",
			Builder: staticMigrationBuilder(`
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
		`),
		}, {
			Index:       3,
			Description: "create_groups_and_memberships",
			Builder: staticMigrationBuilder(`
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
		`),
		}, {
			Index:       4,
			Description: "create_tools_resources_and_group_grants",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_tools (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, id)
			) STRICT;

			CREATE TABLE gatehouse_resources (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
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
		`),
		}, {
			Index:       5,
			Description: "create_keychains",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_keychains (
				id TEXT NOT NULL
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				version INTEGER NOT NULL CHECK (version > 0),
				kek_kdf TEXT NOT NULL CHECK (length(trim(kek_kdf)) > 0),
				key TEXT NOT NULL CHECK (length(trim(key)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (id, version)
			) STRICT;
		`),
		}, {
			Index:       6,
			Description: "add_identity_revisions",
			Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_identities
			ADD COLUMN revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0);
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
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace, id),
				FOREIGN KEY (workspace, author_agent)
					REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				CHECK (
					(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
				)
			) STRICT;

			CREATE INDEX gatehouse_sessions_by_workspace
			ON gatehouse_sessions (workspace, created_at);

			CREATE TABLE gatehouse_session_principal_grants (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace, session, principal),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;

			CREATE INDEX gatehouse_session_principal_grants_by_principal
			ON gatehouse_session_principal_grants (principal, workspace, session);

			CREATE TABLE gatehouse_session_group_grants (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				"group" TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace, session, "group"),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, "group")
					REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;

			CREATE INDEX gatehouse_session_group_grants_by_group
			ON gatehouse_session_group_grants (workspace, "group", session);

			CREATE TABLE gatehouse_session_events (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(trim(id)) > 0),
				kind TEXT NOT NULL CHECK (length(trim(kind)) > 0),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT,
				payload TEXT NOT NULL CHECK (json_valid(payload)) CHECK (json_type(payload) = 'object'),
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, author_agent)
					REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				CHECK (
					(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
					OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
				)
			) STRICT;
		`),
		}, {
			Index:       7,
			Description: "create_agents",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_providers (
				id TEXT PRIMARY KEY CHECK (id GLOB '[a-z]*') CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				revision INTEGER NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol = 'openai-compatible'),
				base_url TEXT NOT NULL CHECK (length(trim(base_url)) > 0),
				keychain_id TEXT NOT NULL,
				keychain_version INTEGER NOT NULL CHECK (keychain_version > 0),
				api_key TEXT NOT NULL CHECK (length(trim(api_key)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version)
			) STRICT;

			CREATE TABLE gatehouse_agent_models (
				id TEXT PRIMARY KEY CHECK (id GLOB '[a-z]*') CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				revision INTEGER NOT NULL CHECK (revision > 0),
				provider_id TEXT NOT NULL REFERENCES gatehouse_agent_providers (id),
				model TEXT NOT NULL CHECK (length(trim(model)) > 0),
				parameters TEXT NOT NULL CHECK (json_valid(parameters)) CHECK (json_type(parameters) = 'object'),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;

			CREATE TABLE gatehouse_workspace_agents (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				model_id TEXT NOT NULL REFERENCES gatehouse_agent_models (id),
				priority INTEGER NOT NULL CHECK (priority > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, model_id)
			) STRICT;
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
