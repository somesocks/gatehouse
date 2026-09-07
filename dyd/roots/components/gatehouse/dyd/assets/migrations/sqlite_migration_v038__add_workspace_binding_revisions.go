package migrations

func sqliteMigrationV038AddWorkspaceBindingRevisions() VersionedMigration {
	return VersionedMigration{
		Index:       38,
		Description: "add_workspace_binding_revisions",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
			ALTER TABLE gatehouse_workspace_storage_providers
				ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
		`),
	}
}
