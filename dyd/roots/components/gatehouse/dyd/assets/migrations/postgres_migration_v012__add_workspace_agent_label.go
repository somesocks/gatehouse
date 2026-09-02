package migrations

func postgresMigrationV012AddWorkspaceAgentLabel() VersionedMigration {
	return VersionedMigration{
		Index:       12,
		Description: "add_workspace_agent_label",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
			ADD COLUMN label TEXT;
		`),
	}
}
