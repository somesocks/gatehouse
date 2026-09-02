package migrations

func postgresMigrationV027DropLegacyWorkspaceToolsAndResources() VersionedMigration {
	return VersionedMigration{
		Index:       27,
		Description: "drop_legacy_workspace_tools_and_resources",
		Builder: staticMigrationBuilder(`
				DROP TABLE IF EXISTS gatehouse_group_tool_grants;
				DROP TABLE IF EXISTS gatehouse_group_resource_grants;
				DROP TABLE IF EXISTS gatehouse_tools;
				DROP TABLE IF EXISTS gatehouse_resources;
			`),
	}
}
