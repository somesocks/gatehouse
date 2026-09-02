package migrations

func postgresMigrationV017CreateSessionApprovalDecisions() VersionedMigration {
	return VersionedMigration{
		Index:       17,
		Description: "create_session_approval_decisions",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_session_approval_decisions (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					approval TEXT NOT NULL,
					response TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					delivered BOOLEAN NOT NULL,
					PRIMARY KEY (workspace, session, approval),
					FOREIGN KEY (workspace, session, approval)
						REFERENCES gatehouse_session_events (workspace, session, id),
					FOREIGN KEY (workspace, session, response)
						REFERENCES gatehouse_session_events (workspace, session, id)
				);

				CREATE INDEX gatehouse_session_approval_decisions_pending
				ON gatehouse_session_approval_decisions (delivered, created_at, response);
		`),
	}
}
