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

func TestMigrateAppliesConfiguredMigrations(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	err, migrations := BuildMigrations(configuration, config.State{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), database, migrations); err != nil {
		t.Fatal(err)
	}

	var migrationCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != 7 {
		t.Fatalf("migration history count = %d, want 7", migrationCount)
	}
	var workspaceCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_workspaces`).Scan(&workspaceCount); err != nil {
		t.Fatal(err)
	}
	if workspaceCount != 1 {
		t.Fatalf("workspace count = %d, want 1", workspaceCount)
	}
}

func TestMigrateWithConfiguredRepeatablesAppliesStrictMigrations(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, migrations := BuildMigrations(configuration, config.State{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), database, migrations); err != nil {
		t.Fatalf("Migrate() after runtime repeatables: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 7 {
		t.Fatalf("migration history count = %d, want 7", count)
	}
	var gatehouseName string
	if err := database.QueryRow(`SELECT name FROM gatehouse_workspaces WHERE id = 'gatehouse'`).Scan(&gatehouseName); err != nil {
		t.Fatal(err)
	}
	if gatehouseName != "Gatehouse" {
		t.Fatalf("Gatehouse workspace name = %q, want %q", gatehouseName, "Gatehouse")
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum
		) VALUES ('versioned', 'invalid', 'test', zeroblob(32))
	`); err == nil {
		t.Fatal("strict migration history accepted text for migration_index")
	}
}

func TestSQLiteMigrationsMaterializeRegisteredRepeatables(t *testing.T) {
	err, registry := sqliteMigrations(config.State{})
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Repeatable) != 4 {
		t.Fatalf("repeatable migration count = %d, want 4", len(registry.Repeatable))
	}
	for index, migration := range registry.Repeatable {
		if migration.Template != "" || migration.Source == "" {
			t.Fatalf("repeatable migration %d was not materialized", index)
		}
	}
}

func TestOpenEphemeralDiscardsStateAfterClose(t *testing.T) {
	context := context.Background()
	err, first := openConfigured(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`CREATE TABLE test_state (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := openConfigured(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
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

func TestOpenSQLiteReconcilesWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, []config.Workspace{{ID: "engineering", Name: stringPointer("Engineering"), Enabled: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var firstID string
	if err := first.QueryRow(`SELECT id FROM gatehouse_workspaces WHERE id = 'engineering'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, []config.Workspace{{ID: "engineering", Name: stringPointer("Platform Engineering"), Enabled: false}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var (
		secondID string
		name     string
		enabled  bool
	)
	if err := second.QueryRow(`
		SELECT id, name, enabled
		FROM gatehouse_workspaces
		WHERE id = 'engineering'
	`).Scan(&secondID, &name, &enabled); err != nil {
		t.Fatal(err)
	}
	if secondID != firstID {
		t.Fatalf("workspace id = %q, want %q", secondID, firstID)
	}
	if name != "Platform Engineering" || enabled {
		t.Fatalf("workspace = (%q, %t), want (%q, %t)", name, enabled, "Platform Engineering", false)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	err, third := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if err := third.QueryRow(`
		SELECT id, name, enabled
		FROM gatehouse_workspaces
		WHERE id = 'engineering'
	`).Scan(&secondID, &name, &enabled); err != nil {
		t.Fatal(err)
	}
	if secondID != firstID || name != "Platform Engineering" || enabled {
		t.Fatalf("omitted workspace = (%q, %q, %t), want (%q, %q, %t)", secondID, name, enabled, firstID, "Platform Engineering", false)
	}
}

func TestOpenSQLiteReconcilesUnnamedWorkspace(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, []config.Workspace{{
		ID:      "engineering",
		Enabled: true,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var name sql.NullString
	if err := database.QueryRow(`SELECT name FROM gatehouse_workspaces WHERE id = 'engineering'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name.Valid {
		t.Fatalf("workspace name = %q, want NULL", name.String)
	}
}

func TestOpenSQLiteReconcilesPrincipalsAndIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	principals := []config.Principal{{
		ID:      "alice",
		Name:    stringPointer("Alice"),
		Enabled: true,
		Identities: []config.Identity{{
			ID:        "gatehouse:alice",
			Verifiers: `[{"kind":"argon2id","password_verifier":"first"}]`,
			Enabled:   true,
		}},
	}}
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, principals)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	updated := []config.Principal{{
		ID:      "alice",
		Name:    stringPointer("Alice Example"),
		Enabled: false,
		Identities: []config.Identity{{
			ID:        "gatehouse:alice",
			Verifiers: `[{"kind":"argon2id","password_verifier":"second {{ brace }}"}]`,
			Enabled:   false,
		}},
	}}
	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, updated)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	var (
		principalName string
		principalOn   bool
		identityOwner string
		verifiers     string
		identityOn    bool
	)
	if err := second.QueryRow(`
		SELECT p.name, p.enabled, i.principal_id, i.verifiers, i.enabled
		FROM gatehouse_principals AS p
		JOIN gatehouse_identities AS i ON i.principal_id = p.id
		WHERE p.id = 'alice' AND i.id = 'gatehouse:alice'
	`).Scan(&principalName, &principalOn, &identityOwner, &verifiers, &identityOn); err != nil {
		t.Fatal(err)
	}
	if principalName != "Alice Example" || principalOn || identityOwner != "alice" || verifiers != `[{"kind":"argon2id","password_verifier":"second {{ brace }}"}]` || identityOn {
		t.Fatalf("reconciled principal and identity = (%q, %t, %q, %q, %t)", principalName, principalOn, identityOwner, verifiers, identityOn)
	}

	if _, err := second.Exec(`INSERT INTO gatehouse_identities (id, principal_id, verifiers, enabled) VALUES ('matrix:@unknown:example.org', 'unknown', '[{"kind":"matrix"}]', TRUE)`); err == nil {
		t.Fatal("identity without a principal was accepted")
	}
}

func TestOpenSQLitePreservesUnconfiguredPrincipals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	configured := []config.Principal{{
		ID:      "alice",
		Enabled: true,
		Identities: []config.Identity{{
			ID:        "gatehouse:alice",
			Verifiers: `[{"kind":"argon2id","password_verifier":"configured"}]`,
			Enabled:   true,
		}},
	}}
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, configured)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`
		INSERT INTO gatehouse_principals (id, name, enabled)
		VALUES ('bob', 'Bob', TRUE);
		INSERT INTO gatehouse_identities (id, principal_id, verifiers, enabled)
		VALUES ('matrix:@bob:example.org', 'bob', '[{"kind":"matrix"}]', TRUE);
	`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, configured)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	var count int
	if err := second.QueryRow(`
		SELECT COUNT(*)
		FROM gatehouse_identities
		WHERE id = 'matrix:@bob:example.org' AND principal_id = 'bob'
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("runtime identity count = %d, want 1", count)
	}
}

func TestMigrateSQLiteReconcilesGroupsWithRuntimePrincipal(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	state := config.State{
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{{ID: "alice", Enabled: true}},
	}
	err, migrations := BuildMigrations(configuration, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), database, migrations); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO gatehouse_principals (id, name, enabled) VALUES ('bob', 'Bob', TRUE)`); err != nil {
		t.Fatal(err)
	}

	state.Groups = []config.Group{{
		WorkspaceID: "engineering",
		ID:          "admins",
		Name:        stringPointer("Administrators"),
		Enabled:     true,
		Members: []config.GroupMember{
			{PrincipalID: "alice", Enabled: true},
			{PrincipalID: "bob", Enabled: false},
		},
	}}
	err, migrations = BuildMigrations(configuration, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), database, migrations); err != nil {
		t.Fatal(err)
	}

	var (
		name       string
		enabled    bool
		memberRows int
	)
	if err := database.QueryRow(`
		SELECT g.name, g.enabled, COUNT(m.principal_id)
		FROM gatehouse_groups AS g
		LEFT JOIN gatehouse_group_members AS m
			ON m.workspace_id = g.workspace_id AND m.group_id = g.id
		WHERE g.workspace_id = 'engineering' AND g.id = 'admins'
		GROUP BY g.workspace_id, g.id
	`).Scan(&name, &enabled, &memberRows); err != nil {
		t.Fatal(err)
	}
	if name != "Administrators" || !enabled || memberRows != 2 {
		t.Fatalf("group = (%q, %t, %d members), want (%q, %t, %d members)", name, enabled, memberRows, "Administrators", true, 2)
	}

	var bobEnabled bool
	if err := database.QueryRow(`
		SELECT enabled
		FROM gatehouse_group_members
		WHERE workspace_id = 'engineering' AND group_id = 'admins' AND principal_id = 'bob'
	`).Scan(&bobEnabled); err != nil {
		t.Fatal(err)
	}
	if bobEnabled {
		t.Fatal("runtime principal membership is enabled, want disabled")
	}
	if _, err := database.Exec(`
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled)
		VALUES ('engineering', 'admins', 'unknown', TRUE)
	`); err == nil {
		t.Fatal("membership without a principal was accepted")
	}
}

func stringPointer(value string) *string {
	return &value
}

func openConfigured(ctx context.Context, configuration config.DatabaseConfig, workspaces []config.Workspace, principals []config.Principal) (error, *sql.DB) {
	err, database := Open(ctx, configuration)
	if err != nil {
		return err, nil
	}
	err, migrations := BuildMigrations(configuration, config.State{Workspaces: workspaces, Principals: principals})
	if err != nil {
		database.Close()
		return err, nil
	}
	if err := Migrate(ctx, database, migrations); err != nil {
		database.Close()
		return err, nil
	}
	return nil, database
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
