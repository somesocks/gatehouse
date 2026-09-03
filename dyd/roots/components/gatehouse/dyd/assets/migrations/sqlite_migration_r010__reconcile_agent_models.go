package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
)

func sqliteMigrationR010ReconcileAgentModels(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       10,
		Description: "reconcile_agent_models",
		Builder:     sqliteMigrationR010ReconcileAgentModelsBuilder(state.AgentModels),
	}
}

type sqliteMigrationR010AgentModelValue struct {
	Alias, ProviderAlias, Model, Parameters, Compaction string
	Revision, MaxTurns, MaxOutputTokens                 int
	Enabled                                             bool
}

func sqliteMigrationR010ReconcileAgentModelsBuilder(models []config.AgentModel) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		values := make([]sqliteMigrationR010AgentModelValue, 0, len(models))
		for _, model := range models {
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
			values = append(values, sqliteMigrationR010AgentModelValue{Alias: model.Alias, Revision: model.Revision, ProviderAlias: model.ProviderAlias, Model: model.Model, Parameters: model.Parameters, Compaction: string(compaction), MaxTurns: model.MaxTurns, MaxOutputTokens: model.MaxOutputTokens, Enabled: model.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled)
			VALUES (COALESCE((SELECT id FROM gatehouse_agent_models WHERE alias = {{ sqlLiteral .Alias }}), gh_id_new('amd')), {{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, (SELECT id FROM gatehouse_agent_providers WHERE alias = {{ sqlLiteral .ProviderAlias }}), {{ sqlLiteral .Model }}, {{ sqlLiteral .Parameters }}, {{ sqlLiteral .Compaction }}, {{ sqlLiteral .MaxTurns }}, {{ sqlLiteral .MaxOutputTokens }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, provider_id = excluded.provider_id, model = excluded.model, parameters = excluded.parameters, compaction = excluded.compaction, max_turns = excluded.max_turns, max_output_tokens = excluded.max_output_tokens, enabled = excluded.enabled
			WHERE gatehouse_agent_models.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}
