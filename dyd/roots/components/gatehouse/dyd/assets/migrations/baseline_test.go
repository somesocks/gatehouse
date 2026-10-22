package migrations

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
)

// testSchemaAtVersion selects a historical schema without applying current
// reconciliation or a baseline newer than the version under test.
func testSchemaAtVersion(registry Registry, version int64) Registry {
	result := Registry{Init: registry.Init}
	for _, baseline := range registry.Baseline {
		if baseline.Index <= version {
			result.Baseline = append(result.Baseline, baseline)
		}
	}
	for _, migration := range registry.Versioned {
		if migration.Index <= version {
			result.Versioned = append(result.Versioned, migration)
		}
	}
	return result
}

func TestSQLiteNewBaselinePreservesExistingHistory(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	if err := migrateSQLite(ctx, db, testSchemaAtVersion(registry, 49)); err != nil {
		t.Fatal(err)
	}
	checksum := historyChecksum(t, db, migrationTypeBaseline, 49)
	if _, err := db.Exec(`INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000000', 'preserved', TRUE)`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	if got := historyIndexes(t, db, migrationTypeBaseline); !reflect.DeepEqual(got, []int64{49}) {
		t.Fatalf("existing database baseline = %v, want [49]", got)
	}
	if got := historyChecksum(t, db, migrationTypeBaseline, 49); got != checksum {
		t.Fatal("existing baseline checksum changed")
	}
	var alias string
	if err := db.QueryRow(`SELECT alias FROM gatehouse_workspaces WHERE id = 'wsp_00000000000000000000000000'`).Scan(&alias); err != nil || alias != "preserved" {
		t.Fatalf("existing workspace = %q, %v", alias, err)
	}
	latest := openConfiguredMigrationTestDatabase(t)
	if err := migrateSQLite(ctx, latest, registry); err != nil {
		t.Fatal(err)
	}
	if got := historyIndexes(t, latest, migrationTypeBaseline); !reflect.DeepEqual(got, []int64{68}) {
		t.Fatalf("fresh database baseline = %v, want [68]", got)
	}
	if got := historyIndexes(t, latest, migrationTypeVersioned); len(got) != 0 {
		t.Fatalf("fresh B068 database executed versioned migrations: %v", got)
	}
	if got, want := sqliteSchemaDefinitions(t, latest), sqliteSchemaDefinitions(t, db); !reflect.DeepEqual(got, want) {
		t.Fatal("new baseline schema differs from upgraded B049 database")
	}
}

func BenchmarkSQLiteFreshSchema(b *testing.B) {
	err, ring := keychain.NewKeyring(nil, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		b.Fatal(err)
	}
	defer ring.Close()
	err, registry := sqliteMigrations(config.State{}, ring)
	if err != nil {
		b.Fatal(err)
	}
	registry.Repeatable = nil
	for _, baseline := range registry.Baseline {
		b.Run(fmt.Sprintf("B%03d", baseline.Index), func(b *testing.B) {
			selected := registry
			selected.Baseline = []BaselineMigration{baseline}
			b.ReportAllocs()
			for b.Loop() {
				err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
				if err != nil {
					b.Fatal(err)
				}
				err = migrateSQLite(context.Background(), store.DB, selected)
				closeErr := store.Close()
				if err != nil {
					b.Fatal(err)
				}
				if closeErr != nil {
					b.Fatal(closeErr)
				}
			}
		})
	}
}
