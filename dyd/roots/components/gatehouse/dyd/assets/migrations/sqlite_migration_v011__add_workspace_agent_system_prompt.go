package migrations

func sqliteMigrationV011AddWorkspaceAgentSystemPrompt() VersionedMigration {
	return VersionedMigration{
		Index:       11,
		Description: "add_workspace_agent_system_prompt",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN system_prompt TEXT;
		`),
	}
}
