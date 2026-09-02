package migrations

func postgresMigrationV021AddAgentModelCompaction() VersionedMigration {
	return VersionedMigration{
		Index:       21,
		Description: "add_agent_model_compaction",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_agent_models
				ADD COLUMN compaction JSONB NOT NULL DEFAULT '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":16384}'::JSONB
					CHECK (jsonb_typeof(compaction) = 'object');
			`),
	}
}
