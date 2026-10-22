package migrations

import _ "embed"

//go:embed postgres_migration_b068__schema.sql
var postgresMigrationB068Source string

func postgresMigrationB068Schema() BaselineMigration {
	return BaselineMigration{
		Index:       68,
		Description: "schema_at_v068",
		Builder:     staticMigrationBuilder(postgresMigrationB068Source),
	}
}
