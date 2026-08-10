package migrations

import (
	"context"
	"crypto/rand"
	"fmt"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/keychain"
	"gatehouse/model"
)

type agentProviderMigrationValue struct {
	ID, Protocol, BaseURL, APIKey string
	Revision                       int
	Keychain                       model.KeychainRef
	Enabled                        bool
}

func agentProviderMigrationBuilder(providers []config.AgentProvider, keyring *keychain.Keyring) MigrationBuilder {
	resolver := agent.NewAPIKeySourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		revisions, err := migrationAgentProviderRevisions(ctx, session)
		if err != nil { return err, "" }
		values := make([]agentProviderMigrationValue, 0, len(providers))
		for _, provider := range providers {
			if revisions[provider.ID] >= provider.Revision { continue }
			apiKeyErr, apiKey := resolver.Resolve(provider.ID, provider.Sources)
			if apiKeyErr != nil { return apiKeyErr, "" }
			deksErr, deks := keyring.Get(ctx, []model.KeychainRef{{Id: provider.Keychain, Version: 1}})
			if deksErr != nil { clear(apiKey); return fmt.Errorf("get keychain for provider %q: %w", provider.ID, deksErr), "" }
			reference := model.KeychainRef{Id: provider.Keychain, Version: 1}
			sealErr, encrypted := keychain.Seal(rand.Reader, deks[reference], []byte("gh=v1|agent-provider="+provider.ID), apiKey)
			clear(apiKey)
			clear(deks[reference])
			if sealErr != nil { return fmt.Errorf("encrypt API key for provider %q: %w", provider.ID, sealErr), "" }
			values = append(values, agentProviderMigrationValue{ID: provider.ID, Revision: provider.Revision, Protocol: provider.Protocol, BaseURL: provider.BaseURL, Keychain: reference, APIKey: encrypted.String(), Enabled: provider.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_agent_providers (id, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Protocol }}, {{ sqlLiteral .BaseURL }}, {{ sqlLiteral .Keychain.Id }}, {{ sqlLiteral .Keychain.Version }}, {{ sqlLiteral .APIKey }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET revision = excluded.revision, protocol = excluded.protocol, base_url = excluded.base_url, keychain_id = excluded.keychain_id, keychain_version = excluded.keychain_version, api_key = excluded.api_key, enabled = excluded.enabled
			WHERE gatehouse_agent_providers.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

func agentModelMigrationBuilder(models []config.AgentModel) MigrationBuilder {
	return templateMigrationBuilder(`
		SELECT 1;
		{{ range . }}
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled)
		VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Provider }}, {{ sqlLiteral .Model }}, {{ sqlLiteral .Parameters }}, {{ sqlBool .Enabled }})
		ON CONFLICT (id) DO UPDATE SET revision = excluded.revision, provider_id = excluded.provider_id, model = excluded.model, parameters = excluded.parameters, enabled = excluded.enabled
		WHERE gatehouse_agent_models.revision < excluded.revision;
		{{ end }}
	`, models)
}

func workspaceAgentMigrationBuilder(agents []config.WorkspaceAgent) MigrationBuilder {
	return templateMigrationBuilder(`
		SELECT 1;
		{{ range . }}
		INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, priority, enabled)
		VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Model }}, {{ sqlLiteral .Priority }}, {{ sqlBool .Enabled }})
		ON CONFLICT (workspace_id, model_id) DO UPDATE SET priority = excluded.priority, enabled = excluded.enabled;
		{{ end }}
	`, agents)
}

func migrationAgentProviderRevisions(ctx context.Context, session *MigrationSession) (map[string]int, error) {
	rows, err := session.QueryContext(ctx, `SELECT id, revision FROM gatehouse_agent_providers`)
	if err != nil { return nil, fmt.Errorf("get agent provider revisions: %w", err) }
	defer rows.Close()
	revisions := map[string]int{}
	for rows.Next() {
		var id string; var revision int
		if err := rows.Scan(&id, &revision); err != nil { return nil, fmt.Errorf("read agent provider revision: %w", err) }
		revisions[id] = revision
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("read agent provider revisions: %w", err) }
	return revisions, nil
}
