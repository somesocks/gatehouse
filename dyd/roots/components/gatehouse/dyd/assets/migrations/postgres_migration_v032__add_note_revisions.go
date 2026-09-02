package migrations

func postgresMigrationV032AddNoteRevisions() VersionedMigration {
	return VersionedMigration{
		Index:       32,
		Description: "add_note_revisions",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_project_notes
					ALTER COLUMN author_principal DROP NOT NULL,
					ADD COLUMN author_agent TEXT,
					ADD COLUMN author_gateway TEXT CHECK (author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
					ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
					ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					ADD CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					);

				ALTER TABLE gatehouse_session_notes
					ALTER COLUMN author_principal DROP NOT NULL,
					ADD COLUMN author_agent TEXT,
					ADD COLUMN author_gateway TEXT CHECK (author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
					ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
					ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					ADD CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					);

				CREATE TABLE gatehouse_project_note_revisions (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					note TEXT NOT NULL,
					revision BIGINT NOT NULL CHECK (revision > 0),
					author_principal TEXT REFERENCES gatehouse_principals (id),
					author_agent TEXT,
					author_gateway TEXT CHECK (author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
					title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (octet_length(title) <= 256),
					description TEXT NOT NULL CHECK (octet_length(description) <= 4096),
					body TEXT NOT NULL CHECK (octet_length(body) <= 1048576),
					sensitive BOOLEAN NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					PRIMARY KEY (workspace, project, note, revision),
					FOREIGN KEY (workspace, project, note) REFERENCES gatehouse_project_notes (workspace, project, id),
					FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					)
				);

				CREATE TABLE gatehouse_session_note_revisions (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					note TEXT NOT NULL,
					revision BIGINT NOT NULL CHECK (revision > 0),
					author_principal TEXT REFERENCES gatehouse_principals (id),
					author_agent TEXT,
					author_gateway TEXT CHECK (author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
					title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (octet_length(title) <= 256),
					description TEXT NOT NULL CHECK (octet_length(description) <= 4096),
					body TEXT NOT NULL CHECK (octet_length(body) <= 1048576),
					sensitive BOOLEAN NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					PRIMARY KEY (workspace, session, note, revision),
					FOREIGN KEY (workspace, session, note) REFERENCES gatehouse_session_notes (workspace, session, id),
					FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(author_principal IS NOT NULL AND author_agent IS NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NOT NULL AND author_gateway IS NULL)
						OR (author_principal IS NULL AND author_agent IS NULL AND author_gateway IS NOT NULL)
					)
				);

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

				CREATE FUNCTION gatehouse_note_revision_immutable() RETURNS trigger AS $$
				BEGIN
					RAISE EXCEPTION 'note revisions are immutable';
				END;
				$$ LANGUAGE plpgsql;

				CREATE TRIGGER gatehouse_project_note_revisions_immutable
				BEFORE UPDATE OR DELETE ON gatehouse_project_note_revisions
				FOR EACH ROW EXECUTE FUNCTION gatehouse_note_revision_immutable();

				CREATE TRIGGER gatehouse_session_note_revisions_immutable
				BEFORE UPDATE OR DELETE ON gatehouse_session_note_revisions
				FOR EACH ROW EXECUTE FUNCTION gatehouse_note_revision_immutable();
			`),
	}
}
