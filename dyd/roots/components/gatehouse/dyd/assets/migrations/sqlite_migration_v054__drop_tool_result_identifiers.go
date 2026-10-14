package migrations

func sqliteMigrationV054DropToolResultIdentifiers() VersionedMigration {
	return VersionedMigration{
		Index:       54,
		Description: "drop_tool_result_identifiers",
		Builder: staticMigrationBuilder(`
			UPDATE gatehouse_session_events
			SET payload = json_remove(payload, '$.name', '$.call_id')
			WHERE kind IN ('tool.success', 'tool.failure')
				AND (json_type(payload, '$.name') IS NOT NULL
					OR json_type(payload, '$.call_id') IS NOT NULL);
		`),
	}
}
