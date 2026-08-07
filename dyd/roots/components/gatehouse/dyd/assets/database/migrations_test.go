package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestMigrateAppliesVersionedMigrationsOnceInIndexOrder(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{
			{
				Index:       2,
				Description: "record_second_version",
				SQL: `
					INSERT INTO gatehouse_test_events (entry) VALUES ('versioned-2');
				`,
			},
			{
				Index:       1,
				Description: "create_events",
				SQL: `
					CREATE TABLE gatehouse_test_events (
						entry TEXT NOT NULL
					) STRICT;
					INSERT INTO gatehouse_test_events (entry) VALUES ('versioned-1');
				`,
			},
		},
	}

	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	if got, want := eventEntries(t, database), []string{"versioned-1", "versioned-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event entries = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, database, migrationTypeVersioned), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("versioned migration indexes = %#v, want %#v", got, want)
	}
}

func TestMigrateRejectsChangedVersionedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_events",
			SQL:         `CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`,
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	registry.Versioned[0].SQL += "\n-- changed"
	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "different checksum") {
		t.Fatalf("migrate() error = %v, want changed checksum error", err)
	}
}

func TestMigrateRejectsOutOfOrderVersionedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       2,
			Description: "create_events",
			SQL:         `CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`,
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	registry.Versioned = append(registry.Versioned, VersionedMigration{
		Index:       1,
		Description: "create_legacy_events",
		SQL:         `CREATE TABLE gatehouse_test_legacy_events (entry TEXT NOT NULL) STRICT;`,
	})
	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "would run out of order") {
		t.Fatalf("migrate() error = %v, want out-of-order error", err)
	}
}

func TestMigrateRejectsPersistedOutOfOrderVersionedHistory(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{
			{Index: 1, Description: "first", SQL: `CREATE TABLE gatehouse_test_first (value TEXT) STRICT;`},
			{Index: 2, Description: "second", SQL: `CREATE TABLE gatehouse_test_second (value TEXT) STRICT;`},
		},
	}
	if err := migrateSQLite(context.Background(), database, Registry{Init: testSQLiteRegistry(t).Init}); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{2, 1} {
		migration := registry.Versioned[index-1]
		checksum := sha256.Sum256([]byte(migration.SQL))
		if _, err := database.Exec(`
			INSERT INTO gatehouse_schema_migrations (
				migration_type, migration_index, description, checksum
			) VALUES (?, ?, ?, ?)
		`, migrationTypeVersioned, migration.Index, migration.Description, checksum[:]); err != nil {
			t.Fatal(err)
		}
	}

	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "was applied out of order") {
		t.Fatalf("migrate() error = %v, want persisted out-of-order error", err)
	}
}

func TestMigrateAppliesOnlyChangedRepeatablesInIndexOrder(t *testing.T) {
	database := openMigrationTestDatabase(t)
	templateRegistry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_events",
			SQL:         `CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`,
		}},
		Repeatable: []RepeatableMigration{
			{
				Index:       2,
				Description: "record_second_value",
				Template:    `INSERT INTO gatehouse_test_events (entry) VALUES ('second:' || {{ sqlLiteral .Second }});`,
			},
			{
				Index:       1,
				Description: "record_first_value",
				Template:    `INSERT INTO gatehouse_test_events (entry) VALUES ('first:' || {{ sqlLiteral .First }});`,
			},
		},
	}

	err, registry := materializeRegistry(templateRegistry, repeatableValues{First: "one", Second: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	err, registry = materializeRegistry(templateRegistry, repeatableValues{First: "o'hare", Second: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	err, registry = materializeRegistry(templateRegistry, repeatableValues{First: "three", Second: "four"})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	if got, want := eventEntries(t, database), []string{
		"first:one",
		"second:two",
		"first:o'hare",
		"first:three",
		"second:four",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event entries = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, database, migrationTypeRepeatable), []int64{1, 2, 1, 1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("repeatable migration indexes = %#v, want %#v", got, want)
	}
}

func TestMigrateRejectsTemplateActionsInVersionedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	err := migrateSQLite(context.Background(), database, Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "invalid_template",
			SQL:         `CREATE TABLE {{ .Table }} (value TEXT) STRICT;`,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain template actions") {
		t.Fatalf("migrate() error = %v, want versioned template error", err)
	}
}

func TestMigrateRejectsMissingRepeatableTemplateValues(t *testing.T) {
	err, _ := materializeRegistry(Registry{
		Init: testSQLiteRegistry(t).Init,
		Repeatable: []RepeatableMigration{{
			Index:       1,
			Description: "missing_value",
			Template:    `SELECT {{ sqlLiteral .Missing }};`,
		}},
	}, struct{}{})
	if err == nil || !strings.Contains(err.Error(), "can't evaluate field Missing") {
		t.Fatalf("migrate() error = %v, want missing template value error", err)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "invalid_sql",
			SQL:         `CREATE TABL gatehouse_test_events (entry TEXT NOT NULL) STRICT;`,
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err == nil {
		t.Fatal("migrate() succeeded for invalid SQL")
	}

	var tableCount int
	if err := database.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'gatehouse_schema_migrations'
	`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Fatalf("migration history table count = %d, want 0", tableCount)
	}
}

func TestMigrateCoordinatesConcurrentSQLiteRunners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	first := openMigrationTestFileDatabase(t, path)
	second := openMigrationTestFileDatabase(t, path)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{
			{
				Index:       1,
				Description: "create_events",
				SQL:         `CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`,
			},
			{
				Index:       2,
				Description: "record_versioned",
				SQL:         `INSERT INTO gatehouse_test_events (entry) VALUES ('versioned');`,
			},
		},
		Repeatable: []RepeatableMigration{{
			Index:       1,
			Description: "record_repeatable",
			Template:    `INSERT INTO gatehouse_test_events (entry) VALUES ('repeatable');`,
		}},
	}

	err, registry := materializeRegistry(registry, struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, database := range []*sql.DB{first, second} {
		go func(database *sql.DB) {
			<-start
			errors <- migrateSQLite(context.Background(), database, registry)
		}(database)
	}
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}

	if got, want := eventEntries(t, first), []string{"versioned", "repeatable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event entries = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, first, migrationTypeVersioned), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("versioned migration indexes = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, first, migrationTypeRepeatable), []int64{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("repeatable migration indexes = %#v, want %#v", got, want)
	}
}

type repeatableValues struct {
	First  string
	Second string
}

func testSQLiteRegistry(t *testing.T) Registry {
	t.Helper()
	err, registry := sqliteMigrations(config.State{})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func eventEntries(t *testing.T, database queryer) []string {
	t.Helper()
	rows, err := database.Query(`SELECT entry FROM gatehouse_test_events ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var entries []string
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func historyIndexes(t *testing.T, database queryer, migrationType string) []int64 {
	t.Helper()
	rows, err := database.Query(`
		SELECT migration_index
		FROM gatehouse_schema_migrations
		WHERE migration_type = ?
		ORDER BY installed_rank
	`, migrationType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var indexes []int64
	for rows.Next() {
		var index int64
		if err := rows.Scan(&index); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return indexes
}

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
}
