package migrations

func postgresMigrationV005CreateKeychains() VersionedMigration {
	return VersionedMigration{
		Index:       5,
		Description: "create_keychains",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_keychains (
				id TEXT NOT NULL
					CHECK (id ~ '^[a-z][a-z0-9_-]*$'),
				version BIGINT NOT NULL CHECK (version > 0),
				kek_kdf TEXT NOT NULL CHECK (length(trim(kek_kdf)) > 0),
				key TEXT NOT NULL CHECK (length(trim(key)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (id, version)
			);
		`),
	}
}
