package migrations

import (
	"context"
	"gatehouse/config"
)

func sqliteMigrationR011ReconcileWorkspaceAgents(agents []config.WorkspaceAgent) RepeatableMigration {
	return RepeatableMigration{
		Index:       11,
		Description: "reconcile_workspace_agents",
		Builder:     sqliteMigrationR011ReconcileWorkspaceAgentsBuilder(agents),
	}
}

func sqliteMigrationR011ReconcileWorkspaceAgentsBuilder(agents []config.WorkspaceAgent) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_workspace_agents_desired;
			DROP TABLE IF EXISTS gatehouse_migration_workspace_agents_state;
			DROP TABLE IF EXISTS gatehouse_migration_workspace_agent_activities;

			CREATE TEMP TABLE gatehouse_migration_workspace_agents_desired (
				workspace_alias TEXT NOT NULL,
				alias TEXT NOT NULL,
				model_alias TEXT NOT NULL,
				revision INTEGER NOT NULL,
				priority INTEGER NOT NULL,
				label TEXT,
				system_prompt TEXT,
				enabled INTEGER NOT NULL,
				PRIMARY KEY (workspace_alias, alias)
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_workspace_agents_desired (workspace_alias, alias, model_alias, revision, priority, label, system_prompt, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .ModelAlias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Priority }}, {{ sqlOptionalString .Label }}, {{ sqlOptionalString .SystemPrompt }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_workspace_agents_state AS
			SELECT
				desired.workspace_alias,
				desired.alias,
				desired.model_alias,
				workspaces.id AS workspace_id,
				models.id AS model_id,
				agents.id AS existing_id,
				COALESCE(agents.id, gh_id_new('wag')) AS binding_id,
				desired.revision,
				agents.revision AS existing_revision,
				desired.priority,
				agents.priority AS existing_priority,
				desired.label,
				agents.label AS existing_label,
				desired.system_prompt,
				agents.system_prompt AS existing_system_prompt,
				desired.enabled,
				agents.enabled AS existing_enabled
			FROM gatehouse_migration_workspace_agents_desired AS desired
			LEFT JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
			LEFT JOIN gatehouse_agent_models AS models ON models.alias = desired.model_alias
			LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = workspaces.id AND agents.alias = desired.alias;

			CREATE TEMP TABLE gatehouse_migration_workspace_agent_activities (
				workspace_id TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL,
				binding_id TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_workspace_agent_activities (workspace_id, id, event, binding_id)
			SELECT
				workspace_id,
				gh_id_new('act'),
				CASE WHEN existing_id IS NULL THEN 'workspace_agent.create' ELSE 'workspace_agent.update' END,
				binding_id
			FROM gatehouse_migration_workspace_agents_state
			WHERE existing_id IS NULL OR existing_revision < revision;

			INSERT INTO gatehouse_workspace_agents (id, workspace_id, alias, model_id, revision, priority, label, system_prompt, enabled)
			SELECT binding_id, workspace_id, alias, model_id, revision, priority, label, system_prompt, enabled
			FROM gatehouse_migration_workspace_agents_state
			WHERE TRUE
			ON CONFLICT (workspace_id, alias) DO UPDATE SET
				model_id = excluded.model_id,
				revision = excluded.revision,
				priority = excluded.priority,
				label = excluded.label,
				system_prompt = excluded.system_prompt,
				enabled = excluded.enabled
			WHERE gatehouse_workspace_agents.revision < excluded.revision;

			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_workspace_agent_workspace, resource_workspace_agent_id, created_at
			)
			SELECT id, event, 'workspace_agent', workspace_id, binding_id, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_agent_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, workspace_id
			FROM gatehouse_migration_workspace_agent_activities;

			DROP TABLE gatehouse_migration_workspace_agent_activities;
			DROP TABLE gatehouse_migration_workspace_agents_state;
			DROP TABLE gatehouse_migration_workspace_agents_desired;
		`, agents)
	}
}
