package migrations

import (
	"context"
	"testing"
)

func TestSQLiteMigrationV059PreservesSessionGrants(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 59 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-01-01');
		INSERT INTO gatehouse_session_grants (id, workspace, session, principal, role, enabled) VALUES ('sgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', 'manager', FALSE);
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		var role string
		var enabled bool
		var revision int
		if err := db.QueryRow(`SELECT role, enabled, revision FROM gatehouse_session_grants`).Scan(&role, &enabled, &revision); err != nil || role != "manager" || enabled || revision != 1 {
			t.Fatalf("grant upgrade = %s %t %d %v", role, enabled, revision, err)
		}
	}
	if _, err := db.Exec(`UPDATE gatehouse_session_grants SET revision = 0`); err == nil {
		t.Fatal("accepted a nonpositive revision")
	}
}
