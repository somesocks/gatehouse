package migrations

import (
	"gatehouse/config"
)

func sqliteMigrationR005ReconcileIdentities(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       5,
		Description: "reconcile_identities",
		Builder:     identityMigrationBuilder(state.Principals),
	}
}
