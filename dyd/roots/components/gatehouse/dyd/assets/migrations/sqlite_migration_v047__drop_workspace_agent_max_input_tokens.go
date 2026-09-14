package migrations

func sqliteMigrationV047DropWorkspaceAgentMaxInputTokens() VersionedMigration {
	return VersionedMigration{
		Index:       47,
		Description: "drop_workspace_agent_max_input_tokens",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents DROP COLUMN max_input_tokens;
		`),
	}
}
