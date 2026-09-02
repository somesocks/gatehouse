package migrations

func postgresMigrationV008CreateSessionsGrantsAndEvents() VersionedMigration {
	return VersionedMigration{
		Index:       8,
		Description: "create_sessions_grants_and_events",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_sessions (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				project TEXT,
				id TEXT NOT NULL CHECK (id ~ '^ses_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT CHECK (author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
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
				id TEXT NOT NULL CHECK (id ~ '^sev_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				parent TEXT,
				kind TEXT NOT NULL CHECK (length(trim(kind)) > 0),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				author_gateway TEXT CHECK (author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
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
			ON gatehouse_session_events (workspace, session, id);

			CREATE INDEX gatehouse_session_events_roots
			ON gatehouse_session_events (workspace, session, id)
			WHERE parent IS NULL;

			CREATE INDEX gatehouse_session_events_children
			ON gatehouse_session_events (workspace, session, parent, id);
		`),
	}
}
