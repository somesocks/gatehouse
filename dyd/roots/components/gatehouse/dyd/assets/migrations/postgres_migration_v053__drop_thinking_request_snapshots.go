package migrations

func postgresMigrationV053DropThinkingRequestSnapshots() VersionedMigration {
	return VersionedMigration{
		Index:       53,
		Description: "drop_thinking_request_snapshots",
		Builder: staticMigrationBuilder(`
			UPDATE gatehouse_session_events
			SET payload = payload - 'provider_request' - 'round_input'
			WHERE kind = 'thinking.request'
				AND (payload ? 'provider_request' OR payload ? 'round_input');
		`),
	}
}
