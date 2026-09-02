package migrations

func postgresMigrationV018AddSessionNoteSensitivity() VersionedMigration {
	return VersionedMigration{
		Index:       18,
		Description: "add_session_note_sensitivity",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_session_notes
				ADD COLUMN sensitive BOOLEAN NOT NULL DEFAULT FALSE;
			`),
	}
}
