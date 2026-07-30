package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestOpenEphemeralInitializesStrictMigrationHistory(t *testing.T) {
	err, database := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("migration history count = %d, want 0", count)
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum
		) VALUES ('versioned', 'invalid', 'test', zeroblob(32))
	`); err == nil {
		t.Fatal("strict migration history accepted text for migration_index")
	}
}

func TestOpenEphemeralDiscardsStateAfterClose(t *testing.T) {
	context := context.Background()
	err, first := Open(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`CREATE TABLE test_state (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := Open(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Exec(`SELECT * FROM test_state`); err == nil {
		t.Fatal("ephemeral database retained state after close")
	}
}

func TestOpenSQLiteCreatesDatabaseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "gatehouse.db")
	err, database := Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPostgresReportsUnsupported(t *testing.T) {
	err, _ := Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindPostgres,
		URL:  "env:GATEHOUSE_DATABASE_URL",
	})
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("Open() error = %v, want unsupported PostgreSQL error", err)
	}
}

func openMigrationTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}

func openMigrationTestFileDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}
