package migrations

func postgresMigrationV058GroupAdministrationRevisions() VersionedMigration {
	return VersionedMigration{Index: 58, Description: "group_administration_revisions", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_groups ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);
		ALTER TABLE gatehouse_group_members ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);
	`)}
}
