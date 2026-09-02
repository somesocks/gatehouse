package migrations

func sqliteMigrationV021AddAgentModelCompaction() VersionedMigration {
	return VersionedMigration{
		Index:       21,
		Description: "add_agent_model_compaction",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_agent_models
				ADD COLUMN compaction TEXT NOT NULL DEFAULT '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":16384}'
					CHECK (json_valid(compaction)) CHECK (json_type(compaction) = 'object');
			`),
	}
}
