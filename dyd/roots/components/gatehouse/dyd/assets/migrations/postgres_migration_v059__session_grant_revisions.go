package migrations

func postgresMigrationV059SessionGrantRevisions() VersionedMigration {
	return VersionedMigration{Index: 59, Description: "session_grant_revisions", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_session_grants ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);
	`)}
}
