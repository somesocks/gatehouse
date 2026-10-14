package migrations

func postgresMigrationV054DropToolResultIdentifiers() VersionedMigration {
	return VersionedMigration{
		Index:       54,
		Description: "drop_tool_result_identifiers",
		Builder: staticMigrationBuilder(`
			UPDATE gatehouse_session_events
			SET payload = payload - 'name' - 'call_id'
			WHERE kind IN ('tool.success', 'tool.failure')
				AND (payload ? 'name' OR payload ? 'call_id');
		`),
	}
}
