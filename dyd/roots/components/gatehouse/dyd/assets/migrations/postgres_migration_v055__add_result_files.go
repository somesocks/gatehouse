package migrations

func postgresMigrationV055AddResultFiles() VersionedMigration {
	return VersionedMigration{
		Index:       55,
		Description: "add_result_files",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_agent_models ADD COLUMN result_inline_bytes INTEGER NOT NULL DEFAULT 4096 CHECK (result_inline_bytes > 0);
			ALTER TABLE gatehouse_session_files
				ADD COLUMN author_principal TEXT REFERENCES gatehouse_principals (id),
				ADD COLUMN author_agent TEXT,
				ADD CONSTRAINT gatehouse_session_files_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				ADD CONSTRAINT gatehouse_session_files_author_check CHECK (author_agent IS NULL OR author_principal IS NULL);
			CREATE INDEX gatehouse_session_files_author ON gatehouse_session_files (workspace, session, author_agent, created_at, id);
		`),
	}
}
