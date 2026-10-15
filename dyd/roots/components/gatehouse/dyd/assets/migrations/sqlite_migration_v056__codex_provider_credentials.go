package migrations

func sqliteMigrationV056CodexProviderCredentials() VersionedMigration {
	return VersionedMigration{
		Index: 56, Description: "codex_provider_credentials",
		Options: MigrationOptions{SQLite: SQLiteMigrationOptions{DisableForeignKeys: true}},
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_agent_providers_migration_56 (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'apr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias GLOB '[a-z]*') CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				revision INTEGER NOT NULL CHECK (revision > 0),
				protocol TEXT NOT NULL CHECK (protocol IN ('builtin', 'openai-chat-completions', 'openai-responses', 'openai-codex')),
				endpoint TEXT CHECK (endpoint IS NULL OR length(trim(endpoint)) > 0),
				keychain_id TEXT,
				keychain_version INTEGER CHECK (keychain_version IS NULL OR keychain_version > 0),
				credential TEXT CHECK (credential IS NULL OR length(trim(credential)) > 0),
				credential_metadata TEXT CHECK (credential_metadata IS NULL OR (json_valid(credential_metadata) AND json_type(credential_metadata) = 'object')),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				FOREIGN KEY (keychain_id, keychain_version) REFERENCES gatehouse_keychains (id, version),
				CHECK (
					(protocol = 'builtin' AND endpoint IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND credential IS NULL)
					OR (protocol IN ('openai-chat-completions', 'openai-responses') AND endpoint IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND credential IS NOT NULL)
					OR (protocol = 'openai-codex' AND endpoint = 'https://chatgpt.com/backend-api/codex/responses' AND endpoint IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL)
				),
				CHECK (credential_metadata IS NULL OR (protocol = 'openai-codex' AND credential IS NOT NULL))
			) STRICT;
			INSERT INTO gatehouse_agent_providers_migration_56 (id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential, enabled)
			SELECT id, alias, revision, protocol, endpoint, keychain_id, keychain_version, api_key, enabled FROM gatehouse_agent_providers;
			DROP TABLE gatehouse_agent_providers;
			ALTER TABLE gatehouse_agent_providers_migration_56 RENAME TO gatehouse_agent_providers;
		`),
	}
}
