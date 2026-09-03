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

func sqliteMigrationR009ReconcileAgentProviders(state config.State, keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       9,
		Description: "reconcile_agent_providers",
		Builder:     sqliteMigrationR009ReconcileAgentProvidersBuilder(state.AgentProviders, keyring),
	}
}

type sqliteMigrationR009AgentProviderValue struct {
	Alias, Protocol string
	BaseURL         any
	APIKey          any
	KeychainID      any
	KeychainVersion any
	Revision        int
	Enabled         bool
}

func sqliteMigrationR009ReconcileAgentProvidersBuilder(providers []config.AgentProvider, keyring *keychain.Keyring) MigrationBuilder {
	resolver := agent.NewAPIKeySourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		revisions, err := sqliteMigrationR009AgentProviderRevisionsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]sqliteMigrationR009AgentProviderValue, 0, len(providers))
		for _, provider := range providers {
			if revisions[provider.Alias] >= provider.Revision {
				continue
			}
			value := sqliteMigrationR009AgentProviderValue{Alias: provider.Alias, Revision: provider.Revision, Protocol: provider.Protocol, Enabled: provider.Enabled}
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
			sealErr, encrypted := keychain.Seal(rand.Reader, keys[reference], []byte("gh=v1|agent-provider|alias="+provider.Alias), apiKey)
			clear(apiKey)
			clear(keys[reference])
			if sealErr != nil {
				return fmt.Errorf("encrypt API key for provider %q: %w", provider.Alias, sealErr), ""
			}
			encrypted.AAD = keychain.AADAlias
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
			VALUES (COALESCE((SELECT id FROM gatehouse_agent_providers WHERE alias = {{ sqlLiteral .Alias }}), gh_id_new('apr')), {{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Protocol }}, {{ sqlLiteral .BaseURL }}, {{ sqlLiteral .KeychainID }}, {{ sqlLiteral .KeychainVersion }}, {{ sqlLiteral .APIKey }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, protocol = excluded.protocol, base_url = excluded.base_url, keychain_id = excluded.keychain_id, keychain_version = excluded.keychain_version, api_key = excluded.api_key, enabled = excluded.enabled
			WHERE gatehouse_agent_providers.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

func sqliteMigrationR009AgentProviderRevisionsByAlias(ctx context.Context, session *MigrationSession) (map[string]int, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, revision FROM gatehouse_agent_providers WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get agent provider revisions: %w", err)
	}
	defer rows.Close()

	revisions := map[string]int{}
	for rows.Next() {
		var alias string
		var revision int
		if err := rows.Scan(&alias, &revision); err != nil {
			return nil, fmt.Errorf("read agent provider: %w", err)
		}
		revisions[alias] = revision
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read agent providers: %w", err)
	}
	return revisions, nil
}
