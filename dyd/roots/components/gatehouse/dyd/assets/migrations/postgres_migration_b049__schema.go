package migrations

import _ "embed"

//go:embed postgres_migration_b049__schema.sql
var postgresMigrationB049Source string

func postgresMigrationB049Schema() BaselineMigration {
	return BaselineMigration{
		Index:       49,
		Description: "schema_at_v049",
		Builder:     staticMigrationBuilder(postgresMigrationB049Source),
	}
}
