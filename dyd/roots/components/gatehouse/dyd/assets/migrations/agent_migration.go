package migrations

import (
	"context"
	"crypto/rand"
	"fmt"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type agentProviderMigrationValue struct {
	ID, Alias, Protocol string
	BaseURL         any
	APIKey          any
	KeychainID      any
	KeychainVersion any
	Revision        int
	Enabled         bool
}

func agentProviderMigrationBuilder(providers []config.AgentProvider, keyring *keychain.Keyring) MigrationBuilder {
	resolver := agent.NewAPIKeySourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := agentProviderIDsAndRevisionsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]agentProviderMigrationValue, 0, len(providers))
		for _, provider := range providers {
			previous := existing[provider.Alias]
			if previous.Revision >= provider.Revision {
				continue
			}
			id := previous.ID
			if id == "" {
				id, err = typed_id.New(typed_id.AgentProvider)
				if err != nil {
					return err, ""
				}
			}
			value := agentProviderMigrationValue{ID: id, Alias: provider.Alias, Revision: provider.Revision, Protocol: provider.Protocol, Enabled: provider.Enabled}
			if provider.Protocol == "builtin" {
				values = append(values, value)
				continue
			}
			apiKeyErr, apiKey := resolver.Resolve(provider.Alias, provider.Sources)
			if apiKeyErr != nil {
				return apiKeyErr, ""
			}
			reference := model.KeychainRef{Id: *provider.Keychain, Version: 1}
			keyringErr, keys := keyring.Get(ctx, []model.KeychainRef{reference})
			if keyringErr != nil {
				clear(apiKey)
				return fmt.Errorf("get keychain for provider %q: %w", provider.Alias, keyringErr), ""
			}
			sealErr, encrypted := keychain.Seal(rand.Reader, keys[reference], []byte("gh=v1|agent-provider="+id), apiKey)
			clear(apiKey)
			clear(keys[reference])
			if sealErr != nil {
				return fmt.Errorf("encrypt API key for provider %q: %w", provider.Alias, sealErr), ""
			}
			value.BaseURL = *provider.BaseURL
			value.KeychainID = reference.Id
			value.KeychainVersion = reference.Version
			value.APIKey = encrypted.String()
			values = append(values, value)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Protocol }}, {{ sqlLiteral .BaseURL }}, {{ sqlLiteral .KeychainID }}, {{ sqlLiteral .KeychainVersion }}, {{ sqlLiteral .APIKey }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, protocol = excluded.protocol, base_url = excluded.base_url, keychain_id = excluded.keychain_id, keychain_version = excluded.keychain_version, api_key = excluded.api_key, enabled = excluded.enabled
			WHERE gatehouse_agent_providers.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

func agentModelMigrationBuilder(models []config.AgentModel) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		providers, err := agentProviderIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		existing, err := agentModelIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]agentModelMigrationValue, 0, len(models))
		for _, model := range models {
			id := existing[model.Alias]
			if id == "" {
				id, err = typed_id.New(typed_id.AgentModel)
				if err != nil {
					return err, ""
				}
			}
			values = append(values, agentModelMigrationValue{ID: id, Alias: model.Alias, Revision: model.Revision, ProviderID: providers[model.ProviderAlias], Model: model.Model, Parameters: model.Parameters, Enabled: model.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .ProviderID }}, {{ sqlLiteral .Model }}, {{ sqlLiteral .Parameters }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, provider_id = excluded.provider_id, model = excluded.model, parameters = excluded.parameters, enabled = excluded.enabled
			WHERE gatehouse_agent_models.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

type agentModelMigrationValue struct {
	ID, Alias, ProviderID, Model, Parameters string
	Revision                              int
	Enabled                               bool
}

func workspaceAgentMigrationBuilder(agents []config.WorkspaceAgent) MigrationBuilder {
	return templateMigrationBuilder(`
		SELECT 1;
		{{ range . }}
		INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, priority, max_turns, label, system_prompt, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), (SELECT id FROM gatehouse_agent_models WHERE alias = {{ sqlLiteral .ModelAlias }}), {{ sqlLiteral .Priority }}, {{ sqlLiteral .MaxTurns }}, {{ sqlOptionalString .Label }}, {{ sqlOptionalString .SystemPrompt }}, {{ sqlBool .Enabled }})
		ON CONFLICT (workspace_id, model_id) DO UPDATE SET priority = excluded.priority, max_turns = excluded.max_turns, label = excluded.label, system_prompt = excluded.system_prompt, enabled = excluded.enabled;
		{{ end }}
	`, agents)
}

type agentProviderIDAndRevision struct {
	ID       string
	Revision int
}

func agentProviderIDsAndRevisionsByAlias(ctx context.Context, session *MigrationSession) (map[string]agentProviderIDAndRevision, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id, revision FROM gatehouse_agent_providers WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get agent provider revisions: %w", err)
	}
	defer rows.Close()
	providers := map[string]agentProviderIDAndRevision{}
	for rows.Next() {
		var alias string
		var provider agentProviderIDAndRevision
		if err := rows.Scan(&alias, &provider.ID, &provider.Revision); err != nil {
			return nil, fmt.Errorf("read agent provider: %w", err)
		}
		providers[alias] = provider
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read agent providers: %w", err)
	}
	return providers, nil
}

func agentProviderIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id FROM gatehouse_agent_providers WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get agent provider IDs by alias: %w", err)
	}
	defer rows.Close()
	ids := map[string]string{}
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, fmt.Errorf("scan agent provider ID: %w", err)
		}
		ids[alias] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent provider IDs: %w", err)
	}
	return ids, nil
}

func agentModelIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id FROM gatehouse_agent_models WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get agent model IDs by alias: %w", err)
	}
	defer rows.Close()
	ids := map[string]string{}
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, fmt.Errorf("scan agent model ID: %w", err)
		}
		ids[alias] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent model IDs: %w", err)
	}
	return ids, nil
}
