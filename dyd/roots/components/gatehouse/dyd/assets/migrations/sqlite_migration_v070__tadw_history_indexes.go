package migrations

func sqliteMigrationV070TADWHistoryIndexes() VersionedMigration {
	return VersionedMigration{
		Index: 70, Description: "tadw_history_indexes",
		Builder: staticMigrationBuilder(`
			CREATE INDEX gatehouse_session_events_thread_order ON gatehouse_session_events
				(workspace, session, parent, created_at DESC, id DESC);
			CREATE INDEX gatehouse_session_events_agent_terminals ON gatehouse_session_events
				(workspace, session, parent, kind, created_at, id)
				WHERE kind IN ('agent.success', 'agent.failure');
		`),
	}
}
