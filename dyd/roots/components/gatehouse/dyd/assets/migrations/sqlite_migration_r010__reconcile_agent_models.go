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
			DROP TABLE IF EXISTS gatehouse_migration_agent_model_desired;
			DROP TABLE IF EXISTS gatehouse_migration_agent_model_state;
			DROP TABLE IF EXISTS gatehouse_migration_agent_model_activities;

			CREATE TEMP TABLE gatehouse_migration_agent_model_desired (
				alias TEXT NOT NULL PRIMARY KEY,
				revision INTEGER NOT NULL,
				provider_alias TEXT NOT NULL,
				model TEXT NOT NULL,
				parameters TEXT NOT NULL,
				compaction TEXT NOT NULL,
				max_turns INTEGER NOT NULL,
				max_output_tokens INTEGER NOT NULL,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_agent_model_desired (alias, revision, provider_alias, model, parameters, compaction, max_turns, max_output_tokens, enabled)
			VALUES ({{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .ProviderAlias }}, {{ sqlLiteral .Model }}, {{ sqlLiteral .Parameters }}, {{ sqlLiteral .Compaction }}, {{ sqlLiteral .MaxTurns }}, {{ sqlLiteral .MaxOutputTokens }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_agent_model_state AS
			SELECT desired.*, providers.id AS provider_id, COALESCE(models.id, gh_id_new('amd')) AS id, models.id AS existing_id
			FROM gatehouse_migration_agent_model_desired AS desired
			LEFT JOIN gatehouse_agent_providers AS providers ON providers.alias = desired.provider_alias
			LEFT JOIN gatehouse_agent_models AS models ON models.alias = desired.alias
			WHERE models.id IS NULL OR models.revision < desired.revision;

			CREATE TEMP TABLE gatehouse_migration_agent_model_activities (
				model TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_agent_model_activities (model, id, event)
			SELECT id, gh_id_new('act'), CASE WHEN existing_id IS NULL THEN 'agent_model.create' ELSE 'agent_model.update' END
			FROM gatehouse_migration_agent_model_state;

			INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled)
			SELECT id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled
			FROM gatehouse_migration_agent_model_state
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, provider_id = excluded.provider_id, model = excluded.model, parameters = excluded.parameters, compaction = excluded.compaction, max_turns = excluded.max_turns, max_output_tokens = excluded.max_output_tokens, enabled = excluded.enabled
			WHERE gatehouse_agent_models.revision < excluded.revision;

			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_agent_model, created_at
			)
			SELECT id, event, 'agent_model', model, gh_id_timestamp(id)
			FROM gatehouse_migration_agent_model_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, 'sys/' || model FROM gatehouse_migration_agent_model_activities;

			DROP TABLE gatehouse_migration_agent_model_activities;
			DROP TABLE gatehouse_migration_agent_model_state;
			DROP TABLE gatehouse_migration_agent_model_desired;
		`, values)
	}
}
