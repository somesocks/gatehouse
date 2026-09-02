package migrations

func postgresMigrationV019AddProjectNoteSensitivity() VersionedMigration {
	return VersionedMigration{
		Index:       19,
		Description: "add_project_note_sensitivity",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_project_notes
				ADD COLUMN sensitive BOOLEAN NOT NULL DEFAULT FALSE;
			`),
	}
}
