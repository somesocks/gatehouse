package migrations

func sqliteMigrationV007CreateAgentsAndProjects() VersionedMigration {
	return VersionedMigration{
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
	}
}
