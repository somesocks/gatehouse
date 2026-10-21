package migrations

import (
	"context"
	"testing"
)

func TestSQLiteGroupMemberRolesUpgrade(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 64 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, enabled, revision) VALUES ('prn_00000000000000000000000000', TRUE, 1), ('prn_00000000000000000000000001', TRUE, 1);
		INSERT INTO gatehouse_groups (workspace_id, id, enabled) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision) VALUES
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, 7),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000001', FALSE, 4);
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct {
		principal string
		enabled   bool
		revision  int
	}{
		{"prn_00000000000000000000000000", true, 7}, {"prn_00000000000000000000000001", false, 4},
	} {
		var role string
		var enabled bool
		var revision int
		if err := db.QueryRow(`SELECT role, enabled, revision FROM gatehouse_group_members WHERE principal_id = ?`, want.principal).Scan(&role, &enabled, &revision); err != nil || role != "member" || enabled != want.enabled || revision != want.revision {
			t.Fatalf("upgraded membership: %s %t %d %v", role, enabled, revision, err)
		}
	}
	if _, err := db.Exec(`UPDATE gatehouse_group_members SET role = 'contributor'`); err == nil {
		t.Fatal("group accepted contributor role")
	}
	if _, err := db.Exec(`UPDATE gatehouse_group_members SET role = 'manager'`); err != nil {
		t.Fatal(err)
	}
}
