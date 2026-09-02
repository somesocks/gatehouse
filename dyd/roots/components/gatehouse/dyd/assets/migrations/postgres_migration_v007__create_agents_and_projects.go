package migrations

func postgresMigrationV007CreateAgentsAndProjects() VersionedMigration {
	return VersionedMigration{
		Index:       7,
		Description: "create_agents_and_projects",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_providers (
				id TEXT PRIMARY KEY CHECK (id ~ '^apr_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
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
				id TEXT PRIMARY KEY CHECK (id ~ '^amd_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
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

			CREATE TABLE gatehouse_projects (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (id ~ '^prj_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				description TEXT CHECK (description IS NULL OR length(trim(description)) > 0),
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, id)
			);

			CREATE TABLE gatehouse_project_notes (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^pnt_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (octet_length(title) <= 256),
				description TEXT NOT NULL CHECK (octet_length(description) <= 4096),
				body TEXT NOT NULL CHECK (octet_length(body) <= 1048576),
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, project, id),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			);

			CREATE INDEX gatehouse_project_notes_by_project_created
			ON gatehouse_project_notes (workspace, project, created_at DESC, id DESC);

			CREATE TABLE gatehouse_project_principal_grants (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace, project, principal),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			);

			CREATE TABLE gatehouse_project_group_grants (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				"group" TEXT NOT NULL,
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace, project, "group"),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			);
		`),
	}
}
