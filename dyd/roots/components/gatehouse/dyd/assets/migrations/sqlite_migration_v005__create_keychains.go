package migrations

func sqliteMigrationV005CreateKeychains() VersionedMigration {
	return VersionedMigration{
		Index:       5,
		Description: "create_keychains",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_keychains (
				id TEXT NOT NULL
					CHECK (id GLOB '[a-z]*')
					CHECK (id NOT GLOB '*[^a-z0-9_-]*'),
				version INTEGER NOT NULL CHECK (version > 0),
				kek_kdf TEXT NOT NULL CHECK (length(trim(kek_kdf)) > 0),
				key TEXT NOT NULL CHECK (length(trim(key)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (id, version)
			) STRICT;
		`),
	}
}
