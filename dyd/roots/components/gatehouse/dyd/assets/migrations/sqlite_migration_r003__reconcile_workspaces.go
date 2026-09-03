package migrations

import (
	"context"

	"gatehouse/config"
)

func sqliteMigrationR003ReconcileWorkspaces(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       3,
		Description: "reconcile_workspaces",
		Builder:     sqliteMigrationR003ReconcileWorkspacesBuilder(state.Workspaces),
	}
}

func sqliteMigrationR003ReconcileWorkspacesBuilder(workspaces []config.Workspace) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_workspace_desired;
			DROP TABLE IF EXISTS gatehouse_migration_workspace_state;
			DROP TABLE IF EXISTS gatehouse_migration_workspace_activities;

			CREATE TEMP TABLE gatehouse_migration_workspace_desired (
				alias TEXT NOT NULL PRIMARY KEY,
				name TEXT,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_workspace_desired (alias, name, enabled)
			VALUES ({{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_workspace_state AS
			SELECT
				desired.alias,
				COALESCE(workspaces.id, gh_id_new('wsp')) AS id,
				workspaces.id AS existing_id,
				desired.name,
				workspaces.name AS existing_name,
				desired.enabled,
				workspaces.enabled AS existing_enabled
			FROM gatehouse_migration_workspace_desired AS desired
			LEFT JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.alias;

			CREATE TEMP TABLE gatehouse_migration_workspace_activities (
				workspace TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_workspace_activities (workspace, id, event)
			SELECT
				id,
				gh_id_new('act'),
				CASE WHEN existing_id IS NULL THEN 'workspace.create' ELSE 'workspace.update' END
			FROM gatehouse_migration_workspace_state
			WHERE existing_id IS NULL OR existing_name IS NOT name OR existing_enabled IS NOT enabled;

			INSERT INTO gatehouse_workspaces (id, alias, name, enabled)
			SELECT id, alias, name, enabled
			FROM gatehouse_migration_workspace_state
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET name = excluded.name, enabled = excluded.enabled;

			INSERT INTO gatehouse_activity_events (
				workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
			)
			SELECT workspace, id, event, 'workspace', NULL, NULL, NULL, NULL, NULL, NULL, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_activities;
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			SELECT workspace, id, 'workspace/' || workspace, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_activities;

			DROP TABLE gatehouse_migration_workspace_activities;
			DROP TABLE gatehouse_migration_workspace_state;
			DROP TABLE gatehouse_migration_workspace_desired;
		`, workspaces)
	}
}
