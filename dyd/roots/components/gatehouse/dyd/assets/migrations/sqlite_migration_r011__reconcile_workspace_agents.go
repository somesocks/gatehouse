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
				model_alias TEXT NOT NULL,
				priority INTEGER NOT NULL,
				label TEXT,
				system_prompt TEXT,
				enabled INTEGER NOT NULL,
				PRIMARY KEY (workspace_alias, model_alias)
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_workspace_agents_desired (workspace_alias, model_alias, priority, label, system_prompt, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ModelAlias }}, {{ sqlLiteral .Priority }}, {{ sqlOptionalString .Label }}, {{ sqlOptionalString .SystemPrompt }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_workspace_agents_state AS
			SELECT
				desired.workspace_alias,
				desired.model_alias,
				workspaces.id AS workspace_id,
				models.id AS model_id,
				agents.workspace_id AS existing_workspace_id,
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
			LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = workspaces.id AND agents.model_id = models.id;

			CREATE TEMP TABLE gatehouse_migration_workspace_agent_activities (
				workspace_id TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL,
				model_id TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_workspace_agent_activities (workspace_id, id, event, model_id)
			SELECT
				workspace_id,
				gh_id_new('act'),
				CASE WHEN existing_workspace_id IS NULL THEN 'workspace_agent.create' ELSE 'workspace_agent.update' END,
				model_id
			FROM gatehouse_migration_workspace_agents_state
			WHERE existing_workspace_id IS NULL
				OR existing_priority IS NOT priority
				OR existing_label IS NOT label
				OR existing_system_prompt IS NOT system_prompt
				OR existing_enabled IS NOT enabled;

			INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, priority, label, system_prompt, enabled)
			SELECT workspace_id, model_id, priority, label, system_prompt, enabled
			FROM gatehouse_migration_workspace_agents_state
			WHERE TRUE
			ON CONFLICT (workspace_id, model_id) DO UPDATE SET
				priority = excluded.priority,
				label = excluded.label,
				system_prompt = excluded.system_prompt,
				enabled = excluded.enabled;

			INSERT INTO gatehouse_activity_events (
				workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
			)
			SELECT workspace_id, id, event, 'workspace_agent', NULL, NULL, NULL, NULL, NULL, model_id, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_agent_activities;
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			SELECT workspace_id, id, 'workspace_agent/' || model_id, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_agent_activities;

			DROP TABLE gatehouse_migration_workspace_agent_activities;
			DROP TABLE gatehouse_migration_workspace_agents_state;
			DROP TABLE gatehouse_migration_workspace_agents_desired;
		`, agents)
	}
}
