package migrations

func sqliteMigrationV032AddNoteRevisions() VersionedMigration {
	return VersionedMigration{
		Index:       32,
		Description: "add_note_revisions",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_project_notes_migration_32 (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pnt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					author_principal TEXT REFERENCES gatehouse_principals (id),
					author_agent TEXT,
					author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
					title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
					description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
					body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
					sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)),
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					revision INTEGER NOT NULL CHECK (revision > 0),
					PRIMARY KEY (workspace, project, id),
					FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
					FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					)
				) STRICT;

				INSERT INTO gatehouse_project_notes_migration_32 (
					workspace, project, id, author_principal, author_agent, author_gateway,
					title, description, body, sensitive, enabled, created_at, revision
				)
				SELECT workspace, project, id, author_principal, NULL, NULL,
					title, description, body, sensitive, enabled, created_at, 1
				FROM gatehouse_project_notes;

				DROP TABLE gatehouse_project_notes;
				ALTER TABLE gatehouse_project_notes_migration_32 RENAME TO gatehouse_project_notes;
				CREATE INDEX gatehouse_project_notes_by_project_created
				ON gatehouse_project_notes (workspace, project, created_at DESC, id DESC);

				CREATE TABLE gatehouse_session_notes_migration_32 (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'snt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					author_principal TEXT REFERENCES gatehouse_principals (id),
					author_agent TEXT,
					author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
					title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
					description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
					body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
					sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)),
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					revision INTEGER NOT NULL CHECK (revision > 0),
					PRIMARY KEY (workspace, session, id),
					FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
					FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					)
				) STRICT;

				INSERT INTO gatehouse_session_notes_migration_32 (
					workspace, session, id, author_principal, author_agent, author_gateway,
					title, description, body, sensitive, enabled, created_at, revision
				)
				SELECT workspace, session, id, author_principal, NULL, NULL,
					title, description, body, sensitive, enabled, created_at, 1
				FROM gatehouse_session_notes;

				DROP TABLE gatehouse_session_notes;
				ALTER TABLE gatehouse_session_notes_migration_32 RENAME TO gatehouse_session_notes;
				CREATE INDEX gatehouse_session_notes_by_session_created
				ON gatehouse_session_notes (workspace, session, created_at DESC, id DESC);

				CREATE TABLE gatehouse_project_note_revisions (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					note TEXT NOT NULL,
					revision INTEGER NOT NULL CHECK (revision > 0),
					author_principal TEXT REFERENCES gatehouse_principals (id),
					author_agent TEXT,
					author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
					title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
					description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
					body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
					sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)),
					created_at TEXT NOT NULL,
					PRIMARY KEY (workspace, project, note, revision),
					FOREIGN KEY (workspace, project, note) REFERENCES gatehouse_project_notes (workspace, project, id),
					FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					)
				) STRICT;

				CREATE TABLE gatehouse_session_note_revisions (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					note TEXT NOT NULL,
					revision INTEGER NOT NULL CHECK (revision > 0),
					author_principal TEXT REFERENCES gatehouse_principals (id),
					author_agent TEXT,
					author_gateway TEXT CHECK (author_gateway IS NULL OR (length(author_gateway) = 30 AND substr(author_gateway, 1, 4) = 'gwy_' AND substr(author_gateway, 5, 1) GLOB '[0-7]' AND substr(author_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
					title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
					description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
					body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
					sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)),
					created_at TEXT NOT NULL,
					PRIMARY KEY (workspace, session, note, revision),
					FOREIGN KEY (workspace, session, note) REFERENCES gatehouse_session_notes (workspace, session, id),
					FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					)
				) STRICT;

				INSERT INTO gatehouse_project_note_revisions (
					workspace, project, note, revision, author_principal, author_agent, author_gateway,
					title, description, body, sensitive, created_at
				)
				SELECT workspace, project, id, revision, author_principal, author_agent, author_gateway,
					title, description, body, sensitive, created_at
				FROM gatehouse_project_notes;

				INSERT INTO gatehouse_session_note_revisions (
					workspace, session, note, revision, author_principal, author_agent, author_gateway,
					title, description, body, sensitive, created_at
				)
				SELECT workspace, session, id, revision, author_principal, author_agent, author_gateway,
					title, description, body, sensitive, created_at
				FROM gatehouse_session_notes;

				ALTER TABLE gatehouse_project_notes DROP COLUMN sensitive;
				ALTER TABLE gatehouse_session_notes DROP COLUMN sensitive;

				CREATE TRIGGER gatehouse_project_note_revisions_immutable_update
				BEFORE UPDATE ON gatehouse_project_note_revisions
				BEGIN SELECT RAISE(ABORT, 'project note revisions are immutable'); END;

				CREATE TRIGGER gatehouse_project_note_revisions_immutable_delete
				BEFORE DELETE ON gatehouse_project_note_revisions
				BEGIN SELECT RAISE(ABORT, 'project note revisions are immutable'); END;

				CREATE TRIGGER gatehouse_session_note_revisions_immutable_update
				BEFORE UPDATE ON gatehouse_session_note_revisions
				BEGIN SELECT RAISE(ABORT, 'session note revisions are immutable'); END;

				CREATE TRIGGER gatehouse_session_note_revisions_immutable_delete
				BEFORE DELETE ON gatehouse_session_note_revisions
				BEGIN SELECT RAISE(ABORT, 'session note revisions are immutable'); END;
			`),
	}
}
