package migrations

func sqliteMigrationV017CreateSessionApprovalDecisions() VersionedMigration {
	return VersionedMigration{
		Index:       17,
		Description: "create_session_approval_decisions",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_session_approval_decisions (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					approval TEXT NOT NULL,
					response TEXT NOT NULL,
					created_at TEXT NOT NULL,
					delivered INTEGER NOT NULL CHECK (delivered IN (0, 1)),
					PRIMARY KEY (workspace, session, approval),
					FOREIGN KEY (workspace, session, approval)
						REFERENCES gatehouse_session_events (workspace, session, id),
					FOREIGN KEY (workspace, session, response)
						REFERENCES gatehouse_session_events (workspace, session, id)
				) STRICT;

				CREATE INDEX gatehouse_session_approval_decisions_pending
				ON gatehouse_session_approval_decisions (delivered, created_at, response);
		`),
	}
}
