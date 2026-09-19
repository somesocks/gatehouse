package migrations

import _ "embed"

//go:embed sqlite_migration_b049__schema.sql
var sqliteMigrationB049Source string

func sqliteMigrationB049Schema() BaselineMigration {
	return BaselineMigration{
		Index:       49,
		Description: "schema_at_v049",
		Builder:     staticMigrationBuilder(sqliteMigrationB049Source),
	}
}
