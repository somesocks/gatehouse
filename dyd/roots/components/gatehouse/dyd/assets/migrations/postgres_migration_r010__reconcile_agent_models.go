package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR010ReconcileAgentModels(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       10,
		Description: "reconcile_agent_models",
		Builder:     agentModelMigrationBuilder(state.AgentModels),
	}
}
