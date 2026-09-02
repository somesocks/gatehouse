package migrations

func postgresMigrationV024IncreaseDefaultAgentModelContextWindow() VersionedMigration {
	return VersionedMigration{
		Index:       24,
		Description: "increase_default_agent_model_context_window",
		Builder: staticMigrationBuilder(`
				UPDATE gatehouse_agent_models
				SET compaction = '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":16384}'::JSONB
				WHERE compaction = '{"algorithm":"mcmtr","history_bytes":81920,"buffer_bytes":16384}'::JSONB;
			`),
	}
}
