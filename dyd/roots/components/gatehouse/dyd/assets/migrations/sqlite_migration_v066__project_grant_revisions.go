package migrations

func sqliteMigrationV066ProjectGrantRevisions() VersionedMigration {
	return VersionedMigration{Index: 66, Description: "project_grant_revisions", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_project_grants ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
	`)}
}
