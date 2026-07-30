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

func TestOpenEphemeralAppliesStrictMigrations(t *testing.T) {
	err, database := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migration history count = %d, want 2", count)
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
	err, first := Open(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`CREATE TABLE test_state (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := Open(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil)
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
	}, nil)
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
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("Open() error = %v, want unsupported PostgreSQL error", err)
	}
}

func TestOpenSQLiteReconcilesWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	err, first := Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, []config.Workspace{{Key: "engineering", Name: "Engineering", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}

	var firstID int64
	if err := first.QueryRow(`SELECT id FROM gatehouse_workspaces WHERE workspace_key = 'engineering'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, []config.Workspace{{Key: "engineering", Name: "Platform Engineering", Enabled: false}})
	if err != nil {
		t.Fatal(err)
	}

	var (
		secondID int64
		name     string
		enabled  bool
	)
	if err := second.QueryRow(`
		SELECT id, name, enabled
		FROM gatehouse_workspaces
		WHERE workspace_key = 'engineering'
	`).Scan(&secondID, &name, &enabled); err != nil {
		t.Fatal(err)
	}
	if secondID != firstID {
		t.Fatalf("workspace id = %d, want %d", secondID, firstID)
	}
	if name != "Platform Engineering" || enabled {
		t.Fatalf("workspace = (%q, %t), want (%q, %t)", name, enabled, "Platform Engineering", false)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	err, third := Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if err := third.QueryRow(`
		SELECT id, name, enabled
		FROM gatehouse_workspaces
		WHERE workspace_key = 'engineering'
	`).Scan(&secondID, &name, &enabled); err != nil {
		t.Fatal(err)
	}
	if secondID != firstID || name != "Platform Engineering" || enabled {
		t.Fatalf("omitted workspace = (%d, %q, %t), want (%d, %q, %t)", secondID, name, enabled, firstID, "Platform Engineering", false)
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
