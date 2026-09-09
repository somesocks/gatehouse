package migrations

func sqliteMigrationV040DropWorkspaceAgentMaxTurns() VersionedMigration {
	return VersionedMigration{
		Index:       40,
		Description: "drop_workspace_agent_max_turns",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents DROP COLUMN max_turns;
		`),
	}
}
