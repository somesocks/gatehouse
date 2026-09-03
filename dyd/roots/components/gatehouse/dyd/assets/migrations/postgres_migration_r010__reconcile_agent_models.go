package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR010ReconcileAgentModels(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       10,
		Description: "reconcile_agent_models",
		Builder:     postgresMigrationR010ReconcileAgentModelsBuilder(state.AgentModels),
	}
}

type postgresMigrationR010AgentModelValue struct {
	ID, Alias, ProviderID, Model, Parameters, Compaction string
	Revision, MaxTurns, MaxOutputTokens                  int
	Enabled                                              bool
}

func postgresMigrationR010ReconcileAgentModelsBuilder(models []config.AgentModel) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		providers, err := postgresMigrationR010AgentProviderIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		existing, err := postgresMigrationR010AgentModelIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]postgresMigrationR010AgentModelValue, 0, len(models))
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
			values = append(values, postgresMigrationR010AgentModelValue{ID: id, Alias: model.Alias, Revision: model.Revision, ProviderID: providers[model.ProviderAlias], Model: model.Model, Parameters: model.Parameters, Compaction: string(compaction), MaxTurns: model.MaxTurns, MaxOutputTokens: model.MaxOutputTokens, Enabled: model.Enabled})
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

func postgresMigrationR010AgentProviderIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
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

func postgresMigrationR010AgentModelIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
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
