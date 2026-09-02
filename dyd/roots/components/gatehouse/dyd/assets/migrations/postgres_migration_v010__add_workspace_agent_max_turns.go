package migrations

func postgresMigrationV010AddWorkspaceAgentMaxTurns() VersionedMigration {
	return VersionedMigration{
		Index:       10,
		Description: "add_workspace_agent_max_turns",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN max_turns BIGINT NOT NULL DEFAULT 127 CHECK (max_turns > 0);
		`),
	}
}
