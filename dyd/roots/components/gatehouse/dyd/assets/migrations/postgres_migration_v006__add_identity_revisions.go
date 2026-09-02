package migrations

func postgresMigrationV006AddIdentityRevisions() VersionedMigration {
	return VersionedMigration{
		Index:       6,
		Description: "add_identity_revisions",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_identities
			ADD COLUMN revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0);
		`),
	}
}
