package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func sqliteMigrations(state config.State, keyring *keychain.Keyring) (error, Registry) {
	gatehouseName := "Gatehouse"
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
			applied_at TEXT NOT NULL
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
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wsp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias GLOB '[a-z]*')
					CHECK (alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;
		`),
		}, {
			Index:       2,
			Description: "create_principals_and_identities",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prn_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;

			CREATE TABLE gatehouse_identities (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'idt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				key TEXT NOT NULL UNIQUE CHECK (instr(key, ':') > 1),
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
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'grp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, alias)
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
				project TEXT,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ses_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, id),
				FOREIGN KEY (workspace, author_agent)
					REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				FOREIGN KEY (workspace, project)
					REFERENCES gatehouse_projects (workspace, id),
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
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sev_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				parent TEXT,
				kind TEXT NOT NULL CHECK (length(trim(kind)) > 0),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				payload TEXT NOT NULL CHECK (json_valid(payload)) CHECK (json_type(payload) = 'object'),
				created_at TEXT NOT NULL,
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
			) STRICT;

			CREATE INDEX gatehouse_session_events_by_session_order
			ON gatehouse_session_events (workspace, session, id);

			CREATE INDEX gatehouse_session_events_roots
			ON gatehouse_session_events (workspace, session, id)
			WHERE parent IS NULL;

			CREATE INDEX gatehouse_session_events_children
			ON gatehouse_session_events (workspace, session, parent, id);
		`),
		}, {
			Index:       7,
			Description: "create_agents_and_projects",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_providers (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'apr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				revision INTEGER NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('builtin', 'openai-chat-completions', 'openai-responses')),
				base_url TEXT CHECK (base_url IS NULL OR length(trim(base_url)) > 0),
				keychain_id TEXT,
				keychain_version INTEGER CHECK (keychain_version IS NULL OR keychain_version > 0),
				api_key TEXT CHECK (api_key IS NULL OR length(trim(api_key)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'builtin' AND base_url IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND api_key IS NULL)
					OR (protocol IN ('openai-chat-completions', 'openai-responses') AND base_url IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND api_key IS NOT NULL)
				)
			) STRICT;

			CREATE TABLE gatehouse_agent_models (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'amd_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
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

			CREATE TABLE gatehouse_projects (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prj_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				description TEXT CHECK (description IS NULL OR length(trim(description)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, id)
			) STRICT;

			CREATE TABLE gatehouse_project_notes (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pnt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, id),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			) STRICT;

			CREATE INDEX gatehouse_project_notes_by_project_created
			ON gatehouse_project_notes (workspace, project, created_at DESC, id DESC);

			CREATE TABLE gatehouse_project_principal_grants (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace, project, principal),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			) STRICT;

			CREATE TABLE gatehouse_project_group_grants (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				"group" TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace, project, "group"),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
		`),
		}, {
			Index:       9,
			Description: "create_session_event_reply_tasks",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_tasks__session_event_reply (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				event TEXT NOT NULL,
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, event),
				FOREIGN KEY (workspace, session, event)
					REFERENCES gatehouse_session_events (workspace, session, id)
			) STRICT;

			CREATE TABLE gatehouse_agent_tasks__session_name (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;
		`),
		}, {
			Index:       10,
			Description: "add_workspace_agent_max_turns",
			Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN max_turns INTEGER NOT NULL DEFAULT 127 CHECK (max_turns > 0);
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
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'stp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				revision INTEGER NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('embedded', 's3')),
				endpoint TEXT,
				region TEXT,
				bucket TEXT,
				access_key_id TEXT,
				keychain_id TEXT,
				keychain_version INTEGER,
				secret_access_key TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'embedded' AND endpoint IS NULL AND region IS NULL AND bucket IS NULL AND access_key_id IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND secret_access_key IS NULL)
					OR (protocol = 's3' AND endpoint IS NOT NULL AND length(trim(endpoint)) > 0 AND region IS NOT NULL AND length(trim(region)) > 0 AND bucket IS NOT NULL AND length(trim(bucket)) > 0 AND access_key_id IS NOT NULL AND length(trim(access_key_id)) > 0 AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND keychain_version > 0 AND secret_access_key IS NOT NULL AND length(trim(secret_access_key)) > 0)
				)
			) STRICT;

			CREATE TABLE gatehouse_workspace_storage_providers (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				priority INTEGER NOT NULL CHECK (priority > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace, provider)
			) STRICT;
		`),
		}, {
			Index:       14,
			Description: "create_storage_objects_and_files",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_storage_objects (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'obj_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				object TEXT NOT NULL CHECK (length(trim(object)) > 0),
				state TEXT NOT NULL CHECK (state IN ('pending', 'success', 'failure')),
				sha256 BLOB CHECK (sha256 IS NULL OR length(sha256) = 32),
				size INTEGER CHECK (size IS NULL OR size >= 0),
				created_at TEXT NOT NULL,
				CHECK ((sha256 IS NULL AND size IS NULL) OR (sha256 IS NOT NULL AND size IS NOT NULL))
			) STRICT;

			CREATE TABLE gatehouse_embedded_storage_objects (
				id TEXT PRIMARY KEY
			) STRICT;

			CREATE TABLE gatehouse_embedded_storage_object_chunks (
				embedded_storage_object TEXT NOT NULL REFERENCES gatehouse_embedded_storage_objects (id),
				ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
				sha256 BLOB NOT NULL CHECK (length(sha256) = 32),
				size INTEGER NOT NULL CHECK (size > 0 AND size <= 262144),
				bytes BLOB NOT NULL CHECK (length(bytes) = size),
				PRIMARY KEY (embedded_storage_object, ordinal)
			) STRICT;

			CREATE TABLE gatehouse_session_files (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sfi_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				storage_object TEXT NOT NULL REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, id),
				UNIQUE (storage_object),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;

			CREATE TABLE gatehouse_project_files (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pfi_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				storage_object TEXT NOT NULL REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, id),
				UNIQUE (storage_object),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			) STRICT;
		`),
		}, {
			Index:       15,
			Description: "create_activity_events",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_activity_events (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				event TEXT NOT NULL CHECK (length(trim(event)) > 0),
				resource_kind TEXT NOT NULL CHECK (resource_kind IN ('project', 'session', 'session_event')),
				project TEXT,
				session TEXT,
				session_event TEXT,
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, id),
				UNIQUE (workspace, id, created_at),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, project)
					REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, session, session_event)
					REFERENCES gatehouse_session_events (workspace, session, id),
				CHECK (
					(resource_kind = 'project' AND project IS NOT NULL AND session IS NULL AND session_event IS NULL)
					OR (resource_kind = 'session' AND session IS NOT NULL AND session_event IS NULL)
					OR (resource_kind = 'session_event' AND session IS NOT NULL AND session_event IS NOT NULL)
				)
			) STRICT;

			CREATE TABLE gatehouse_activity_event_topics (
				workspace TEXT NOT NULL,
				activity TEXT NOT NULL,
				topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, activity, topic),
				FOREIGN KEY (workspace, activity, created_at)
					REFERENCES gatehouse_activity_events (workspace, id, created_at)
			) STRICT;

			CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor
			ON gatehouse_activity_event_topics (workspace, topic, activity);
		`),
		}, {
			Index:       16,
			Description: "create_session_notes",
			Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_notes (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'snt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;

				CREATE INDEX gatehouse_session_notes_by_session_created
				ON gatehouse_session_notes (workspace, session, created_at DESC, id DESC);
			`),
		}, {
			Index:       17,
			Description: "create_session_approval_decisions",
			Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_session_approval_decisions (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					approval TEXT NOT NULL,
					response TEXT NOT NULL,
					created_at TEXT NOT NULL,
					delivered INTEGER NOT NULL CHECK (delivered IN (0, 1)),
					PRIMARY KEY (workspace, session, approval),
					FOREIGN KEY (workspace, session, approval)
						REFERENCES gatehouse_session_events (workspace, session, id),
					FOREIGN KEY (workspace, session, response)
						REFERENCES gatehouse_session_events (workspace, session, id)
				) STRICT;

				CREATE INDEX gatehouse_session_approval_decisions_pending
				ON gatehouse_session_approval_decisions (delivered, created_at, response);
		`),
		}, {
			Index:       18,
			Description: "add_session_note_sensitivity",
			Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_session_notes
				ADD COLUMN sensitive INTEGER NOT NULL DEFAULT FALSE CHECK (sensitive IN (0, 1));
			`),
		}, {
			Index:       19,
			Description: "add_project_note_sensitivity",
			Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_project_notes
				ADD COLUMN sensitive INTEGER NOT NULL DEFAULT FALSE CHECK (sensitive IN (0, 1));
			`),
		}, {
			Index:       20,
			Description: "add_workspace_agent_provider_limits",
			Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN max_input_tokens INTEGER NOT NULL DEFAULT 120000 CHECK (max_input_tokens > 0);

				ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN max_output_tokens INTEGER NOT NULL DEFAULT 16000 CHECK (max_output_tokens > 0);
			`),
		}, {
			Index:       21,
			Description: "add_agent_model_compaction",
			Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_agent_models
				ADD COLUMN compaction TEXT NOT NULL DEFAULT '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":16384}'
					CHECK (json_valid(compaction)) CHECK (json_type(compaction) = 'object');
			`),
		}, {
			Index:       22,
			Description: "add_agent_model_execution_limits",
			Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_agent_models
				ADD COLUMN max_turns INTEGER NOT NULL DEFAULT 127 CHECK (max_turns > 0);

				ALTER TABLE gatehouse_agent_models
				ADD COLUMN max_output_tokens INTEGER NOT NULL DEFAULT 16000 CHECK (max_output_tokens > 0);
			`),
		}, {
			Index:       23,
			Description: "add_agent_contexts",
			Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_agent_contexts (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					root TEXT NOT NULL,
					model TEXT NOT NULL,
					profile TEXT NOT NULL CHECK (length(trim(profile)) > 0),
					state TEXT NOT NULL CHECK (json_valid(state)) CHECK (json_type(state) = 'object'),
					updated_at TEXT NOT NULL,
					PRIMARY KEY (workspace, session, root),
					FOREIGN KEY (workspace, session, root) REFERENCES gatehouse_session_events (workspace, session, id),
					FOREIGN KEY (workspace, model) REFERENCES gatehouse_workspace_agents (workspace_id, model_id)
				) STRICT;
			`),
		}, {
			Index:       24,
			Description: "increase_default_agent_model_context_window",
			Builder: staticMigrationBuilder(`
				UPDATE gatehouse_agent_models
				SET compaction = '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":16384}'
				WHERE compaction = '{"algorithm":"mcmtr","history_bytes":81920,"buffer_bytes":16384}';
			`),
		}, {
			Index:       25,
			Description: "add_session_event_metrics",
			Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_session_events
				ADD COLUMN metrics TEXT CHECK (metrics IS NULL OR (json_valid(metrics) AND json_type(metrics) = 'object'));
			`),
		}, {
			Index:       26,
			Description: "index_latest_agent_contexts",
			Builder: staticMigrationBuilder(`
				CREATE INDEX gatehouse_agent_contexts_latest
				ON gatehouse_agent_contexts (workspace, session, model, profile, updated_at DESC, root DESC);
			`),
		}, {
			Index:       27,
			Description: "drop_legacy_workspace_tools_and_resources",
			Builder: staticMigrationBuilder(`
				DROP TABLE IF EXISTS gatehouse_group_tool_grants;
				DROP TABLE IF EXISTS gatehouse_group_resource_grants;
				DROP TABLE IF EXISTS gatehouse_tools;
				DROP TABLE IF EXISTS gatehouse_resources;
			`),
		}, {
			Index:       28,
			Description: "create_session_secrets",
			Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_session_secrets (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ssc_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
					description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (length(CAST(description AS BLOB)) <= 4096),
					ciphertext TEXT NOT NULL,
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					PRIMARY KEY (workspace, session, id),
					FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
				) STRICT;

				CREATE INDEX gatehouse_session_secrets_by_session_created
				ON gatehouse_session_secrets (workspace, session, created_at DESC, id DESC);
			`),
		}, {
			Index:       29,
			Description: "create_project_secrets",
			Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_project_secrets (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'psc_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
					description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (length(CAST(description AS BLOB)) <= 4096),
					ciphertext TEXT NOT NULL,
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					PRIMARY KEY (workspace, project, id),
					FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
				) STRICT;

				CREATE INDEX gatehouse_project_secrets_by_project_created
				ON gatehouse_project_secrets (workspace, project, created_at DESC, id DESC);
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
