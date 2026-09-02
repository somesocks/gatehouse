package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR004ReconcilePrincipals(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       4,
		Description: "reconcile_principals",
		Builder:     principalMigrationBuilder(state.Principals),
	}
}
