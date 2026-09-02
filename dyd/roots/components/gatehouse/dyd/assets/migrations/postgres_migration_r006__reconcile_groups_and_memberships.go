package migrations

import (
	"gatehouse/config"
)

func postgresMigrationR006ReconcileGroupsAndMemberships(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       6,
		Description: "reconcile_groups_and_memberships",
		Builder:     groupMigrationBuilder(state.Groups),
	}
}
