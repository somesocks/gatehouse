package migrations

import (
	"context"
	"testing"
)

func TestSQLiteProjectGrantRevisionsUpgrade(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 66 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, enabled, revision) VALUES ('prn_00000000000000000000000000', TRUE, 1);
		INSERT INTO gatehouse_projects (workspace, id, enabled, created_at) VALUES ('wsp_00000000000000000000000000', 'prj_00000000000000000000000000', TRUE, '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_project_grants (id, workspace, project, principal, role, enabled) VALUES
			('pgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'prj_00000000000000000000000000', 'prn_00000000000000000000000000', 'manager', TRUE),
			('pgr_00000000000000000000000001', 'wsp_00000000000000000000000000', 'prj_00000000000000000000000000', 'prn_00000000000000000000000000', 'member', FALSE);
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_project_grants WHERE revision = 1 AND ((role = 'manager' AND enabled = TRUE) OR (role = 'member' AND enabled = FALSE))`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("existing grants changed: %d %v", count, err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_project_grants SET revision = 0`); err == nil {
		t.Fatal("project grant accepted a non-positive revision")
	}
}
