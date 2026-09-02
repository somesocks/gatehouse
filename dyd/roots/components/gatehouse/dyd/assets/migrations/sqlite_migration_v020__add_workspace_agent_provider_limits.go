package migrations

func sqliteMigrationV020AddWorkspaceAgentProviderLimits() VersionedMigration {
	return VersionedMigration{
		Index:       20,
		Description: "add_workspace_agent_provider_limits",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN max_input_tokens INTEGER NOT NULL DEFAULT 120000 CHECK (max_input_tokens > 0);

				ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN max_output_tokens INTEGER NOT NULL DEFAULT 16000 CHECK (max_output_tokens > 0);
			`),
	}
}
