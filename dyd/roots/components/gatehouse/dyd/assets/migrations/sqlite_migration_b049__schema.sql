CREATE TABLE gatehouse_workspaces (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wsp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias GLOB '[a-z]*')
					CHECK (alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;
CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prn_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				revision INTEGER NOT NULL CHECK (revision > 0),
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
			, revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0)) STRICT;
CREATE INDEX gatehouse_identities_by_principal
			ON gatehouse_identities (principal_id);
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
			, compaction TEXT NOT NULL DEFAULT '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":16384}'
					CHECK (json_valid(compaction)) CHECK (json_type(compaction) = 'object'), max_turns INTEGER NOT NULL DEFAULT 127 CHECK (max_turns > 0), max_output_tokens INTEGER NOT NULL DEFAULT 16000 CHECK (max_output_tokens > 0)) STRICT;
CREATE TABLE gatehouse_projects (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prj_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				description TEXT CHECK (description IS NULL OR length(trim(description)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, id)
			) STRICT;
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
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
				PRIMARY KEY (workspace, provider)
			) STRICT;
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
CREATE TABLE "gatehouse_workspace_grants" (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id),
				"group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL CHECK (revision > 0),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
CREATE UNIQUE INDEX gatehouse_workspace_grants_principal_once ON gatehouse_workspace_grants (workspace, role, principal) WHERE principal IS NOT NULL;
CREATE UNIQUE INDEX gatehouse_workspace_grants_group_once ON gatehouse_workspace_grants (workspace, role, "group") WHERE "group" IS NOT NULL;
CREATE INDEX gatehouse_workspace_grants_principal_enabled ON gatehouse_workspace_grants (principal, workspace) WHERE principal IS NOT NULL AND enabled = TRUE;
CREATE INDEX gatehouse_workspace_grants_group_enabled ON gatehouse_workspace_grants (workspace, "group") WHERE "group" IS NOT NULL AND enabled = TRUE;
CREATE TABLE "gatehouse_project_grants" (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id),
				"group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
CREATE UNIQUE INDEX gatehouse_project_grants_principal_once ON gatehouse_project_grants (workspace, project, role, principal) WHERE principal IS NOT NULL;
CREATE UNIQUE INDEX gatehouse_project_grants_group_once ON gatehouse_project_grants (workspace, project, role, "group") WHERE "group" IS NOT NULL;
CREATE INDEX gatehouse_project_grants_principal_enabled ON gatehouse_project_grants (principal, workspace, project) WHERE principal IS NOT NULL AND enabled = TRUE;
CREATE INDEX gatehouse_project_grants_group_enabled ON gatehouse_project_grants (workspace, "group", project) WHERE "group" IS NOT NULL AND enabled = TRUE;
CREATE UNIQUE INDEX gatehouse_groups_id_once ON gatehouse_groups (id);
CREATE UNIQUE INDEX gatehouse_group_members_group_principal_once ON gatehouse_group_members (group_id, principal_id);
CREATE UNIQUE INDEX gatehouse_projects_id_once ON gatehouse_projects (id);
CREATE UNIQUE INDEX gatehouse_project_files_id_once ON gatehouse_project_files (id);
CREATE UNIQUE INDEX gatehouse_project_secrets_id_once ON gatehouse_project_secrets (id);
CREATE TABLE gatehouse_system_grants (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'syg_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				principal TEXT NOT NULL UNIQUE REFERENCES gatehouse_principals (id),
				role TEXT NOT NULL CHECK (role = 'manager'),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL CHECK (revision > 0)
			) STRICT;
CREATE TABLE gatehouse_sessions (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id), project TEXT,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ses_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), created_at TEXT NOT NULL, PRIMARY KEY (workspace, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL))
			) STRICT;
CREATE TABLE gatehouse_session_grants (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), workspace TEXT NOT NULL, session TEXT NOT NULL, role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')), principal TEXT REFERENCES gatehouse_principals (id), "group" TEXT, enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id), FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
CREATE TABLE gatehouse_session_files (
				workspace TEXT NOT NULL, session TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sfi_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), storage_object TEXT NOT NULL REFERENCES gatehouse_storage_objects (id), name TEXT NOT NULL CHECK (length(trim(name)) > 0), media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0), created_at TEXT NOT NULL, enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), PRIMARY KEY (workspace, session, id), UNIQUE (storage_object), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;
CREATE TABLE gatehouse_session_secrets (
				workspace TEXT NOT NULL, session TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ssc_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id), description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (length(CAST(description AS BLOB)) <= 4096), ciphertext TEXT NOT NULL, enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (workspace, session, id), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;
CREATE TABLE gatehouse_agent_tasks__session_name (workspace TEXT NOT NULL, session TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY (workspace, session), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)) STRICT;
CREATE TABLE gatehouse_session_events (
				workspace TEXT NOT NULL, session TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sev_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), parent TEXT, kind TEXT NOT NULL CHECK (length(trim(kind)) > 0), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), payload TEXT NOT NULL CHECK (json_valid(payload)) CHECK (json_type(payload) = 'object'), created_at TEXT NOT NULL, metrics TEXT CHECK (metrics IS NULL OR (json_valid(metrics) AND json_type(metrics) = 'object')), PRIMARY KEY (workspace, session, id), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id), FOREIGN KEY (workspace, session, parent) REFERENCES gatehouse_session_events (workspace, session, id), FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)), CHECK (parent IS NULL OR parent <> id)
			) STRICT;
CREATE TABLE gatehouse_session_approval_decisions (workspace TEXT NOT NULL, session TEXT NOT NULL, approval TEXT NOT NULL, response TEXT NOT NULL, created_at TEXT NOT NULL, delivered INTEGER NOT NULL CHECK (delivered IN (0, 1)), PRIMARY KEY (workspace, session, approval), FOREIGN KEY (workspace, session, approval) REFERENCES gatehouse_session_events (workspace, session, id), FOREIGN KEY (workspace, session, response) REFERENCES gatehouse_session_events (workspace, session, id)) STRICT;
CREATE TABLE gatehouse_agent_contexts (workspace TEXT NOT NULL, session TEXT NOT NULL, root TEXT NOT NULL, model TEXT NOT NULL, profile TEXT NOT NULL CHECK (length(trim(profile)) > 0), state TEXT NOT NULL CHECK (json_valid(state)) CHECK (json_type(state) = 'object'), updated_at TEXT NOT NULL, PRIMARY KEY (workspace, session, root), FOREIGN KEY (workspace, session, root) REFERENCES gatehouse_session_events (workspace, session, id), FOREIGN KEY (workspace, model) REFERENCES gatehouse_workspace_agents (workspace_id, id)) STRICT;
CREATE TABLE gatehouse_project_notes (workspace TEXT NOT NULL, project TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pnt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256), description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096), body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576), enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), created_at TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision > 0), PRIMARY KEY (workspace, project, id), FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id), FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL))) STRICT;
CREATE TABLE gatehouse_project_note_revisions (workspace TEXT NOT NULL, project TEXT NOT NULL, note TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision > 0), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256), description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096), body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576), sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), created_at TEXT NOT NULL, PRIMARY KEY (workspace, project, note, revision), FOREIGN KEY (workspace, project, note) REFERENCES gatehouse_project_notes (workspace, project, id), FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL))) STRICT;
CREATE TABLE gatehouse_session_notes (workspace TEXT NOT NULL, session TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'snt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256), description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096), body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576), enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), created_at TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision > 0), PRIMARY KEY (workspace, session, id), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id), FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL))) STRICT;
CREATE TABLE gatehouse_session_note_revisions (workspace TEXT NOT NULL, session TEXT NOT NULL, note TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision > 0), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256), description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096), body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576), sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), created_at TEXT NOT NULL, PRIMARY KEY (workspace, session, note, revision), FOREIGN KEY (workspace, session, note) REFERENCES gatehouse_session_notes (workspace, session, id), FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL) OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL))) STRICT;
CREATE TABLE gatehouse_project_tasks (workspace TEXT NOT NULL, project TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ptk_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256), description TEXT CHECK (description IS NULL OR length(CAST(description AS BLOB)) <= 4096), sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'in_progress', 'done', 'cancelled')), enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), creator_principal TEXT REFERENCES gatehouse_principals (id), creator_agent TEXT, creator_gateway TEXT CHECK (creator_gateway IS NULL OR (length(creator_gateway) = 30 AND substr(creator_gateway, 1, 4) = 'gwy_' AND substr(creator_gateway, 5, 1) GLOB '[0-7]' AND substr(creator_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), created_at TEXT NOT NULL, updater_principal TEXT REFERENCES gatehouse_principals (id), updater_agent TEXT, updater_gateway TEXT CHECK (updater_gateway IS NULL OR (length(updater_gateway) = 30 AND substr(updater_gateway, 1, 4) = 'gwy_' AND substr(updater_gateway, 5, 1) GLOB '[0-7]' AND substr(updater_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), updated_at TEXT NOT NULL, PRIMARY KEY (workspace, project, id), UNIQUE (id), FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id), FOREIGN KEY (workspace, creator_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), FOREIGN KEY (workspace, updater_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((creator_principal IS NOT NULL AND creator_agent IS NULL AND creator_gateway IS NULL) OR (creator_principal IS NULL AND creator_agent IS NOT NULL AND creator_gateway IS NULL) OR (creator_principal IS NULL AND creator_agent IS NULL AND creator_gateway IS NOT NULL)), CHECK ((updater_principal IS NOT NULL AND updater_agent IS NULL AND updater_gateway IS NULL) OR (updater_principal IS NULL AND updater_agent IS NOT NULL AND updater_gateway IS NULL) OR (updater_principal IS NULL AND updater_agent IS NULL AND updater_gateway IS NOT NULL))) STRICT;
CREATE TABLE gatehouse_session_tasks (workspace TEXT NOT NULL, session TEXT NOT NULL, id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'stk_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256), description TEXT CHECK (description IS NULL OR length(CAST(description AS BLOB)) <= 4096), sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'in_progress', 'done', 'cancelled')), enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), creator_principal TEXT REFERENCES gatehouse_principals (id), creator_agent TEXT, creator_gateway TEXT CHECK (creator_gateway IS NULL OR (length(creator_gateway) = 30 AND substr(creator_gateway, 1, 4) = 'gwy_' AND substr(creator_gateway, 5, 1) GLOB '[0-7]' AND substr(creator_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), created_at TEXT NOT NULL, updater_principal TEXT REFERENCES gatehouse_principals (id), updater_agent TEXT, updater_gateway TEXT CHECK (updater_gateway IS NULL OR (length(updater_gateway) = 30 AND substr(updater_gateway, 1, 4) = 'gwy_' AND substr(updater_gateway, 5, 1) GLOB '[0-7]' AND substr(updater_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')), updated_at TEXT NOT NULL, PRIMARY KEY (workspace, session, id), UNIQUE (id), FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id), FOREIGN KEY (workspace, creator_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), FOREIGN KEY (workspace, updater_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id), CHECK ((creator_principal IS NOT NULL AND creator_agent IS NULL AND creator_gateway IS NULL) OR (creator_principal IS NULL AND creator_agent IS NOT NULL AND creator_gateway IS NULL) OR (creator_principal IS NULL AND creator_agent IS NULL AND creator_gateway IS NOT NULL)), CHECK ((updater_principal IS NOT NULL AND updater_agent IS NULL AND updater_gateway IS NULL) OR (updater_principal IS NULL AND updater_agent IS NOT NULL AND updater_gateway IS NULL) OR (updater_principal IS NULL AND updater_agent IS NULL AND updater_gateway IS NOT NULL))) STRICT;
CREATE UNIQUE INDEX gatehouse_sessions_id_once ON gatehouse_sessions (id);
CREATE UNIQUE INDEX gatehouse_session_events_id_once ON gatehouse_session_events (id);
CREATE UNIQUE INDEX gatehouse_session_files_id_once ON gatehouse_session_files (id);
CREATE UNIQUE INDEX gatehouse_session_notes_id_once ON gatehouse_session_notes (id);
CREATE UNIQUE INDEX gatehouse_session_secrets_id_once ON gatehouse_session_secrets (id);
CREATE UNIQUE INDEX gatehouse_project_notes_id_once ON gatehouse_project_notes (id);
CREATE INDEX gatehouse_sessions_by_workspace ON gatehouse_sessions (workspace, created_at);
CREATE UNIQUE INDEX gatehouse_session_grants_principal_once ON gatehouse_session_grants (workspace, session, role, principal) WHERE principal IS NOT NULL;
CREATE UNIQUE INDEX gatehouse_session_grants_group_once ON gatehouse_session_grants (workspace, session, role, "group") WHERE "group" IS NOT NULL;
CREATE INDEX gatehouse_session_grants_principal_enabled ON gatehouse_session_grants (principal, workspace, session) WHERE principal IS NOT NULL AND enabled = TRUE;
CREATE INDEX gatehouse_session_grants_group_enabled ON gatehouse_session_grants (workspace, "group", session) WHERE "group" IS NOT NULL AND enabled = TRUE;
CREATE INDEX gatehouse_session_secrets_by_session_created ON gatehouse_session_secrets (workspace, session, created_at DESC, id DESC);
CREATE INDEX gatehouse_session_events_by_session_order ON gatehouse_session_events (workspace, session, id);
CREATE INDEX gatehouse_session_events_roots ON gatehouse_session_events (workspace, session, id) WHERE parent IS NULL;
CREATE INDEX gatehouse_session_events_children ON gatehouse_session_events (workspace, session, parent, id);
CREATE INDEX gatehouse_session_approval_decisions_pending ON gatehouse_session_approval_decisions (delivered, created_at, response);
CREATE INDEX gatehouse_agent_contexts_latest ON gatehouse_agent_contexts (workspace, session, model, profile, updated_at DESC, root DESC);
CREATE INDEX gatehouse_project_notes_by_project_created ON gatehouse_project_notes (workspace, project, created_at DESC, id DESC);
CREATE INDEX gatehouse_session_notes_by_session_created ON gatehouse_session_notes (workspace, session, created_at DESC, id DESC);
CREATE INDEX gatehouse_project_tasks_by_project_created ON gatehouse_project_tasks (workspace, project, created_at DESC, id DESC);
CREATE INDEX gatehouse_session_tasks_by_session_created ON gatehouse_session_tasks (workspace, session, created_at DESC, id DESC);
CREATE TRIGGER gatehouse_project_note_revisions_immutable_update BEFORE UPDATE ON gatehouse_project_note_revisions BEGIN SELECT RAISE(ABORT, 'project note revisions are immutable'); END;
CREATE TRIGGER gatehouse_project_note_revisions_immutable_delete BEFORE DELETE ON gatehouse_project_note_revisions BEGIN SELECT RAISE(ABORT, 'project note revisions are immutable'); END;
CREATE TRIGGER gatehouse_session_note_revisions_immutable_update BEFORE UPDATE ON gatehouse_session_note_revisions BEGIN SELECT RAISE(ABORT, 'session note revisions are immutable'); END;
CREATE TRIGGER gatehouse_session_note_revisions_immutable_delete BEFORE DELETE ON gatehouse_session_note_revisions BEGIN SELECT RAISE(ABORT, 'session note revisions are immutable'); END;
CREATE TABLE gatehouse_project_record_schemas (
				workspace TEXT NOT NULL, project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prs_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT NOT NULL CHECK (name GLOB '[a-z]*') CHECK (name NOT GLOB '*[^a-z0-9_]*'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (length(CAST(label AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, id), UNIQUE (id), UNIQUE (workspace, project, name),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
CREATE INDEX gatehouse_project_record_schemas_by_project_created ON gatehouse_project_record_schemas (workspace, project, created_at DESC, id DESC);
CREATE TABLE gatehouse_project_records (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, id), UNIQUE (id),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
CREATE INDEX gatehouse_project_records_by_schema_created ON gatehouse_project_records (workspace, project, schema, created_at DESC, id DESC);
CREATE TABLE gatehouse_activity_events (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), event TEXT NOT NULL CHECK (length(trim(event)) > 0),
				resource_kind TEXT NOT NULL CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_task', 'project_secret', 'project_record_schema', 'project_record_attribute', 'project_record', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_task', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				resource_keychain_id TEXT, resource_keychain_version INTEGER, resource_agent_provider TEXT, resource_agent_model TEXT, resource_group TEXT, resource_group_member_group TEXT, resource_group_member_principal TEXT, resource_identity TEXT, resource_principal TEXT, resource_project TEXT, resource_project_file TEXT, resource_project_grant TEXT, resource_project_note TEXT, resource_project_task TEXT REFERENCES gatehouse_project_tasks (id), resource_project_secret TEXT, resource_project_record_schema TEXT, resource_project_record_attribute TEXT, resource_project_record TEXT, resource_session TEXT, resource_session_event TEXT, resource_session_file TEXT, resource_session_grant TEXT, resource_session_note TEXT, resource_session_task TEXT REFERENCES gatehouse_session_tasks (id), resource_session_secret TEXT, resource_storage_provider TEXT, resource_system_grant TEXT REFERENCES gatehouse_system_grants (id), resource_workspace TEXT REFERENCES gatehouse_workspaces (id), resource_workspace_agent_workspace TEXT, resource_workspace_agent_id TEXT, resource_workspace_grant TEXT, resource_workspace_storage_provider_workspace TEXT, resource_workspace_storage_provider_provider TEXT, created_at TEXT NOT NULL,
				FOREIGN KEY (resource_keychain_id, resource_keychain_version) REFERENCES gatehouse_keychains (id, version), FOREIGN KEY (resource_agent_provider) REFERENCES gatehouse_agent_providers (id), FOREIGN KEY (resource_agent_model) REFERENCES gatehouse_agent_models (id), FOREIGN KEY (resource_group) REFERENCES gatehouse_groups (id), FOREIGN KEY (resource_group_member_group, resource_group_member_principal) REFERENCES gatehouse_group_members (group_id, principal_id), FOREIGN KEY (resource_identity) REFERENCES gatehouse_identities (id), FOREIGN KEY (resource_principal) REFERENCES gatehouse_principals (id), FOREIGN KEY (resource_project) REFERENCES gatehouse_projects (id), FOREIGN KEY (resource_project_file) REFERENCES gatehouse_project_files (id), FOREIGN KEY (resource_project_grant) REFERENCES gatehouse_project_grants (id), FOREIGN KEY (resource_project_note) REFERENCES gatehouse_project_notes (id), FOREIGN KEY (resource_project_secret) REFERENCES gatehouse_project_secrets (id), FOREIGN KEY (resource_session) REFERENCES gatehouse_sessions (id), FOREIGN KEY (resource_session_event) REFERENCES gatehouse_session_events (id), FOREIGN KEY (resource_session_file) REFERENCES gatehouse_session_files (id), FOREIGN KEY (resource_session_grant) REFERENCES gatehouse_session_grants (id), FOREIGN KEY (resource_session_note) REFERENCES gatehouse_session_notes (id), FOREIGN KEY (resource_session_secret) REFERENCES gatehouse_session_secrets (id), FOREIGN KEY (resource_storage_provider) REFERENCES gatehouse_storage_providers (id), FOREIGN KEY (resource_workspace_grant) REFERENCES gatehouse_workspace_grants (id), FOREIGN KEY (resource_workspace_agent_workspace, resource_workspace_agent_id) REFERENCES gatehouse_workspace_agents (workspace_id, id), FOREIGN KEY (resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) REFERENCES gatehouse_workspace_storage_providers (workspace, provider),
				CHECK (CASE resource_kind WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_id IS NOT NULL WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL WHEN 'project_record_schema' THEN resource_project_record_schema IS NOT NULL WHEN 'project_record_attribute' THEN resource_project_record_attribute IS NOT NULL WHEN 'project_record' THEN resource_project_record IS NOT NULL WHEN 'project_task' THEN resource_project_task IS NOT NULL WHEN 'session_task' THEN resource_session_task IS NOT NULL WHEN 'system_grant' THEN resource_system_grant IS NOT NULL WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL WHEN 'agent_model' THEN resource_agent_model IS NOT NULL WHEN 'group' THEN resource_group IS NOT NULL WHEN 'identity' THEN resource_identity IS NOT NULL WHEN 'principal' THEN resource_principal IS NOT NULL WHEN 'project' THEN resource_project IS NOT NULL WHEN 'project_file' THEN resource_project_file IS NOT NULL WHEN 'project_grant' THEN resource_project_grant IS NOT NULL WHEN 'project_note' THEN resource_project_note IS NOT NULL WHEN 'project_secret' THEN resource_project_secret IS NOT NULL WHEN 'session' THEN resource_session IS NOT NULL WHEN 'session_event' THEN resource_session_event IS NOT NULL WHEN 'session_file' THEN resource_session_file IS NOT NULL WHEN 'session_grant' THEN resource_session_grant IS NOT NULL WHEN 'session_note' THEN resource_session_note IS NOT NULL WHEN 'session_secret' THEN resource_session_secret IS NOT NULL WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL WHEN 'workspace' THEN resource_workspace IS NOT NULL WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL END AND ((resource_keychain_id IS NOT NULL) + (resource_keychain_version IS NOT NULL) + (resource_agent_provider IS NOT NULL) + (resource_agent_model IS NOT NULL) + (resource_group IS NOT NULL) + (resource_group_member_group IS NOT NULL) + (resource_group_member_principal IS NOT NULL) + (resource_identity IS NOT NULL) + (resource_principal IS NOT NULL) + (resource_project IS NOT NULL) + (resource_project_file IS NOT NULL) + (resource_project_grant IS NOT NULL) + (resource_project_note IS NOT NULL) + (resource_project_task IS NOT NULL) + (resource_project_secret IS NOT NULL) + (resource_project_record_schema IS NOT NULL) + (resource_project_record_attribute IS NOT NULL) + (resource_project_record IS NOT NULL) + (resource_session IS NOT NULL) + (resource_session_event IS NOT NULL) + (resource_session_file IS NOT NULL) + (resource_session_grant IS NOT NULL) + (resource_session_note IS NOT NULL) + (resource_session_task IS NOT NULL) + (resource_session_secret IS NOT NULL) + (resource_storage_provider IS NOT NULL) + (resource_system_grant IS NOT NULL) + (resource_workspace IS NOT NULL) + (resource_workspace_agent_workspace IS NOT NULL) + (resource_workspace_agent_id IS NOT NULL) + (resource_workspace_grant IS NOT NULL) + (resource_workspace_storage_provider_workspace IS NOT NULL) + (resource_workspace_storage_provider_provider IS NOT NULL)) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END)
			) STRICT;
CREATE TABLE gatehouse_activity_event_topics (activity TEXT NOT NULL, topic TEXT NOT NULL CHECK (length(trim(topic)) > 0), PRIMARY KEY (activity, topic), FOREIGN KEY (activity) REFERENCES gatehouse_activity_events (id)) STRICT;
CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor ON gatehouse_activity_event_topics (topic, activity);
CREATE TABLE "gatehouse_agent_tasks__session_event_reply" (
				workspace TEXT NOT NULL, session TEXT NOT NULL, event TEXT NOT NULL, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, event),
				FOREIGN KEY (workspace, session, event) REFERENCES gatehouse_session_events (workspace, session, id)
			) STRICT;
CREATE TABLE "gatehouse_project_record_attributes" (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pra_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT NOT NULL CHECK (name GLOB '[a-z]*') CHECK (name NOT GLOB '*[^a-z0-9_]*'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (length(CAST(label AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				type TEXT NOT NULL CHECK (type IN ('text', 'number', 'boolean', 'datetime', 'record', 'file')),
				target_schema TEXT,
				cardinality TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
				uniqueness TEXT NOT NULL CHECK (uniqueness IN ('none', 'record', 'global')),
				display TEXT NOT NULL CHECK (display IN ('none', 'primary', 'secondary')),
				display_order INTEGER NOT NULL DEFAULT 0 CHECK (display_order >= 0),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, id), UNIQUE (id), UNIQUE (workspace, project, schema, name),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, project, target_schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((type = 'record' AND target_schema IS NOT NULL) OR (type <> 'record' AND target_schema IS NULL)),
				CHECK (NOT (cardinality = 'one' AND uniqueness = 'record')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
CREATE TABLE "gatehouse_project_record_values" (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL, record TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prv_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				attribute TEXT NOT NULL,
				value_type TEXT NOT NULL CHECK (value_type IN ('text', 'number', 'boolean', 'datetime', 'record', 'file')),
				value_text TEXT, value_number REAL, value_boolean INTEGER CHECK (value_boolean IN (0, 1)), value_datetime TEXT, value_reference_schema TEXT, value_reference TEXT, value_reference_file TEXT,
				value_key TEXT NOT NULL, attribute_cardinality TEXT NOT NULL CHECK (attribute_cardinality IN ('one', 'many')), attribute_uniqueness TEXT NOT NULL CHECK (attribute_uniqueness IN ('none', 'record', 'global')),
				sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, record, id), UNIQUE (id),
				FOREIGN KEY (workspace, project, schema, record) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute, value_type, attribute_cardinality, attribute_uniqueness) REFERENCES "gatehouse_project_record_attributes" (workspace, project, schema, id, type, cardinality, uniqueness) ON UPDATE CASCADE ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute, value_reference_schema) REFERENCES "gatehouse_project_record_attributes" (workspace, project, schema, id, target_schema) ON UPDATE CASCADE ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, value_reference_schema, value_reference) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE RESTRICT,
				FOREIGN KEY (workspace, project, value_reference_file) REFERENCES gatehouse_project_files (workspace, project, id) ON DELETE RESTRICT,
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((value_type = 'text' AND value_text IS NOT NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'number' AND value_text IS NULL AND value_number IS NOT NULL AND value_number = value_number AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'boolean' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NOT NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'datetime' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NOT NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'record' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NOT NULL AND value_reference IS NOT NULL AND value_reference_file IS NULL) OR (value_type = 'file' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NOT NULL)),
				CHECK (NOT (sensitive = 1 AND attribute_uniqueness = 'global')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
CREATE UNIQUE INDEX gatehouse_project_record_attributes_value_compatibility ON gatehouse_project_record_attributes (workspace, project, schema, id, type, cardinality, uniqueness);
CREATE UNIQUE INDEX gatehouse_project_record_attributes_reference_target ON gatehouse_project_record_attributes (workspace, project, schema, id, target_schema);
CREATE INDEX gatehouse_project_record_attributes_by_schema_name ON gatehouse_project_record_attributes (workspace, project, schema, name);
CREATE INDEX gatehouse_project_record_attributes_by_schema_display_order_name ON gatehouse_project_record_attributes (workspace, project, schema, display_order, name, id);
CREATE UNIQUE INDEX gatehouse_project_record_values_one ON gatehouse_project_record_values (workspace, project, schema, record, attribute) WHERE attribute_cardinality = 'one';
CREATE UNIQUE INDEX gatehouse_project_record_values_record_unique ON gatehouse_project_record_values (workspace, project, schema, record, attribute, value_key) WHERE attribute_uniqueness = 'record';
CREATE UNIQUE INDEX gatehouse_project_record_values_global_unique ON gatehouse_project_record_values (workspace, project, schema, attribute, value_key) WHERE attribute_uniqueness = 'global';
CREATE INDEX gatehouse_project_record_values_by_record_created ON gatehouse_project_record_values (workspace, project, schema, record, created_at DESC, id DESC);
CREATE INDEX gatehouse_project_record_values_by_reference ON gatehouse_project_record_values (workspace, project, value_reference_schema, value_reference, id DESC) WHERE value_type = 'record';
CREATE INDEX gatehouse_project_record_values_by_file_reference ON gatehouse_project_record_values (workspace, project, value_reference_file, id DESC) WHERE value_type = 'file';
CREATE TRIGGER gatehouse_project_record_values_file_reference_validate_insert
			BEFORE INSERT ON gatehouse_project_record_values
			WHEN NEW.value_type = 'file'
			BEGIN SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM gatehouse_project_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = NEW.workspace AND files.project = NEW.project AND files.id = NEW.value_reference_file AND files.enabled = 1 AND objects.state = 'success') THEN RAISE(ABORT, 'project file reference is unavailable') END; END;
CREATE TRIGGER gatehouse_project_record_values_file_reference_validate_update
			BEFORE UPDATE ON gatehouse_project_record_values
			WHEN NEW.value_type = 'file'
			BEGIN SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM gatehouse_project_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = NEW.workspace AND files.project = NEW.project AND files.id = NEW.value_reference_file AND files.enabled = 1 AND objects.state = 'success') THEN RAISE(ABORT, 'project file reference is unavailable') END; END;
CREATE TRIGGER gatehouse_project_files_reject_remove_when_referenced
			BEFORE UPDATE OF enabled ON gatehouse_project_files
			WHEN OLD.enabled = 1 AND NEW.enabled = 0 AND EXISTS (SELECT 1 FROM gatehouse_project_record_values WHERE workspace = OLD.workspace AND project = OLD.project AND value_type = 'file' AND value_reference_file = OLD.id)
			BEGIN SELECT RAISE(ABORT, 'project file is referenced by a record value'); END;
CREATE TABLE "gatehouse_workspace_agents" (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wag_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				alias TEXT NOT NULL CHECK (alias <> '') CHECK (alias NOT GLOB '*[^a-z0-9_/-]*') CHECK (alias NOT GLOB '/*' AND alias NOT GLOB '*/' AND alias NOT GLOB '*//*'),
				model_id TEXT NOT NULL REFERENCES gatehouse_agent_models (id),
				revision INTEGER NOT NULL CHECK (revision > 0),
				label TEXT,
				system_prompt TEXT,
				prelude TEXT,
				"default" INTEGER NOT NULL CHECK ("default" IN (0, 1)),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				UNIQUE (workspace_id, id), UNIQUE (workspace_id, alias),
				CHECK ("default" = 0 OR enabled = 1)
			) STRICT;
CREATE UNIQUE INDEX gatehouse_workspace_agents_default_once
			ON gatehouse_workspace_agents (workspace_id) WHERE "default" = 1;
