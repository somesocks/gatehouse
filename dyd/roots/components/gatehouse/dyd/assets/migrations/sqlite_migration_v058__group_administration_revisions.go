package migrations

func sqliteMigrationV058GroupAdministrationRevisions() VersionedMigration {
	return VersionedMigration{Index: 58, Description: "group_administration_revisions", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_groups ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
		ALTER TABLE gatehouse_group_members ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
	`)}
}
