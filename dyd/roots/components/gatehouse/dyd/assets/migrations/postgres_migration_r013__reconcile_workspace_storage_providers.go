package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR013ReconcileWorkspaceStorageProviders(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       13,
		Description: "reconcile_workspace_storage_providers",
		Builder: templateMigrationBuilder(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_workspace_storage_providers (workspace, provider, priority, enabled)
				VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), (SELECT id FROM gatehouse_storage_providers WHERE alias = {{ sqlLiteral .ProviderAlias }}), {{ sqlLiteral .Priority }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace, provider) DO UPDATE SET priority = excluded.priority, enabled = excluded.enabled;
			{{ end }}
		`, state.WorkspaceStorageProviders),
	}
}
