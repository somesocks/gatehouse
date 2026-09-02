package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func postgresMigrationR009ReconcileAgentProviders(state config.State, keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       9,
		Description: "reconcile_agent_providers",
		Builder:     agentProviderMigrationBuilder(state.AgentProviders, keyring),
	}
}
