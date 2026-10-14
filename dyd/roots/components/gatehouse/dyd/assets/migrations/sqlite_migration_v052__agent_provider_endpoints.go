package migrations

func sqliteMigrationV052AgentProviderEndpoints() VersionedMigration {
	return VersionedMigration{
		Index:       52,
		Description: "agent_provider_endpoints",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_agent_providers RENAME COLUMN base_url TO endpoint;
			WITH boundaries AS (
				SELECT id, endpoint, protocol,
					CASE
						WHEN instr(endpoint, '?') = 0 AND instr(endpoint, '#') = 0 THEN length(endpoint) + 1
						WHEN instr(endpoint, '?') = 0 THEN instr(endpoint, '#')
						WHEN instr(endpoint, '#') = 0 THEN instr(endpoint, '?')
						WHEN instr(endpoint, '?') < instr(endpoint, '#') THEN instr(endpoint, '?')
						ELSE instr(endpoint, '#')
					END AS suffix_at
				FROM gatehouse_agent_providers WHERE protocol <> 'builtin'
			), converted AS (
				SELECT id, rtrim(substr(endpoint, 1, suffix_at - 1), '/') ||
					CASE protocol WHEN 'openai-responses' THEN '/responses' ELSE '/chat/completions' END ||
					substr(endpoint, suffix_at) AS endpoint
				FROM boundaries
			)
			UPDATE gatehouse_agent_providers
			SET endpoint = (SELECT endpoint FROM converted WHERE converted.id = gatehouse_agent_providers.id)
			WHERE id IN (SELECT id FROM converted);
		`),
	}
}
