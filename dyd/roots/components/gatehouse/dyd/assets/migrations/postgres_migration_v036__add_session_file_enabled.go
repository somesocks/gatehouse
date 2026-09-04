package migrations

func postgresMigrationV036AddSessionFileEnabled() VersionedMigration {
	return VersionedMigration{
		Index:       36,
		Description: "add_session_file_enabled",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_session_files ADD COLUMN enabled BOOLEAN;
			UPDATE gatehouse_session_files SET enabled = TRUE WHERE enabled IS NULL;
			ALTER TABLE gatehouse_session_files ALTER COLUMN enabled SET NOT NULL;
		`),
	}
}
