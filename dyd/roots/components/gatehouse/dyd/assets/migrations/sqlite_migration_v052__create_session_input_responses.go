package migrations

func sqliteMigrationV052CreateSessionInputResponses() VersionedMigration {
	return VersionedMigration{
		Index:       52,
		Description: "create_session_input_responses",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_input_responses (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				input TEXT NOT NULL,
				response TEXT NOT NULL,
				created_at TEXT NOT NULL,
				delivered INTEGER NOT NULL CHECK (delivered IN (0, 1)),
				PRIMARY KEY (workspace, session, input),
				FOREIGN KEY (workspace, session, input)
					REFERENCES gatehouse_session_events (workspace, session, id),
				FOREIGN KEY (workspace, session, response)
					REFERENCES gatehouse_session_events (workspace, session, id)
			) STRICT;
			CREATE INDEX gatehouse_session_input_responses_pending
				ON gatehouse_session_input_responses (delivered, created_at, response);
		`),
	}
}
