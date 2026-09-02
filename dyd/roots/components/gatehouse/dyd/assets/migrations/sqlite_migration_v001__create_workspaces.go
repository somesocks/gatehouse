package migrations

func sqliteMigrationV001CreateWorkspaces() VersionedMigration {
	return VersionedMigration{
		Index:       1,
		Description: "create_workspaces",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_workspaces (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wsp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias GLOB '[a-z]*')
					CHECK (alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;
		`),
	}
}
