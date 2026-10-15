package migrations

func sqliteMigrationV055AddResultFiles() VersionedMigration {
	return VersionedMigration{
		Index:       55,
		Description: "add_result_files",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_agent_models ADD COLUMN result_inline_bytes INTEGER NOT NULL DEFAULT 4096 CHECK (result_inline_bytes > 0);
			ALTER TABLE gatehouse_session_files ADD COLUMN author_principal TEXT REFERENCES gatehouse_principals (id);
			ALTER TABLE gatehouse_session_files ADD COLUMN author_agent TEXT REFERENCES gatehouse_workspace_agents (id)
				CHECK (author_agent IS NULL OR author_principal IS NULL);
			CREATE INDEX gatehouse_session_files_author ON gatehouse_session_files (workspace, session, author_agent, created_at, id);
			CREATE TRIGGER gatehouse_session_files_author_insert BEFORE INSERT ON gatehouse_session_files
			WHEN NEW.author_agent IS NOT NULL AND NOT EXISTS (SELECT 1 FROM gatehouse_workspace_agents WHERE workspace_id = NEW.workspace AND id = NEW.author_agent)
			BEGIN SELECT RAISE(ABORT, 'session file agent author belongs to another workspace'); END;
			CREATE TRIGGER gatehouse_session_files_author_update BEFORE UPDATE OF workspace, author_agent ON gatehouse_session_files
			WHEN NEW.author_agent IS NOT NULL AND NOT EXISTS (SELECT 1 FROM gatehouse_workspace_agents WHERE workspace_id = NEW.workspace AND id = NEW.author_agent)
			BEGIN SELECT RAISE(ABORT, 'session file agent author belongs to another workspace'); END;
		`),
	}
}
