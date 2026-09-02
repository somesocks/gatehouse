package migrations

import (
	"gatehouse/config"
)

func sqliteMigrationR002SeedGatehouseWorkspace() RepeatableMigration {
	gatehouseName := "Gatehouse"
	return RepeatableMigration{
		Index:       2,
		Description: "seed_gatehouse_workspace",
		Builder:     sqliteMigrationR003ReconcileWorkspacesBuilder([]config.Workspace{{Alias: "gatehouse", Name: &gatehouseName, Enabled: true}}),
	}
}
