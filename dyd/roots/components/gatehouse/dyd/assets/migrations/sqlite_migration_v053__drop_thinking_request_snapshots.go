package migrations

func sqliteMigrationV053DropThinkingRequestSnapshots() VersionedMigration {
	return VersionedMigration{
		Index:       53,
		Description: "drop_thinking_request_snapshots",
		Builder: staticMigrationBuilder(`
			UPDATE gatehouse_session_events
			SET payload = json_remove(payload, '$.provider_request', '$.round_input')
			WHERE kind = 'thinking.request'
				AND (json_type(payload, '$.provider_request') IS NOT NULL
					OR json_type(payload, '$.round_input') IS NOT NULL);
		`),
	}
}
