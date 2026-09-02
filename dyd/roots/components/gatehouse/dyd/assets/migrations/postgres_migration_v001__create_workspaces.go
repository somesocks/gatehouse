package migrations

func postgresMigrationV001CreateWorkspaces() VersionedMigration {
	return VersionedMigration{
		Index:       1,
		Description: "create_workspaces",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_workspaces (
				id TEXT PRIMARY KEY CHECK (id ~ '^wsp_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);
		`),
	}
}
