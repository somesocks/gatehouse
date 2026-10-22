package migrations

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/identity"
)

func TestSQLitePrincipalIdentityActivityMigration(t *testing.T) {
	verifyPrincipalIdentityActivityMigration(t, openConfiguredMigrationTestDatabase(t), testSQLiteBaselineRegistry(t), migrateSQLite)
}

func TestPostgresPrincipalIdentityActivityMigration(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_IDENTITY_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_IDENTITY_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifyPrincipalIdentityActivityMigration(t, store.DB, registry, migratePostgres)
	verifyPostgresPrincipalIdentityAdministration(t, store)
}

func verifyPostgresPrincipalIdentityAdministration(t *testing.T, store *database.Store) {
	t.Helper()
	ctx := context.Background()
	alias, username, password := "new-account", "new-login", "first password"
	err, principal := store.SystemPrincipalCreate(ctx, database.SystemPrincipalCreateInput{Alias: &alias, Enabled: true})
	if err != nil || principal == nil || len(principal.Identities) != 0 {
		t.Fatalf("create principal: %#v %v", principal, err)
	}
	if err, _ := store.SystemPrincipalCreate(ctx, database.SystemPrincipalCreateInput{Alias: &alias, Enabled: true}); !errors.Is(err, database.ErrPrincipalAliasExists) {
		t.Fatalf("duplicate principal: %v", err)
	}
	err, principal = store.SystemPrincipalIdentityCreate(ctx, principal.ID, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil || principal == nil || len(principal.Identities) != 1 {
		t.Fatalf("create login: %#v %v", principal, err)
	}
	login := principal.Identities[0]
	if err, _ := store.SystemPrincipalIdentityCreate(ctx, principal.ID, database.SystemPrincipalIdentityInput{Username: &username, Password: &password}); !errors.Is(err, database.ErrIdentityUsernameExists) {
		t.Fatalf("duplicate username: %v", err)
	}
	verifyPassword := func(want string) {
		t.Helper()
		err, active := store.ActiveIdentityGetByID(ctx, login.ID)
		if err != nil || active == nil {
			t.Fatalf("active identity: %#v %v", active, err)
		}
		verifier := active.Verifiers[0].(map[string]any)["password_verifier"].(string)
		if err, valid := identity.VerifyPassword(verifier, []byte(want)); err != nil || !valid {
			t.Fatalf("password mismatch: %v", err)
		}
	}
	verifyPassword(password)
	username = "renamed-login"
	if err, _ := store.SystemPrincipalIdentityUpdate(ctx, principal.ID, login.ID, 1, database.SystemPrincipalIdentityInput{Username: &username}); err != nil {
		t.Fatal(err)
	}
	verifyPassword(password)
	password = "replacement password"
	if err, _ := store.SystemPrincipalIdentityUpdate(ctx, principal.ID, login.ID, 2, database.SystemPrincipalIdentityInput{Password: &password}); err != nil {
		t.Fatal(err)
	}
	verifyPassword(password)
	otherUsername := "other-login"
	if err, _ := store.SystemPrincipalIdentityCreate(ctx, principal.ID, database.SystemPrincipalIdentityInput{Username: &otherUsername, Password: &password}); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemPrincipalIdentityUpdate(ctx, principal.ID, login.ID, 3, database.SystemPrincipalIdentityInput{Username: &otherUsername}); !errors.Is(err, database.ErrIdentityUsernameExists) {
		t.Fatalf("conflicting rename: %v", err)
	}
	if err, _ := store.SystemPrincipalIdentityRemove(ctx, principal.ID, login.ID, 2); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err, _ := store.SystemPrincipalIdentityRemove(ctx, principal.ID, login.ID, 3); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemPrincipalIdentityCreate(ctx, principal.ID, database.SystemPrincipalIdentityInput{Username: &username, Password: &password}); err != nil {
		t.Fatalf("reuse deleted username: %v", err)
	}
	var count int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE detail_identity = $1 AND resource_principal = $2`, login.ID, principal.ID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("deleted login history: %d %v", count, err)
	}
}

func verifyPrincipalIdentityActivityMigration(t *testing.T, db *sql.DB, registry Registry, migrate func(context.Context, *sql.DB, Registry) error) {
	t.Helper()
	ctx := context.Background()
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 56)
	if err := migrate(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000001', 1, TRUE);
		INSERT INTO gatehouse_identities (id, key, principal_id, verifiers, enabled, revision)
		VALUES ('idt_00000000000000000000000000', 'gatehouse:original', 'prn_00000000000000000000000000', '[{"kind":"password","password_verifier":"gh-ver:test"}]', TRUE, 1);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_identity, created_at)
		VALUES ('act_00000000000000000000000000', 'identity.create', 'identity', 'idt_00000000000000000000000000', '2026-10-16T00:00:00Z');
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		VALUES ('act_00000000000000000000000000', 'prn_00000000000000000000000000/idt_00000000000000000000000000');
		UPDATE gatehouse_identities SET principal_id = 'prn_00000000000000000000000001';
	`); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := migrate(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`DELETE FROM gatehouse_identities`); err != nil {
		t.Fatal(err)
	}
	var kind, principal, identity, topic string
	if err := db.QueryRow(`SELECT resource_kind, resource_principal, detail_identity, topic FROM gatehouse_activity_events JOIN gatehouse_activity_event_topics ON activity = id`).Scan(&kind, &principal, &identity, &topic); err != nil {
		t.Fatal(err)
	}
	if kind != "principal" || principal != "prn_00000000000000000000000000" || identity != "idt_00000000000000000000000000" || topic != principal+"/"+identity {
		t.Fatalf("migrated history = %s %s %s %s", kind, principal, identity, topic)
	}
	if _, err := db.Exec(`DELETE FROM gatehouse_principals`); err == nil {
		t.Fatal("activity lost its principal foreign key")
	}
}
