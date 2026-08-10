package database_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
)

func TestMigrateAppliesConfiguredMigrations(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateState(context.Background(), database, configuration, config.State{}); err != nil {
		t.Fatal(err)
	}

	var workspaceCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_workspaces`).Scan(&workspaceCount); err != nil {
		t.Fatal(err)
	}
	if workspaceCount != 1 {
		t.Fatalf("workspace count = %d, want 1", workspaceCount)
	}
	for _, table := range []string{"gatehouse_agent_providers", "gatehouse_agent_models", "gatehouse_workspace_agents"} {
		if _, err := database.Exec(`SELECT * FROM ` + table + ` LIMIT 0`); err != nil {
			t.Fatalf("agent table %q is unavailable: %v", table, err)
		}
	}
}

func TestMigrateWithConfiguredRepeatablesAppliesStrictMigrations(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	if err := migrateState(context.Background(), database, configuration, config.State{}); err != nil {
		t.Fatalf("Migrate() after runtime repeatables: %v", err)
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
			migration_type, migration_index, description, checksum, applied_at
		) VALUES ('versioned', 'invalid', 'test', zeroblob(32), '2026-01-01T00:00:00.000Z')
	`); err == nil {
		t.Fatal("strict migration history accepted text for migration_index")
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
	err, database := database.Open(context.Background(), config.DatabaseConfig{
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
	err, _ := database.Open(context.Background(), config.DatabaseConfig{
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
	}}
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, principals)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: "gatehouse:alice", Principal: model.PrincipalRef{Id: "alice"}, Revision: 1, Verifiers: []interface{}{"gh-ver:first"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	updated := []config.Principal{{
		ID:      "alice",
		Name:    stringPointer("Alice Example"),
		Enabled: false,
	}}
	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, updated)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: "gatehouse:alice", Principal: model.PrincipalRef{Id: "alice"}, Revision: 2, Verifiers: []interface{}{"gh-ver:second"}, Enabled: false,
	}}); err != nil {
		t.Fatal(err)
	}

	var (
		principalName string
		principalOn   bool
		identityOwner string
		verifiers     string
		identityOn    bool
		revision      int
	)
	if err := second.QueryRow(`
		SELECT p.name, p.enabled, i.principal_id, i.verifiers, i.enabled, i.revision
		FROM gatehouse_principals AS p
		JOIN gatehouse_identities AS i ON i.principal_id = p.id
		WHERE p.id = 'alice' AND i.id = 'gatehouse:alice'
	`).Scan(&principalName, &principalOn, &identityOwner, &verifiers, &identityOn, &revision); err != nil {
		t.Fatal(err)
	}
	if principalName != "Alice Example" || principalOn || identityOwner != "alice" || verifiers != `["gh-ver:second"]` || identityOn || revision != 2 {
		t.Fatalf("reconciled principal and identity = (%q, %t, %q, %q, %t, %d)", principalName, principalOn, identityOwner, verifiers, identityOn, revision)
	}
	if err := second.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: "gatehouse:alice", Principal: model.PrincipalRef{Id: "alice"}, Revision: 1, Verifiers: []interface{}{"gh-ver:older"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := second.QueryRow(`SELECT verifiers, enabled, revision FROM gatehouse_identities WHERE id = 'gatehouse:alice'`).Scan(&verifiers, &identityOn, &revision); err != nil {
		t.Fatal(err)
	}
	if verifiers != `["gh-ver:second"]` || identityOn || revision != 2 {
		t.Fatalf("lower revision replaced identity = (%q, %t, %d)", verifiers, identityOn, revision)
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
			ID:       "gatehouse:alice",
			Revision: 1,
			Verifiers: []config.Verifier{{Value: stringPointer("gh-ver:configured"), Stored: "gh-ver:configured"}},
			Enabled:  true,
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
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	state := config.State{
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{{ID: "alice", Enabled: true}},
	}
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
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
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
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

func TestMigrateSQLiteReconcilesToolsResourcesAndGroupGrants(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	state := config.State{
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Tools: []config.Tool{{
			WorkspaceID: "engineering",
			ID:          "github",
			Source:      "file:./tools/github.lisp",
			Enabled:     true,
		}},
		Resources: []config.Resource{
			{WorkspaceID: "engineering", ID: "github-url", Source: "file:./resources/github-url", Secret: false, Enabled: true},
			{WorkspaceID: "engineering", ID: "github-token", Source: "env:GITHUB_TOKEN", Secret: true, Enabled: true},
		},
		Groups: []config.Group{{
			WorkspaceID: "engineering",
			ID:          "developers",
			Enabled:     true,
			ToolGrants: []config.GroupToolGrant{{
				ToolID: "github", Enabled: true,
			}},
			ResourceGrants: []config.GroupResourceGrant{
				{ResourceID: "github-url", Enabled: true},
				{ResourceID: "github-token", Enabled: false},
			},
		}},
	}
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
		t.Fatal(err)
	}

	var (
		toolRef          string
		toolEnabled      bool
		resourceRef      string
		resourceSecret   bool
		resourceEnabled  bool
		grantEnabled     bool
		resourceGrantOn  bool
	)
	if err := database.QueryRow(`
		SELECT source, enabled FROM gatehouse_tools
		WHERE workspace_id = 'engineering' AND id = 'github'
	`).Scan(&toolRef, &toolEnabled); err != nil {
		t.Fatal(err)
	}
	if toolRef != "file:./tools/github.lisp" || !toolEnabled {
		t.Fatalf("tool = (%q, %t), want (%q, %t)", toolRef, toolEnabled, "file:./tools/github.lisp", true)
	}
	if err := database.QueryRow(`
		SELECT source, secret, enabled FROM gatehouse_resources
		WHERE workspace_id = 'engineering' AND id = 'github-token'
	`).Scan(&resourceRef, &resourceSecret, &resourceEnabled); err != nil {
		t.Fatal(err)
	}
	if resourceRef != "env:GITHUB_TOKEN" || !resourceSecret || !resourceEnabled {
		t.Fatalf("resource = (%q, %t, %t), want (%q, %t, %t)", resourceRef, resourceSecret, resourceEnabled, "env:GITHUB_TOKEN", true, true)
	}
	if err := database.QueryRow(`
		SELECT enabled FROM gatehouse_group_tool_grants
		WHERE workspace_id = 'engineering' AND group_id = 'developers' AND tool_id = 'github'
	`).Scan(&grantEnabled); err != nil {
		t.Fatal(err)
	}
	if !grantEnabled {
		t.Fatal("tool grant is disabled, want enabled")
	}
	if err := database.QueryRow(`
		SELECT enabled FROM gatehouse_group_resource_grants
		WHERE workspace_id = 'engineering' AND group_id = 'developers' AND resource_id = 'github-token'
	`).Scan(&resourceGrantOn); err != nil {
		t.Fatal(err)
	}
	if resourceGrantOn {
		t.Fatal("resource grant is enabled, want disabled")
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_group_tool_grants (workspace_id, group_id, tool_id, enabled)
		VALUES ('engineering', 'developers', 'unknown', TRUE)
	`); err == nil {
		t.Fatal("tool grant without a tool was accepted")
	}
	if _, err := database.Exec(`
		INSERT INTO gatehouse_group_resource_grants (workspace_id, group_id, resource_id, enabled)
		VALUES ('engineering', 'developers', 'unknown', TRUE)
	`); err == nil {
		t.Fatal("resource grant without a resource was accepted")
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_tools (workspace_id, id, source, enabled)
		VALUES ('engineering', 'runtime-tool', 'file:./tools/runtime.lisp', TRUE);
		INSERT INTO gatehouse_resources (workspace_id, id, source, secret, enabled)
		VALUES ('engineering', 'runtime-resource', 'env:RUNTIME_RESOURCE', TRUE, TRUE);
	`); err != nil {
		t.Fatal(err)
	}
	state.Tools[0].Source = "file:./tools/github-v2.lisp"
	state.Tools[0].Enabled = false
	state.Resources[1].Enabled = false
	state.Groups[0].ToolGrants[0].Enabled = false
	state.Groups[0].ResourceGrants[1].Enabled = true
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
		t.Fatal(err)
	}

	if err := database.QueryRow(`
		SELECT source, enabled FROM gatehouse_tools
		WHERE workspace_id = 'engineering' AND id = 'github'
	`).Scan(&toolRef, &toolEnabled); err != nil {
		t.Fatal(err)
	}
	if toolRef != "file:./tools/github-v2.lisp" || toolEnabled {
		t.Fatalf("updated tool = (%q, %t), want (%q, %t)", toolRef, toolEnabled, "file:./tools/github-v2.lisp", false)
	}
	if err := database.QueryRow(`
		SELECT enabled FROM gatehouse_group_tool_grants
		WHERE workspace_id = 'engineering' AND group_id = 'developers' AND tool_id = 'github'
	`).Scan(&grantEnabled); err != nil {
		t.Fatal(err)
	}
	if grantEnabled {
		t.Fatal("updated tool grant is enabled, want disabled")
	}
	if err := database.QueryRow(`
		SELECT enabled FROM gatehouse_group_resource_grants
		WHERE workspace_id = 'engineering' AND group_id = 'developers' AND resource_id = 'github-token'
	`).Scan(&resourceGrantOn); err != nil {
		t.Fatal(err)
	}
	if !resourceGrantOn {
		t.Fatal("updated resource grant is disabled, want enabled")
	}
	var runtimeCount int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM gatehouse_tools
		WHERE workspace_id = 'engineering' AND id = 'runtime-tool'
	`).Scan(&runtimeCount); err != nil {
		t.Fatal(err)
	}
	if runtimeCount != 1 {
		t.Fatalf("runtime tool count = %d, want 1", runtimeCount)
	}
}

func TestMigrateSQLiteEnforcesKeychainConstraints(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateState(context.Background(), database, configuration, config.State{}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('default', 1, 'gh-kdf:salt?alg=pbkdf2-hmac-sha256-v1', 'gh-enc:payload?alg=aes128-gcm-v1', TRUE)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('default', 2, 'gh-kdf:salt?alg=pbkdf2-hmac-sha256-v1', 'gh-enc:payload?alg=aes128-gcm-v1', TRUE)
	`); err != nil {
		t.Fatalf("second enabled keychain version was rejected: %v", err)
	}

	invalid := []string{
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('Default', 3, 'kdf', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 0, 'kdf', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', -1, 'kdf', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 3, ' ', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 3, 'kdf', ' ', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 3, 'kdf', 'key', 2)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 1, 'kdf', 'key', TRUE)`,
	}
	for _, statement := range invalid {
		if _, err := database.Exec(statement); err == nil {
			t.Fatalf("invalid keychain row was accepted: %s", statement)
		}
	}
}

func TestKeychainsInsertAndGet(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	keychains := []model.Keychain{
		{
			Ref:     model.KeychainRef{Id: "default", Version: 1},
			KekKdf:  "gh-kdf:default?alg=pbkdf2-hmac-sha256-v1",
			Key:     "gh-enc:default?alg=aes128-gcm-v1",
			Enabled: true,
		},
		{
			Ref:     model.KeychainRef{Id: "alpha", Version: 1},
			KekKdf:  "gh-kdf:alpha?alg=pbkdf2-hmac-sha256-v1",
			Key:     "gh-enc:alpha?alg=aes128-gcm-v1",
			Enabled: true,
		},
	}
	if err := database.KeychainsInsert(context.Background(), keychains); err != nil {
		t.Fatal(err)
	}
	if err := database.KeychainsInsert(context.Background(), []model.Keychain{{
		Ref:     model.KeychainRef{Id: "default", Version: 1},
		KekKdf:  "replacement",
		Key:     "replacement",
		Enabled: false,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := database.KeychainsInsert(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	err, got := database.KeychainsGet(context.Background(), []model.KeychainRef{
		{Id: "default", Version: 1},
		{Id: "missing", Version: 1},
		{Id: "alpha", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []model.Keychain{keychains[1], keychains[0]}) {
		t.Fatalf("KeychainsGet() = %#v, want %#v", got, []model.Keychain{keychains[1], keychains[0]})
	}
	err, got = database.KeychainsGet(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("KeychainsGet(nil) = %#v, want no keychains", got)
	}
}

func TestKeychainsGetCurrent(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	keychains := []model.Keychain{
		{Ref: model.KeychainRef{Id: "alpha", Version: 1}, KekKdf: "alpha-1", Key: "alpha-1", Enabled: true},
		{Ref: model.KeychainRef{Id: "alpha", Version: 2}, KekKdf: "alpha-2", Key: "alpha-2", Enabled: true},
		{Ref: model.KeychainRef{Id: "alpha", Version: 3}, KekKdf: "alpha-3", Key: "alpha-3", Enabled: false},
		{Ref: model.KeychainRef{Id: "default", Version: 1}, KekKdf: "default-1", Key: "default-1", Enabled: false},
	}
	if err := database.KeychainsInsert(context.Background(), keychains); err != nil {
		t.Fatal(err)
	}

	err, got := database.KeychainsGetCurrent(context.Background(), []string{"default", "missing", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Keychain{keychains[1]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KeychainsGetCurrent() = %#v, want %#v", got, want)
	}
	err, got = database.KeychainsGetCurrent(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("KeychainsGetCurrent(nil) = %#v, want no keychains", got)
	}
}

func stringPointer(value string) *string {
	return &value
}

func openConfigured(ctx context.Context, configuration config.DatabaseConfig, workspaces []config.Workspace, principals []config.Principal) (error, *database.Store) {
	err, database := database.Open(ctx, configuration)
	if err != nil {
		return err, nil
	}
	if err := migrateState(ctx, database, configuration, config.State{Workspaces: workspaces, Principals: principals}); err != nil {
		database.Close()
		return err, nil
	}
	return nil, database
}

func migrateState(ctx context.Context, store *database.Store, configuration config.DatabaseConfig, state config.State) error {
	err, keyring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		return err
	}
	defer keyring.Close()
	err, set := migrations.Build(configuration, state, keyring)
	if err != nil {
		return err
	}
	return migrations.Run(ctx, store, set)
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
