package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestSQLiteGroupAgentReconciliationSourceIsSnapshotIndependent(t *testing.T) {
	migration := sqliteMigrationR015ReconcileGroupAgents([]config.Group{{WorkspaceID: "engineering", Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 3}}}})
	session := &MigrationSession{}
	err, first := migration.Builder(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	err, second := migration.Builder(context.Background(), session)
	if err != nil || first != second {
		t.Fatalf("unstable reconciliation source: %v", err)
	}
}

func sqliteGroupAgentMigrationTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	if err := migrateSQLite(context.Background(), db, registry); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000000', 'team', TRUE), ('wsp_00000000000000000000000001', 'other', TRUE);
		INSERT INTO gatehouse_groups (workspace_id, id, alias, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'developers', TRUE, 1);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES ('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, "default", enabled) VALUES
			('wsp_00000000000000000000000000', 'wag_00000000000000000000000000', 'admin-assistant', 'amd_00000000000000000000000000', 1, FALSE, FALSE),
			('wsp_00000000000000000000000001', 'wag_00000000000000000000000001', 'elsewhere', 'amd_00000000000000000000000000', 1, FALSE, TRUE);
	`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLiteGroupAgentReconciliationReportsUnresolvedAliases(t *testing.T) {
	for _, test := range []struct {
		name, workspace, group, agent, reason string
	}{
		{"workspace", "missing", "developers", "admin-assistant", "workspace is unavailable"},
		{"group", "team", "missing", "admin-assistant", "group is unavailable in this workspace"},
		{"bot", "team", "developers", "missing", "bot is unavailable in this workspace"},
		{"foreign bot", "team", "developers", "elsewhere", "bot is unavailable in this workspace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := sqliteGroupAgentMigrationTestDatabase(t)
			groups := []config.Group{
				{WorkspaceID: "team", Alias: "developers", Agents: []config.GroupAgent{{AgentID: "admin-assistant", Enabled: true, Revision: 1}}},
				{WorkspaceID: test.workspace, Alias: test.group, Agents: []config.GroupAgent{{AgentID: test.agent, Enabled: true, Revision: 1}}},
			}
			migration := sqliteMigrationR015ReconcileGroupAgents(groups)
			err, source := migration.Builder(context.Background(), &MigrationSession{})
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			_, executionErr := tx.Exec(source)
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			for _, detail := range []string{"cannot reconcile group bot membership", "workspace '" + test.workspace + "'", "group '" + test.group + "'", "bot '" + test.agent + "'", test.reason} {
				if executionErr == nil || !strings.Contains(executionErr.Error(), detail) {
					t.Fatalf("migration error = %v, want %q", executionErr, detail)
				}
			}
			for _, table := range []string{"gatehouse_group_agents", "gatehouse_activity_events", "gatehouse_activity_event_topics"} {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("failed reconciliation changed %s: count %d, error %v", table, count, err)
				}
			}
		})
	}
}

func TestSQLiteGroupAgentReconciliationAcceptsAdministrativelyCreatedBots(t *testing.T) {
	db := sqliteGroupAgentMigrationTestDatabase(t)
	migration := sqliteMigrationR015ReconcileGroupAgents([]config.Group{{WorkspaceID: "team", Alias: "developers", Agents: []config.GroupAgent{{AgentID: "admin-assistant", Enabled: true, Revision: 3}}}})
	err, source := migration.Builder(context.Background(), &MigrationSession{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(source); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var enabled bool
	var revision, count int
	if err := db.QueryRow(`SELECT enabled, revision FROM gatehouse_group_agents`).Scan(&enabled, &revision); err != nil || !enabled || revision != 3 {
		t.Fatalf("membership for database-managed disabled bot = %t revision %d, %v", enabled, revision, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'group_agent.create'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replay duplicated activity: count %d, error %v", count, err)
	}
}
