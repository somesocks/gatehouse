package migrations

func sqliteMigrationV013CreateStorageProviders() VersionedMigration {
	return VersionedMigration{
		Index:       13,
		Description: "create_storage_providers",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_storage_providers (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'stp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				revision INTEGER NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('embedded', 's3')),
				endpoint TEXT,
				region TEXT,
				bucket TEXT,
				access_key_id TEXT,
				keychain_id TEXT,
				keychain_version INTEGER,
				secret_access_key TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'embedded' AND endpoint IS NULL AND region IS NULL AND bucket IS NULL AND access_key_id IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND secret_access_key IS NULL)
					OR (protocol = 's3' AND endpoint IS NOT NULL AND length(trim(endpoint)) > 0 AND region IS NOT NULL AND length(trim(region)) > 0 AND bucket IS NOT NULL AND length(trim(bucket)) > 0 AND access_key_id IS NOT NULL AND length(trim(access_key_id)) > 0 AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND keychain_version > 0 AND secret_access_key IS NOT NULL AND length(trim(secret_access_key)) > 0)
				)
			) STRICT;

			CREATE TABLE gatehouse_workspace_storage_providers (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				priority INTEGER NOT NULL CHECK (priority > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace, provider)
			) STRICT;
		`),
	}
}
