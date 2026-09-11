package migrations

func postgresMigrationV045DropReplyAuthorizationSnapshots() VersionedMigration {
	return VersionedMigration{
		Index:       45,
		Description: "drop_reply_authorization_snapshots",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_agent_tasks__session_event_reply
				DROP COLUMN request_principal,
				DROP COLUMN request_project,
				DROP COLUMN request_project_actions;
		`),
	}
}
