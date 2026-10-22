package migrations

import _ "embed"

//go:embed sqlite_migration_b068__schema.sql
var sqliteMigrationB068Source string

func sqliteMigrationB068Schema() BaselineMigration {
	return BaselineMigration{
		Index:       68,
		Description: "schema_at_v068",
		Builder:     staticMigrationBuilder(sqliteMigrationB068Source),
	}
}
