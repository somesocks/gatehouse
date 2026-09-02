package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR002SeedGatehouseWorkspace() RepeatableMigration {
	gatehouseName := "Gatehouse"
	return RepeatableMigration{
		Index:       2,
		Description: "seed_gatehouse_workspace",
		Builder:     postgresMigrationR003ReconcileWorkspacesBuilder([]config.Workspace{{Alias: "gatehouse", Name: &gatehouseName, Enabled: true}}),
	}
}
