package migrations

func postgresMigrationV052CreateSessionInputResponses() VersionedMigration {
	return VersionedMigration{
		Index:       52,
		Description: "create_session_input_responses",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_input_responses (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				input TEXT NOT NULL,
				response TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				delivered BOOLEAN NOT NULL,
				PRIMARY KEY (workspace, session, input),
				FOREIGN KEY (workspace, session, input)
					REFERENCES gatehouse_session_events (workspace, session, id),
				FOREIGN KEY (workspace, session, response)
					REFERENCES gatehouse_session_events (workspace, session, id)
			);
			CREATE INDEX gatehouse_session_input_responses_pending
				ON gatehouse_session_input_responses (delivered, created_at, response);
		`),
	}
}
