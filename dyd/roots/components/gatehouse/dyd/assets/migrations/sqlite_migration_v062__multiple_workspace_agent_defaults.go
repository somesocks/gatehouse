package migrations

func sqliteMigrationV062MultipleWorkspaceAgentDefaults() VersionedMigration {
	return VersionedMigration{
		Index:       62,
		Description: "multiple_workspace_agent_defaults",
		Builder: staticMigrationBuilder(`
			DROP INDEX gatehouse_workspace_agents_default_once;
		`),
	}
}
