package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"
)

func TestPostgresBaselinesMatchVersionedSchema(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_BASELINE_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL server with CREATE DATABASE permission")
	}
	ctx := context.Background()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	legacy := newPostgresBaselineTestDatabase(t)
	versioned := registry
	versioned.Baseline = nil
	if err := migratePostgres(ctx, legacy, versioned); err != nil {
		t.Fatal(err)
	}
	expected := postgresSchemaDefinitions(t, legacy)
	followUp := VersionedMigration{Index: 70, Description: "baseline_follow_up", Builder: staticMigrationBuilder(`CREATE TABLE gatehouse_baseline_follow_up (id TEXT PRIMARY KEY);`)}
	for _, baseline := range registry.Baseline {
		t.Run(fmt.Sprintf("B%03d", baseline.Index), func(t *testing.T) {
			db := newPostgresBaselineTestDatabase(t)
			selected := registry
			selected.Baseline = []BaselineMigration{baseline}
			started := time.Now()
			if err := migratePostgres(ctx, db, selected); err != nil {
				t.Fatal(err)
			}
			t.Logf("fresh schema from B%03d: %s", baseline.Index, time.Since(started))
			if got := postgresSchemaDefinitions(t, db); !reflect.DeepEqual(got, expected) {
				assertPostgresSchemaEqual(t, got, expected)
			}
			if got := postgresHistoryIndexes(t, db, migrationTypeBaseline); !reflect.DeepEqual(got, []int64{baseline.Index}) {
				t.Fatalf("baseline history = %v, want [%d]", got, baseline.Index)
			}
			var wantVersioned []int64
			for _, migration := range selected.Versioned {
				if migration.Index > baseline.Index {
					wantVersioned = append(wantVersioned, migration.Index)
				}
			}
			if got := postgresHistoryIndexes(t, db, migrationTypeVersioned); !reflect.DeepEqual(got, wantVersioned) {
				t.Fatalf("versioned history = %v, want %v", got, wantVersioned)
			}
			selected.Versioned = append(append([]VersionedMigration(nil), selected.Versioned...), followUp)
			for range 2 {
				if err := migratePostgres(ctx, db, selected); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`SELECT * FROM gatehouse_baseline_follow_up LIMIT 0`); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("upgrade-existing-B049", func(t *testing.T) {
		db := newPostgresBaselineTestDatabase(t)
		if err := migratePostgres(ctx, db, testSchemaAtVersion(registry, 49)); err != nil {
			t.Fatal(err)
		}
		var checksum []byte
		if err := db.QueryRow(`SELECT checksum FROM gatehouse_schema_migrations WHERE migration_type = 'baseline' AND migration_index = 49`).Scan(&checksum); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000000', 'preserved', TRUE)`); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := migratePostgres(ctx, db, registry); err != nil {
				t.Fatal(err)
			}
		}
		if got := postgresHistoryIndexes(t, db, migrationTypeBaseline); !reflect.DeepEqual(got, []int64{49}) {
			t.Fatalf("existing baseline history = %v, want [49]", got)
		}
		var preservedChecksum []byte
		if err := db.QueryRow(`SELECT checksum FROM gatehouse_schema_migrations WHERE migration_type = 'baseline' AND migration_index = 49`).Scan(&preservedChecksum); err != nil || !reflect.DeepEqual(preservedChecksum, checksum) {
			t.Fatalf("existing checksum changed: %v", err)
		}
		var alias string
		if err := db.QueryRow(`SELECT alias FROM gatehouse_workspaces WHERE id = 'wsp_00000000000000000000000000'`).Scan(&alias); err != nil || alias != "preserved" {
			t.Fatalf("existing workspace = %q, %v", alias, err)
		}
		if got := postgresSchemaDefinitions(t, db); !reflect.DeepEqual(got, expected) {
			assertPostgresSchemaEqual(t, got, expected)
		}
	})
}

func newPostgresBaselineTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	err, control := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_BASELINE_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { control.Close() })
	id, err := typed_id.New(typed_id.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	name := "baseline_" + strings.TrimPrefix(id, "wsp_")
	err, identifier := sqlIdentifier(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.ExecContext(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := control.ExecContext(ctx, "DROP DATABASE "+identifier); err != nil {
			t.Error(err)
		}
	})
	target, err := url.Parse(os.Getenv("GATEHOUSE_TEST_BASELINE_POSTGRES_URL"))
	if err != nil {
		t.Fatal(err)
	}
	target.Path, target.RawPath = "/"+name, ""
	t.Setenv("GATEHOUSE_TEST_BASELINE_DATABASE_POSTGRES_URL", target.String())
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_BASELINE_DATABASE_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store.DB
}

func postgresHistoryIndexes(t *testing.T, db *sql.DB, kind string) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT migration_index FROM gatehouse_schema_migrations WHERE migration_type = $1 ORDER BY installed_rank`, kind)
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

func postgresSchemaDefinitions(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`
		WITH tables AS (
			SELECT c.oid, c.relname, c.relpersistence, c.relrowsecurity, c.relforcerowsecurity, c.reloptions
			FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p')
			AND c.relname LIKE 'gatehouse_%' AND c.relname <> 'gatehouse_schema_migrations'
		), definitions AS (
			SELECT 'table:' || t.relname AS name,
				concat_ws('|', t.relpersistence, t.relrowsecurity, t.relforcerowsecurity, array_to_string(t.reloptions, ',')) AS definition
			FROM tables t
			UNION ALL
			SELECT 'column:' || t.relname || ':' || a.attname AS name,
				concat_ws('|', row_number() OVER (PARTITION BY t.oid ORDER BY a.attnum),
					format_type(a.atttypid, a.atttypmod), a.attnotnull, a.attidentity, a.attgenerated,
					a.attcollation::regcollation, pg_get_expr(d.adbin, d.adrelid)) AS definition
			FROM tables t JOIN pg_attribute a ON a.attrelid = t.oid
			LEFT JOIN pg_attrdef d ON d.adrelid = t.oid AND d.adnum = a.attnum
			WHERE a.attnum > 0 AND NOT a.attisdropped
			UNION ALL
			SELECT 'constraint:' || t.relname || ':' || c.conname, pg_get_constraintdef(c.oid, true) || '|validated=' || c.convalidated
			FROM tables t JOIN pg_constraint c ON c.conrelid = t.oid
			UNION ALL
			SELECT 'index:' || t.relname || ':' || c.relname, pg_get_indexdef(i.indexrelid)
			FROM tables t JOIN pg_index i ON i.indrelid = t.oid JOIN pg_class c ON c.oid = i.indexrelid
			UNION ALL
			SELECT 'trigger:' || t.relname || ':' || g.tgname, pg_get_triggerdef(g.oid, true) || '|enabled=' || g.tgenabled::text
			FROM tables t JOIN pg_trigger g ON g.tgrelid = t.oid WHERE NOT g.tgisinternal
			UNION ALL
			SELECT 'function:' || p.proname || ':' || pg_get_function_identity_arguments(p.oid), pg_get_functiondef(p.oid)
			FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public' AND p.proname LIKE 'gatehouse_%'
		)
		SELECT name || '=' || definition FROM definitions ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var definitions []string
	for rows.Next() {
		var definition string
		if err := rows.Scan(&definition); err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, strings.Join(strings.Fields(definition), " "))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return definitions
}

func assertPostgresSchemaEqual(t *testing.T, got, want []string) {
	t.Helper()
	for index := range min(len(got), len(want)) {
		if got[index] != want[index] {
			t.Fatalf("schema differs at definition %d\ngot:  %s\nwant: %s", index, got[index], want[index])
		}
	}
	if len(got) != len(want) {
		t.Fatalf("schema definition counts differ: got %d, want %d", len(got), len(want))
	}
}
