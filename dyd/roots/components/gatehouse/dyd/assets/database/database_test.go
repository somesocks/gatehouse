package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestOpenEphemeralMigratesDatabase(t *testing.T) {
	database, err := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRow(`SELECT version FROM gatehouse_schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("migration version = %d, want 1", version)
	}
}

func TestOpenEphemeralDiscardsStateAfterClose(t *testing.T) {
	context := context.Background()
	first, err := Open(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`CREATE TABLE test_state (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
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
	database, err := Open(context.Background(), config.DatabaseConfig{
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
	_, err := Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindPostgres,
		URL:  "env:GATEHOUSE_DATABASE_URL",
	})
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("Open() error = %v, want unsupported PostgreSQL error", err)
	}
}
