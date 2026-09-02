package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR013ReconcileWorkspaceStorageProviders(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       13,
		Description: "reconcile_workspace_storage_providers",
		Builder:     workspaceStorageProviderMigrationBuilder(state.WorkspaceStorageProviders),
	}
}
