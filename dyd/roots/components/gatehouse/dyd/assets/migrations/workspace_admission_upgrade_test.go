package migrations

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
)

func TestSQLiteExplicitWorkspaceAdmissionUpgrade(t *testing.T) {
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	testExplicitWorkspaceAdmissionUpgrade(t, db, registry, func(r Registry) error { return migrateSQLite(context.Background(), db, r) }, true)
	root := "root"
	registry.Repeatable = []RepeatableMigration{sqliteMigrationR007ReconcileWorkspaceGrants(config.State{WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "admin", PrincipalID: &root, Role: "admin", Revision: 1, Enabled: true}}})}
	if err := migrateSQLite(context.Background(), db, registry); err != nil {
		t.Fatal(err)
	}
	assertRevokedRootAdmission(t, db)
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("workspace admission migration broke activity history foreign keys")
	}
}

func TestPostgresExplicitWorkspaceAdmissionUpgrade(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_ADMISSION_POSTGRES_URL") == "" {
		t.Skip("requires a disposable PostgreSQL database")
	}
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_ADMISSION_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	testExplicitWorkspaceAdmissionUpgrade(t, store.DB, registry, func(r Registry) error { return migratePostgres(context.Background(), store.DB, r) }, false)
	root := "root"
	registry.Repeatable = []RepeatableMigration{postgresMigrationR007ReconcileWorkspaceGrants(config.State{WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "admin", PrincipalID: &root, Role: "admin", Revision: 1, Enabled: true}}})}
	if err := migratePostgres(context.Background(), store.DB, registry); err != nil {
		t.Fatal(err)
	}
	assertRevokedRootAdmission(t, store.DB)
}

func assertRevokedRootAdmission(t *testing.T, db *sql.DB) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_workspace_grants grants JOIN gatehouse_principals principal ON principal.id = grants.principal WHERE principal.alias = 'root' AND grants.enabled = TRUE`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("default bootstrap resurrected revoked admission: %d %v", count, err)
	}
}

func TestSQLiteRootWorkspaceRoleDowngradeSurvivesBootstrapUpgrade(t *testing.T) {
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	root := "root"
	bootstrap := sqliteMigrationR007ReconcileWorkspaceGrants(config.State{WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "admin", PrincipalID: &root, Role: "admin", Enabled: true, Revision: 1}}})
	testRootWorkspaceRoleDowngradeUpgrade(t, db, registry, bootstrap, func(r Registry) error { return migrateSQLite(context.Background(), db, r) })
}

func TestPostgresRootWorkspaceRoleDowngradeSurvivesBootstrapUpgrade(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_ADMISSION_DOWNGRADE_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_ADMISSION_DOWNGRADE_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := "root"
	bootstrap := postgresMigrationR007ReconcileWorkspaceGrants(config.State{WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "admin", PrincipalID: &root, Role: "admin", Enabled: true, Revision: 1}}})
	testRootWorkspaceRoleDowngradeUpgrade(t, store.DB, registry, bootstrap, func(r Registry) error { return migratePostgres(context.Background(), store.DB, r) })
}

func testRootWorkspaceRoleDowngradeUpgrade(t *testing.T, db *sql.DB, registry Registry, bootstrap RepeatableMigration, migrate func(Registry) error) {
	t.Helper()
	old := testSchemaAtVersion(registry, 62)
	if err := migrate(old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000000', 'admin', TRUE);
		INSERT INTO gatehouse_principals (id, alias, enabled, revision) VALUES ('prn_00000000000000000000000000', 'root', TRUE, 1);
		INSERT INTO gatehouse_groups (workspace_id, id, alias, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'renamed-root', TRUE, 1);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, 1);
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, "group", enabled, revision) VALUES ('wgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'contributor', 'grp_00000000000000000000000000', TRUE, 5);
	`); err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = []RepeatableMigration{bootstrap}
	for range 2 {
		if err := migrate(registry); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Query(`SELECT role FROM gatehouse_workspace_grants WHERE principal = 'prn_00000000000000000000000000' AND enabled = TRUE`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	roles := []string{}
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			t.Fatal(err)
		}
		roles = append(roles, role)
	}
	if len(roles) != 1 || roles[0] != "member" {
		t.Fatalf("bootstrap changed downgraded root roles: %#v", roles)
	}
}

func testExplicitWorkspaceAdmissionUpgrade(t *testing.T, db *sql.DB, registry Registry, migrate func(Registry) error, testRollback bool) {
	t.Helper()
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 62)
	if err := migrate(old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000001', 'admin', TRUE);
		INSERT INTO gatehouse_principals (id, alias, enabled, revision) VALUES
		('prn_00000000000000000000000000', 'alice', TRUE, 1), ('prn_00000000000000000000000001', 'bob', TRUE, 1),
		('prn_00000000000000000000000002', 'disabled-account', FALSE, 1), ('prn_00000000000000000000000003', 'disabled-member', TRUE, 1), ('prn_00000000000000000000000004', 'disabled-group', TRUE, 1), ('prn_00000000000000000000000005', 'root', TRUE, 1);
		INSERT INTO gatehouse_groups (workspace_id, id, alias, enabled, revision) VALUES ('wsp_00000000000000000000000001', 'grp_00000000000000000000000003', 'root', TRUE, 1);
		INSERT INTO gatehouse_groups (workspace_id, id, enabled, revision) VALUES
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', TRUE, 1),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000001', TRUE, 1),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000002', FALSE, 1);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision) VALUES
		('wsp_00000000000000000000000001', 'grp_00000000000000000000000003', 'prn_00000000000000000000000005', FALSE, 3),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, 3),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000001', 'prn_00000000000000000000000000', TRUE, 4),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000001', TRUE, 1),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000001', 'prn_00000000000000000000000001', TRUE, 1),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000002', TRUE, 1),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000003', FALSE, 1),
		('wsp_00000000000000000000000000', 'grp_00000000000000000000000002', 'prn_00000000000000000000000004', TRUE, 1);
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision) VALUES
		('wgr_00000000000000000000000015', 'wsp_00000000000000000000000001', 'manager', NULL, 'grp_00000000000000000000000003', TRUE, 2),
		('wgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'contributor', 'prn_00000000000000000000000000', NULL, TRUE, 9),
		('wgr_00000000000000000000000001', 'wsp_00000000000000000000000000', 'member', 'prn_00000000000000000000000000', NULL, TRUE, 7),
		('wgr_00000000000000000000000002', 'wsp_00000000000000000000000000', 'contributor', 'prn_00000000000000000000000001', NULL, FALSE, 12),
		('wgr_00000000000000000000000010', 'wsp_00000000000000000000000000', 'contributor', NULL, 'grp_00000000000000000000000000', TRUE, 2),
		('wgr_00000000000000000000000011', 'wsp_00000000000000000000000000', 'contributor', NULL, 'grp_00000000000000000000000001', TRUE, 3),
		('wgr_00000000000000000000000012', 'wsp_00000000000000000000000000', 'manager', NULL, 'grp_00000000000000000000000001', TRUE, 4),
		('wgr_00000000000000000000000013', 'wsp_00000000000000000000000000', 'manager', NULL, 'grp_00000000000000000000000000', FALSE, 5),
		('wgr_00000000000000000000000014', 'wsp_00000000000000000000000000', 'manager', NULL, 'grp_00000000000000000000000002', TRUE, 6);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_workspace_grant, created_at) VALUES
		('act_00000000000000000000000000', 'workspace_grant.create', 'workspace_grant', 'wgr_00000000000000000000000010', '2026-01-01T00:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	count := func(query string) int {
		t.Helper()
		var value int
		if err := db.QueryRow(query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if testRollback {
		if _, err := db.Exec(`CREATE TRIGGER fail_admission_activity BEFORE INSERT ON gatehouse_activity_events BEGIN SELECT RAISE(ABORT, 'test activity failure'); END`); err != nil {
			t.Fatal(err)
		}
		if err := migrate(registry); err == nil {
			t.Fatal("migration ignored activity failure")
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_workspace_grants WHERE principal IS NOT NULL AND enabled = TRUE`); got != 2 {
			t.Fatalf("failed migration left direct grants behind: %d", got)
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_workspace_grants WHERE "group" IS NOT NULL AND enabled = TRUE`); got != 5 {
			t.Fatalf("failed migration retired group grants: %d", got)
		}
		if _, err := db.Exec(`DROP TRIGGER fail_admission_activity`); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := migrate(registry); err != nil {
			t.Fatal(err)
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_workspace_grants WHERE principal IS NOT NULL AND enabled = TRUE`); got != 5 {
			t.Fatalf("effective direct grants = %d, want 5", got)
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_workspace_grants WHERE "group" IS NOT NULL AND enabled = TRUE`); got != 0 {
			t.Fatalf("group grants still effective: %d", got)
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_workspace_grants WHERE principal IN ('prn_00000000000000000000000002', 'prn_00000000000000000000000003', 'prn_00000000000000000000000004')`); got != 0 {
			t.Fatalf("disabled paths acquired admission: %d", got)
		}
		if got := count(`SELECT revision FROM gatehouse_workspace_grants WHERE id = 'wgr_00000000000000000000000000'`); got != 9 {
			t.Fatalf("enabled direct grant changed: revision %d", got)
		}
		if got := count(`SELECT revision FROM gatehouse_workspace_grants WHERE id = 'wgr_00000000000000000000000002'`); got != 13 {
			t.Fatalf("disabled direct grant was not restored once: revision %d", got)
		}
		if got := count(`SELECT revision FROM gatehouse_workspace_grants WHERE id = 'wgr_00000000000000000000000013'`); got != 5 {
			t.Fatalf("disabled legacy grant changed: revision %d", got)
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_activity_events`); got != 10 {
			t.Fatalf("migration activity count = %d, want 10", got)
		}
		if got := count(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_workspace_grant = 'wgr_00000000000000000000000010'`); got != 2 {
			t.Fatalf("legacy history lost: %d", got)
		}
	}
	if _, err := db.Exec(`UPDATE gatehouse_workspace_grants SET enabled = TRUE WHERE id = 'wgr_00000000000000000000000010'`); err == nil {
		t.Fatal("retired group grant could be restored")
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_workspace_grants (id, workspace, role, "group", enabled, revision) VALUES ('wgr_00000000000000000000000099', 'wsp_00000000000000000000000000', 'member', 'grp_00000000000000000000000000', FALSE, 1)`); err == nil {
		t.Fatal("new group workspace grant accepted")
	}
}
