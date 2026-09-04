package migrations

func sqliteMigrationV036AddSessionFileEnabled() VersionedMigration {
	return VersionedMigration{
		Index:       36,
		Description: "add_session_file_enabled",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_session_files
			ADD COLUMN enabled INTEGER NOT NULL DEFAULT TRUE CHECK (enabled IN (0, 1));
		`),
	}
}
