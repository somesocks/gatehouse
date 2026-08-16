package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func postgresMigrations(state config.State, keyring *keychain.Keyring) (error, Registry) {
	values := migrationValuesFor(state)
	gatehouseName := "Gatehouse"
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
				id TEXT PRIMARY KEY CHECK (id ~ '^wsp_[a-z2-7]{26}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);
		`),
		}, {
			Index:       2,
			Description: "create_principals_and_identities",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY CHECK (id ~ '^prn_[a-z2-7]{26}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);

			CREATE TABLE gatehouse_identities (
				id TEXT PRIMARY KEY CHECK (id ~ '^idt_[a-z2-7]{26}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				key TEXT NOT NULL UNIQUE CHECK (position(':' IN key) > 1),
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
				id TEXT NOT NULL CHECK (id ~ '^grp_[a-z2-7]{26}$'),
				alias TEXT CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, alias)
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
				id TEXT NOT NULL CHECK (id ~ '^tol_[a-z2-7]{26}$'),
				alias TEXT CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, alias)
			);

			CREATE TABLE gatehouse_resources (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (id ~ '^res_[a-z2-7]{26}$'),
				alias TEXT CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				secret BOOLEAN NOT NULL,
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, alias)
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
				id TEXT NOT NULL CHECK (id ~ '^ses_[a-z2-7]{26}$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
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
				id TEXT NOT NULL CHECK (id ~ '^sev_[a-z2-7]{26}$'),
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
				id TEXT PRIMARY KEY CHECK (id ~ '^apr_[a-z2-7]{26}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				revision BIGINT NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('builtin', 'openai-chat-completions', 'openai-responses')),
				base_url TEXT CHECK (base_url IS NULL OR length(trim(base_url)) > 0),
				keychain_id TEXT,
				keychain_version BIGINT CHECK (keychain_version IS NULL OR keychain_version > 0),
				api_key TEXT CHECK (api_key IS NULL OR length(trim(api_key)) > 0),
				enabled BOOLEAN NOT NULL,
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'builtin' AND base_url IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND api_key IS NULL)
					OR (protocol IN ('openai-chat-completions', 'openai-responses') AND base_url IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND api_key IS NOT NULL)
				)
			);

			CREATE TABLE gatehouse_agent_models (
				id TEXT PRIMARY KEY CHECK (id ~ '^amd_[a-z2-7]{26}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
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

			CREATE TABLE gatehouse_agent_tasks__session_name (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, session),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id)
			);
		`),
		}, {
			Index:       10,
			Description: "add_workspace_agent_max_turns",
			Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN max_turns BIGINT NOT NULL DEFAULT 127 CHECK (max_turns > 0);
		`),
		}, {
			Index:       11,
			Description: "add_workspace_agent_system_prompt",
			Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN system_prompt TEXT;
		`),
		}, {
			Index:       12,
			Description: "add_workspace_agent_label",
			Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN label TEXT;
		`),
		}, {
			Index:       13,
			Description: "create_storage_providers",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_storage_providers (
				id TEXT PRIMARY KEY CHECK (id ~ '^stp_[a-z2-7]{26}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				revision BIGINT NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('embedded', 's3')),
				endpoint TEXT,
				region TEXT,
				bucket TEXT,
				access_key_id TEXT,
				keychain_id TEXT,
				keychain_version BIGINT,
				secret_access_key TEXT,
				enabled BOOLEAN NOT NULL,
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'embedded' AND endpoint IS NULL AND region IS NULL AND bucket IS NULL AND access_key_id IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND secret_access_key IS NULL)
					OR (protocol = 's3' AND endpoint IS NOT NULL AND length(trim(endpoint)) > 0 AND region IS NOT NULL AND length(trim(region)) > 0 AND bucket IS NOT NULL AND length(trim(bucket)) > 0 AND access_key_id IS NOT NULL AND length(trim(access_key_id)) > 0 AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND keychain_version > 0 AND secret_access_key IS NOT NULL AND length(trim(secret_access_key)) > 0)
				)
			);

			CREATE TABLE gatehouse_workspace_storage_providers (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				priority BIGINT NOT NULL CHECK (priority > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace, provider)
			);
		`),
		}, {
			Index:       14,
			Description: "create_storage_objects_and_session_files",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_storage_objects (
				id TEXT PRIMARY KEY,
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				object TEXT NOT NULL CHECK (length(trim(object)) > 0),
				state TEXT NOT NULL CHECK (state IN ('pending', 'success', 'failure')),
				sha256 BYTEA CHECK (sha256 IS NULL OR octet_length(sha256) = 32),
				size BIGINT CHECK (size IS NULL OR size >= 0),
				created_at TIMESTAMPTZ NOT NULL,
				CHECK ((sha256 IS NULL AND size IS NULL) OR (sha256 IS NOT NULL AND size IS NOT NULL))
			);

			CREATE TABLE gatehouse_embedded_storage_objects (
				id TEXT PRIMARY KEY
			);

			CREATE TABLE gatehouse_embedded_storage_object_chunks (
				embedded_storage_object TEXT NOT NULL REFERENCES gatehouse_embedded_storage_objects (id),
				ordinal BIGINT NOT NULL CHECK (ordinal >= 0),
				sha256 BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
				size BIGINT NOT NULL CHECK (size > 0 AND size <= 262144),
				bytes BYTEA NOT NULL CHECK (octet_length(bytes) = size),
				PRIMARY KEY (embedded_storage_object, ordinal)
			);

			CREATE TABLE gatehouse_session_files (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL,
				storage_object TEXT NOT NULL UNIQUE REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			);
		`),
		}, {
			Index:       15,
			Description: "create_activity_events",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_activity_events (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (id ~ '^act_[a-z2-7]{26}$'),
				event TEXT NOT NULL CHECK (length(trim(event)) > 0),
				resource_kind TEXT NOT NULL CHECK (resource_kind IN ('session', 'session_event')),
				session TEXT,
				session_event TEXT,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, id),
				UNIQUE (workspace, id, created_at),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, session, session_event)
					REFERENCES gatehouse_session_events (workspace, session, id),
				CHECK (
					(resource_kind = 'session' AND session IS NOT NULL AND session_event IS NULL)
					OR (resource_kind = 'session_event' AND session IS NOT NULL AND session_event IS NOT NULL)
				)
			);

			CREATE TABLE gatehouse_activity_event_topics (
				workspace TEXT NOT NULL,
				activity TEXT NOT NULL,
				topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, activity, topic),
				FOREIGN KEY (workspace, activity, created_at)
					REFERENCES gatehouse_activity_events (workspace, id, created_at)
			);

			CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor
			ON gatehouse_activity_event_topics (workspace, topic, created_at, activity);
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
				Builder: workspaceMigrationBuilder([]config.Workspace{{Alias: "gatehouse", Name: &gatehouseName, Enabled: true}}),
			}, {
				Index:       3,
				Description: "reconcile_workspaces",
				Builder: workspaceMigrationBuilder(state.Workspaces),
			}, {
				Index:       4,
				Description: "reconcile_principals",
				Builder:     principalMigrationBuilder(state.Principals),
			}, {
				Index:       5,
				Description: "reconcile_identities",
				Builder:     identityMigrationBuilder(state.Principals),
			}, {
				Index:       6,
				Description: "reconcile_groups_and_memberships",
				Builder:     groupMigrationBuilder(state.Groups),
			}, {
				Index:       7,
				Description: "reconcile_tools_and_resources",
				Builder:     toolsResourcesMigrationBuilder(state.Tools, state.Resources),
			}, {
				Index:       8,
				Description: "reconcile_group_grants",
				Builder: templateMigrationBuilder(`
			SELECT 1;
			{{ range .Groups }}
			{{ $group := . }}
			{{ range .ToolGrants }}
			INSERT INTO gatehouse_group_tool_grants (workspace_id, group_id, tool_id, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral $group.WorkspaceID }}), (SELECT groups.id FROM gatehouse_groups AS groups JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id WHERE workspaces.alias = {{ sqlLiteral $group.WorkspaceID }} AND groups.alias = {{ sqlLiteral $group.Alias }}), (SELECT tools.id FROM gatehouse_tools AS tools JOIN gatehouse_workspaces AS workspaces ON workspaces.id = tools.workspace_id WHERE workspaces.alias = {{ sqlLiteral $group.WorkspaceID }} AND tools.alias = {{ sqlLiteral .ToolAlias }}), {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, group_id, tool_id) DO UPDATE SET
				enabled = excluded.enabled;
			{{ end }}
			{{ range .ResourceGrants }}
			INSERT INTO gatehouse_group_resource_grants (workspace_id, group_id, resource_id, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral $group.WorkspaceID }}), (SELECT groups.id FROM gatehouse_groups AS groups JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id WHERE workspaces.alias = {{ sqlLiteral $group.WorkspaceID }} AND groups.alias = {{ sqlLiteral $group.Alias }}), (SELECT resources.id FROM gatehouse_resources AS resources JOIN gatehouse_workspaces AS workspaces ON workspaces.id = resources.workspace_id WHERE workspaces.alias = {{ sqlLiteral $group.WorkspaceID }} AND resources.alias = {{ sqlLiteral .ResourceAlias }}), {{ sqlBool .Enabled }})
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
			}, {
				Index:       12,
				Description: "reconcile_storage_providers",
				Builder:     storageProviderMigrationBuilder(state.StorageProviders, keyring),
			}, {
				Index:       13,
				Description: "reconcile_workspace_storage_providers",
				Builder:     workspaceStorageProviderMigrationBuilder(state.WorkspaceStorageProviders),
			}},
	}
	return nil, registry
}
