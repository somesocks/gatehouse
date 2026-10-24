package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestSQLiteRoleNamesUpgrade(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		t.Run(fmt.Sprintf("baseline=%t", baseline), func(t *testing.T) {
			registry := testSQLiteBaselineRegistry(t)
			if !baseline {
				registry.Baseline = nil
			}
			testRoleNamesUpgrade(t, openConfiguredMigrationTestDatabase(t), registry, migrateSQLite, true)
		})
	}
}

func TestPostgresRoleNamesUpgrade(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_BASELINE_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL databases with CREATE DATABASE permission")
	}
	for _, baseline := range []bool{true, false} {
		t.Run(fmt.Sprintf("baseline=%t", baseline), func(t *testing.T) {
			err, registry := postgresMigrations(config.State{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !baseline {
				registry.Baseline = nil
			}
			testRoleNamesUpgrade(t, newPostgresBaselineTestDatabase(t), registry, migratePostgres, false)
		})
	}
}

func testRoleNamesUpgrade(t *testing.T, db *sql.DB, registry Registry, migrate func(context.Context, *sql.DB, Registry) error, sqlite bool) {
	t.Helper()
	ctx := context.Background()
	registry.Repeatable = nil
	if err := migrate(ctx, db, testSchemaAtVersion(registry, 68)); err != nil {
		t.Fatal(err)
	}
	exec := func(source string) {
		t.Helper()
		if _, err := db.Exec(source); err != nil {
			t.Fatalf("%s: %v", source, err)
		}
	}
	exec(`
		INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000000', 'team', TRUE);
		INSERT INTO gatehouse_principals (id, alias, enabled, revision) VALUES
			('prn_00000000000000000000000000', 'alice', TRUE, 3),
			('prn_00000000000000000000000001', 'bob', FALSE, 4);
		INSERT INTO gatehouse_groups (workspace_id, id, alias, enabled, revision) VALUES
			('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'team', TRUE, 5);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, role, enabled, revision) VALUES
			('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', 'manager', TRUE, 6),
			('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000001', 'member', FALSE, 7);
		INSERT INTO gatehouse_projects (workspace, id, enabled, created_at) VALUES
			('wsp_00000000000000000000000000', 'prj_00000000000000000000000000', TRUE, '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES
			('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES
			('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, enabled, "default") VALUES
			('wsp_00000000000000000000000000', 'wag_00000000000000000000000000', 'bot', 'amd_00000000000000000000000000', 1, TRUE, FALSE);
	`)
	// Historical group workspace grants cannot be created by the current API.
	// Seed their retained rows, then restore the pre-upgrade trigger immediately.
	if sqlite {
		exec(`DROP TRIGGER gatehouse_workspace_grants_principal_insert`)
	} else {
		exec(`ALTER TABLE gatehouse_workspace_grants DISABLE TRIGGER gatehouse_workspace_grants_principal_only`)
	}
	activity := 0
	state := config.State{}
	for _, resource := range []struct{ name, prefix, scope string }{
		{"workspace", "wgr", ""}, {"project", "pgr", ", project"}, {"session", "sgr", ", session"},
	} {
		kinds := []string{"principal", "group"}
		if resource.name == "session" {
			kinds = append(kinds, "agent")
		}
		for kindIndex, kind := range kinds {
			for roleIndex, role := range []string{"member", "contributor", "manager"} {
				id := fmt.Sprintf("%s_%026d", resource.prefix, kindIndex*3+roleIndex)
				subject := map[string]string{"principal": "prn", "group": "grp", "agent": "wag"}[kind] + "_00000000000000000000000000"
				scopeValue := ""
				if resource.scope != "" {
					scopeValue = ", '" + map[string]string{"project": "prj", "session": "ses"}[resource.name] + "_00000000000000000000000000'"
				}
				enabled := roleIndex != 1 && !(resource.name == "workspace" && kind == "group")
				revision := roleIndex + 8
				exec(fmt.Sprintf(`INSERT INTO gatehouse_%s_grants (id, workspace%s, "%s", role, enabled, revision)
					VALUES ('%s', 'wsp_00000000000000000000000000'%s, '%s', '%s', %s, %d)`,
					resource.name, resource.scope, kind, id, scopeValue, subject, role, sqlBool(enabled), revision))
				activity++
				exec(fmt.Sprintf(`INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_%s_grant, created_at)
					VALUES ('act_%026d', '%s_grant.create', '%s_grant', '%s', '2026-01-01T00:00:00Z');
					INSERT INTO gatehouse_activity_event_topics (activity, topic) VALUES ('act_%026d', 'prn_00000000000000000000000000/%s')`,
					resource.name, activity, resource.name, resource.name, id, activity, id))
				if resource.name == "workspace" && kind == "principal" {
					principal := "alice"
					state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{
						WorkspaceID: "team", PrincipalID: &principal,
						Role: []string{"guest", "member", "admin"}[roleIndex], Enabled: enabled, Revision: revision,
					})
				}
			}
		}
	}
	if sqlite {
		exec(`CREATE TRIGGER gatehouse_workspace_grants_principal_insert BEFORE INSERT ON gatehouse_workspace_grants
			WHEN NEW.principal IS NULL OR NEW."group" IS NOT NULL BEGIN SELECT RAISE(ABORT, 'workspace grants require a principal'); END`)
	} else {
		exec(`ALTER TABLE gatehouse_workspace_grants ENABLE TRIGGER gatehouse_workspace_grants_principal_only`)
	}
	exec(`INSERT INTO gatehouse_system_grants (id, principal, role, enabled, revision) VALUES
		('syg_00000000000000000000000000', 'prn_00000000000000000000000000', 'manager', TRUE, 11),
		('syg_00000000000000000000000001', 'prn_00000000000000000000000001', 'manager', FALSE, 12);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_system_grant, created_at)
		VALUES ('act_00000000000000000000000100', 'system_grant.create', 'system_grant', 'syg_00000000000000000000000000', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_member_group, resource_group_member_principal, created_at)
		VALUES ('act_00000000000000000000000101', 'group_member.create', 'group_member', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', '2026-01-01T00:00:00Z');
	`)
	before := roleUpgradeSnapshot(t, db, false)
	want := roleUpgradeSnapshot(t, db, true)
	// Force failure after every role/constraint change, when recording V069.
	if sqlite {
		exec(`CREATE TRIGGER fail_role_history BEFORE INSERT ON gatehouse_schema_migrations
			WHEN NEW.migration_type = 'versioned' AND NEW.migration_index = 69 BEGIN SELECT RAISE(ABORT, 'role test failure'); END`)
	} else {
		exec(`CREATE FUNCTION fail_role_history() RETURNS trigger AS $$ BEGIN
			IF NEW.migration_type = 'versioned' AND NEW.migration_index = 69 THEN RAISE EXCEPTION 'role test failure'; END IF;
			RETURN NEW; END; $$ LANGUAGE plpgsql;
			CREATE TRIGGER fail_role_history BEFORE INSERT ON gatehouse_schema_migrations FOR EACH ROW EXECUTE FUNCTION fail_role_history()`)
	}
	if err := migrate(ctx, db, registry); err == nil || !strings.Contains(err.Error(), "role test failure") {
		t.Fatalf("injected migration failure = %v", err)
	}
	if got := roleUpgradeSnapshot(t, db, false); !reflect.DeepEqual(got, before) {
		t.Fatal("failed rename changed grants or activity history")
	}
	if sqlite {
		exec(`DROP TRIGGER fail_role_history`)
	} else {
		exec(`DROP TRIGGER fail_role_history ON gatehouse_schema_migrations; DROP FUNCTION fail_role_history()`)
	}
	state.SystemGrants = []config.SystemGrant{{PrincipalID: "alice", Enabled: true, Revision: 11}, {PrincipalID: "bob", Enabled: false, Revision: 12}}
	state.Groups = []config.Group{{WorkspaceID: "team", Alias: "team", Enabled: true, Revision: 5, Members: []config.GroupMember{
		{PrincipalID: "alice", Role: "admin", Enabled: true, Revision: 6},
		{PrincipalID: "bob", Role: "member", Enabled: false, Revision: 7},
	}}}
	if sqlite {
		registry.Repeatable = []RepeatableMigration{sqliteMigrationR007ReconcileWorkspaceGrants(state), sqliteMigrationR014ReconcileSystemGrants(state), sqliteMigrationR016ReconcileGroupMembers(state.Groups)}
	} else {
		registry.Repeatable = []RepeatableMigration{postgresMigrationR007ReconcileWorkspaceGrants(state), postgresMigrationR014ReconcileSystemGrants(state), postgresMigrationR016ReconcileGroupMembers(state.Groups)}
	}
	for range 2 {
		if err := migrate(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		if got := roleUpgradeSnapshot(t, db, false); !reflect.DeepEqual(got, want) {
			t.Fatalf("upgrade/reconciliation changed grant identity, metadata, permissions, or history\ngot: %#v\nwant: %#v", got, want)
		}
	}
	for _, source := range []string{
		`UPDATE gatehouse_workspace_grants SET enabled = TRUE WHERE "group" IS NOT NULL`,
		`UPDATE gatehouse_project_grants SET role = 'manager'`,
		`UPDATE gatehouse_session_grants SET role = 'contributor'`,
		`UPDATE gatehouse_group_members SET role = 'guest'`,
		`UPDATE gatehouse_system_grants SET role = 'member'`,
	} {
		if _, err := db.Exec(source); err == nil {
			t.Fatalf("upgraded schema accepted %s", source)
		}
	}
	if sqlite {
		var enabled, violations int
		if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil || enabled != 1 {
			t.Fatalf("foreign keys not restored: %d %v", enabled, err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil || violations != 0 {
			t.Fatalf("foreign key violations: %d %v", violations, err)
		}
	}
}

// Compare every column, not just role counts: history references, recipients,
// disabled rows, IDs, revisions and enabled states must all survive the rename.
func roleUpgradeSnapshot(t *testing.T, db *sql.DB, translate bool) map[string][][]any {
	t.Helper()
	result := map[string][][]any{}
	for _, table := range []string{"workspace_grants", "project_grants", "session_grants", "group_members", "system_grants", "activity_events", "activity_event_topics"} {
		order := "id"
		if table == "group_members" {
			order = "workspace_id, group_id, principal_id"
		} else if table == "activity_event_topics" {
			order = "activity, topic"
		}
		rows, err := db.Query("SELECT * FROM gatehouse_" + table + " ORDER BY " + order)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			for index, column := range columns {
				if bytes, ok := values[index].([]byte); ok {
					values[index] = string(bytes)
				}
				if translate && column == "role" {
					role := values[index].(string)
					if role == "manager" {
						values[index] = "admin"
					} else if table != "group_members" {
						values[index] = map[string]string{"member": "guest", "contributor": "member"}[role]
					}
				}
			}
			result[table] = append(result[table], values)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}
