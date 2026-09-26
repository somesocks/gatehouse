package migrations

func sqliteMigrationV051CreateSessionInputDrafts() VersionedMigration {
	return VersionedMigration{
		Index:       51,
		Description: "create_session_input_drafts",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_input_drafts (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				input TEXT NOT NULL,
				draft TEXT NOT NULL CHECK (json_valid(draft)) CHECK (json_type(draft) = 'object'),
				updated_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, input),
				FOREIGN KEY (workspace, session, input)
					REFERENCES gatehouse_session_events (workspace, session, id)
			) STRICT;
		`),
	}
}
