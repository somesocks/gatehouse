package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR011ReconcileWorkspaceAgents(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       11,
		Description: "reconcile_workspace_agents",
		Builder:     workspaceAgentMigrationBuilder(state.WorkspaceAgents),
	}
}
