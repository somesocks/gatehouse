package migrations

import (
	"context"
	"testing"
)

func TestSQLiteMigrationV058PreservesGroupsAndMemberships(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 58 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'existing', 'Existing group', FALSE);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', FALSE);
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		var name string
		var enabled bool
		var revision int
		if err := db.QueryRow(`SELECT name, enabled, revision FROM gatehouse_groups WHERE alias = 'existing'`).Scan(&name, &enabled, &revision); err != nil || name != "Existing group" || enabled || revision != 1 {
			t.Fatalf("group upgrade = %s %t %d %v", name, enabled, revision, err)
		}
		if err := db.QueryRow(`SELECT enabled, revision FROM gatehouse_group_members`).Scan(&enabled, &revision); err != nil || enabled || revision != 1 {
			t.Fatalf("membership upgrade = %t %d %v", enabled, revision, err)
		}
	}
	for _, table := range []string{"gatehouse_groups", "gatehouse_group_members"} {
		if _, err := db.Exec(`UPDATE ` + table + ` SET revision = 0`); err == nil {
			t.Fatalf("%s accepted a nonpositive revision", table)
		}
	}
}
