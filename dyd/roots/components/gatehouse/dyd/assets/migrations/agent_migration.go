package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
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
			if model.Compaction.Algorithm == "" {
				model.Compaction = config.AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: config.DefaultAgentModelHistoryBytes, BufferBytes: config.DefaultAgentModelBufferBytes}
			}
			if model.MaxTurns == 0 {
				model.MaxTurns = config.DefaultAgentModelMaxTurns
			}
			if model.MaxOutputTokens == 0 {
				model.MaxOutputTokens = config.DefaultAgentModelMaxOutputTokens
			}
			compaction, err := json.Marshal(model.Compaction)
			if err != nil {
				return fmt.Errorf("encode agent model compaction %q: %w", model.Alias, err), ""
			}
			values = append(values, agentModelMigrationValue{ID: id, Alias: model.Alias, Revision: model.Revision, ProviderID: providers[model.ProviderAlias], Model: model.Model, Parameters: model.Parameters, Compaction: string(compaction), MaxTurns: model.MaxTurns, MaxOutputTokens: model.MaxOutputTokens, Enabled: model.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .ProviderID }}, {{ sqlLiteral .Model }}, {{ sqlLiteral .Parameters }}, {{ sqlLiteral .Compaction }}, {{ sqlLiteral .MaxTurns }}, {{ sqlLiteral .MaxOutputTokens }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, provider_id = excluded.provider_id, model = excluded.model, parameters = excluded.parameters, compaction = excluded.compaction, max_turns = excluded.max_turns, max_output_tokens = excluded.max_output_tokens, enabled = excluded.enabled
			WHERE gatehouse_agent_models.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

type agentModelMigrationValue struct {
	ID, Alias, ProviderID, Model, Parameters, Compaction string
	Revision, MaxTurns, MaxOutputTokens               int
	Enabled                                            bool
}

func workspaceAgentMigrationBuilder(agents []config.WorkspaceAgent) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := workspaceAgentsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		modelIDs, err := agentModelIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		events := []activityMigrationEvent{}
		for _, agent := range agents {
			key := workspaceAgentAliasKey{Workspace: agent.WorkspaceID, Model: agent.ModelAlias}
			stored, exists := existing[key]
			if !exists || stored.Priority != agent.Priority || stored.Enabled != agent.Enabled || !sameOptionalString(stored.Label, agent.Label) || !sameOptionalString(stored.SystemPrompt, agent.SystemPrompt) {
				eventName := "workspace_agent.create"
				if exists {
					eventName = "workspace_agent.update"
				}
				modelID := modelIDs[agent.ModelAlias]
				event, err := newActivityMigrationEvent(agent.WorkspaceID, eventName, "workspace_agent", "", "", modelID, "workspace_agent/"+modelID)
				if err != nil {
					return err, ""
				}
				events = append(events, event)
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Agents }}
			INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, priority, label, system_prompt, enabled)
				VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), (SELECT id FROM gatehouse_agent_models WHERE alias = {{ sqlLiteral .ModelAlias }}), {{ sqlLiteral .Priority }}, {{ sqlOptionalString .Label }}, {{ sqlOptionalString .SystemPrompt }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, model_id) DO UPDATE SET priority = excluded.priority, label = excluded.label, system_prompt = excluded.system_prompt, enabled = excluded.enabled;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
			) VALUES (
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceAlias }}), {{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, NULL, NULL, NULL, NULL, NULL, {{ sqlLiteral .WorkspaceAgent }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral $event.WorkspaceAlias }}), {{ sqlLiteral $event.ID }}, {{ sqlLiteral . }}, {{ sqlLiteral $event.CreatedAt }});
			{{ end }}
			{{ end }}
		`, struct {
			Agents []config.WorkspaceAgent
			Events []activityMigrationEvent
		}{Agents: agents, Events: events})
	}
}

type workspaceAgentAliasKey struct {
	Workspace string
	Model     string
}

type storedWorkspaceAgent struct {
	Priority     int
	Label        sql.NullString
	SystemPrompt sql.NullString
	Enabled      bool
}

func workspaceAgentsByAlias(ctx context.Context, session *MigrationSession) (map[workspaceAgentAliasKey]storedWorkspaceAgent, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, models.alias, agents.priority, agents.label, agents.system_prompt, agents.enabled
		FROM gatehouse_workspace_agents AS agents
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = agents.workspace_id
		JOIN gatehouse_agent_models AS models ON models.id = agents.model_id
		WHERE workspaces.alias IS NOT NULL AND models.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get workspace agents by alias: %w", err)
	}
	defer rows.Close()

	agents := map[workspaceAgentAliasKey]storedWorkspaceAgent{}
	for rows.Next() {
		var key workspaceAgentAliasKey
		var agent storedWorkspaceAgent
		if err := rows.Scan(&key.Workspace, &key.Model, &agent.Priority, &agent.Label, &agent.SystemPrompt, &agent.Enabled); err != nil {
			return nil, fmt.Errorf("scan workspace agent: %w", err)
		}
		agents[key] = agent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace agents: %w", err)
	}
	return agents, nil
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
