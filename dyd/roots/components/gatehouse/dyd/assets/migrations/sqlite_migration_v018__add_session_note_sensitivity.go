package migrations

func sqliteMigrationV018AddSessionNoteSensitivity() VersionedMigration {
	return VersionedMigration{
		Index:       18,
		Description: "add_session_note_sensitivity",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_session_notes
				ADD COLUMN sensitive INTEGER NOT NULL DEFAULT FALSE CHECK (sensitive IN (0, 1));
			`),
	}
}
