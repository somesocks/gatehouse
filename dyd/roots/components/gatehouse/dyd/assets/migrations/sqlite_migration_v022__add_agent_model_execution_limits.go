package migrations

func sqliteMigrationV022AddAgentModelExecutionLimits() VersionedMigration {
	return VersionedMigration{
		Index:       22,
		Description: "add_agent_model_execution_limits",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_agent_models
				ADD COLUMN max_turns INTEGER NOT NULL DEFAULT 127 CHECK (max_turns > 0);

				ALTER TABLE gatehouse_agent_models
				ADD COLUMN max_output_tokens INTEGER NOT NULL DEFAULT 16000 CHECK (max_output_tokens > 0);
			`),
	}
}
