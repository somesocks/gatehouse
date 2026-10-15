package migrations

func postgresMigrationV056CodexProviderCredentials() VersionedMigration {
	return VersionedMigration{Index: 56, Description: "codex_provider_credentials", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_agent_providers RENAME COLUMN api_key TO credential;
		ALTER TABLE gatehouse_agent_providers ADD COLUMN credential_metadata TEXT
			CHECK (credential_metadata IS NULL OR jsonb_typeof(credential_metadata::jsonb) = 'object');
		ALTER TABLE gatehouse_agent_providers DROP CONSTRAINT gatehouse_agent_providers_protocol_check;
		ALTER TABLE gatehouse_agent_providers DROP CONSTRAINT gatehouse_agent_providers_check;
		ALTER TABLE gatehouse_agent_providers ADD CONSTRAINT gatehouse_agent_providers_protocol_check
			CHECK (protocol IN ('builtin', 'openai-chat-completions', 'openai-responses', 'openai-codex'));
		ALTER TABLE gatehouse_agent_providers ADD CONSTRAINT gatehouse_agent_providers_check CHECK (
			(protocol = 'builtin' AND endpoint IS NULL AND keychain_id IS NULL AND keychain_version IS NULL AND credential IS NULL)
			OR (protocol IN ('openai-chat-completions', 'openai-responses') AND endpoint IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL AND credential IS NOT NULL)
			OR (protocol = 'openai-codex' AND endpoint = 'https://chatgpt.com/backend-api/codex/responses' AND endpoint IS NOT NULL AND keychain_id IS NOT NULL AND keychain_version IS NOT NULL)
		);
		ALTER TABLE gatehouse_agent_providers ADD CONSTRAINT gatehouse_agent_providers_metadata_check
			CHECK (credential_metadata IS NULL OR (protocol = 'openai-codex' AND credential IS NOT NULL));
	`)}
}
