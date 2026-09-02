package migrations

func sqliteMigrationV009CreateSessionEventReplyTasks() VersionedMigration {
	return VersionedMigration{
		Index:       9,
		Description: "create_session_event_reply_tasks",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_tasks__session_event_reply (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				event TEXT NOT NULL,
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, event),
				FOREIGN KEY (workspace, session, event)
					REFERENCES gatehouse_session_events (workspace, session, id)
			) STRICT;

			CREATE TABLE gatehouse_agent_tasks__session_name (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;
		`),
	}
}
