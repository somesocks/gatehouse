package migrations

func sqliteMigrationV045DropReplyAuthorizationSnapshots() VersionedMigration {
	return VersionedMigration{
		Index:       45,
		Description: "drop_reply_authorization_snapshots",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_tasks__session_event_reply_migration_45 (
				workspace TEXT NOT NULL, session TEXT NOT NULL, event TEXT NOT NULL, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, event),
				FOREIGN KEY (workspace, session, event) REFERENCES gatehouse_session_events (workspace, session, id)
			) STRICT;
			INSERT INTO gatehouse_agent_tasks__session_event_reply_migration_45 (workspace, session, event, created_at)
			SELECT workspace, session, event, created_at FROM gatehouse_agent_tasks__session_event_reply;
			DROP TABLE gatehouse_agent_tasks__session_event_reply;
			ALTER TABLE gatehouse_agent_tasks__session_event_reply_migration_45 RENAME TO gatehouse_agent_tasks__session_event_reply;
		`),
	}
}
