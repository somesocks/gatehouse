package migrations

import (
	"context"

	"gatehouse/config"
)

func sqliteMigrationR013ReconcileWorkspaceStorageProviders(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       13,
		Description: "reconcile_workspace_storage_providers",
		Builder:     sqliteMigrationR013ReconcileWorkspaceStorageProvidersBuilder(state.WorkspaceStorageProviders),
	}
}

func sqliteMigrationR013ReconcileWorkspaceStorageProvidersBuilder(providers []config.WorkspaceStorageProvider) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_workspace_storage_provider_desired;
			DROP TABLE IF EXISTS gatehouse_migration_workspace_storage_provider_state;
			DROP TABLE IF EXISTS gatehouse_migration_workspace_storage_provider_activities;

			CREATE TEMP TABLE gatehouse_migration_workspace_storage_provider_desired (
				workspace_alias TEXT NOT NULL,
				provider_alias TEXT NOT NULL,
				priority INTEGER NOT NULL,
				enabled INTEGER NOT NULL,
				PRIMARY KEY (workspace_alias, provider_alias)
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_workspace_storage_provider_desired (workspace_alias, provider_alias, priority, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .ProviderAlias }}, {{ sqlLiteral .Priority }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_workspace_storage_provider_state AS
			SELECT
				desired.priority,
				desired.enabled,
				workspaces.id AS workspace,
				providers.id AS provider,
				bindings.provider AS existing_provider,
				bindings.priority AS existing_priority,
				bindings.enabled AS existing_enabled
			FROM gatehouse_migration_workspace_storage_provider_desired AS desired
			JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
			JOIN gatehouse_storage_providers AS providers ON providers.alias = desired.provider_alias
			LEFT JOIN gatehouse_workspace_storage_providers AS bindings ON bindings.workspace = workspaces.id AND bindings.provider = providers.id;

			CREATE TEMP TABLE gatehouse_migration_workspace_storage_provider_activities (
				workspace TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL,
				provider TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_workspace_storage_provider_activities (workspace, id, event, provider)
			SELECT
				workspace,
				gh_id_new('act'),
				CASE WHEN existing_provider IS NULL THEN 'workspace_storage_provider.create' ELSE 'workspace_storage_provider.update' END,
				provider
			FROM gatehouse_migration_workspace_storage_provider_state
			WHERE existing_provider IS NULL OR existing_priority IS NOT priority OR existing_enabled IS NOT enabled;

			INSERT INTO gatehouse_workspace_storage_providers (workspace, provider, priority, enabled)
			SELECT workspace, provider, priority, enabled
			FROM gatehouse_migration_workspace_storage_provider_state
			WHERE TRUE
			ON CONFLICT (workspace, provider) DO UPDATE SET priority = excluded.priority, enabled = excluded.enabled;

			INSERT INTO gatehouse_activity_events (
				workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
			)
			SELECT workspace, id, event, 'workspace_storage_provider', NULL, NULL, NULL, NULL, NULL, NULL, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_storage_provider_activities;
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			SELECT workspace, id, 'workspace_storage_provider/' || provider, gh_id_timestamp(id)
			FROM gatehouse_migration_workspace_storage_provider_activities;

			DROP TABLE gatehouse_migration_workspace_storage_provider_activities;
			DROP TABLE gatehouse_migration_workspace_storage_provider_state;
			DROP TABLE gatehouse_migration_workspace_storage_provider_desired;
		`, providers)
	}
}
