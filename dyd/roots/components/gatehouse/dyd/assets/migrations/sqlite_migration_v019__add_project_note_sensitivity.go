package migrations

func sqliteMigrationV019AddProjectNoteSensitivity() VersionedMigration {
	return VersionedMigration{
		Index:       19,
		Description: "add_project_note_sensitivity",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_project_notes
				ADD COLUMN sensitive INTEGER NOT NULL DEFAULT FALSE CHECK (sensitive IN (0, 1));
			`),
	}
}
