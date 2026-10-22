package migrations

import (
	"context"
	"testing"
)

func TestSQLiteMultipleWorkspaceAgentDefaultsUpgrade(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 61)
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_groups (workspace_id, id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', TRUE, 5);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES ('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (id, workspace_id, alias, label, model_id, revision, "default", enabled) VALUES
			('wag_00000000000000000000000000', 'wsp_00000000000000000000000000', 'assistant', 'First assistant', 'amd_00000000000000000000000000', 7, TRUE, TRUE),
			('wag_00000000000000000000000001', 'wsp_00000000000000000000000000', 'reviewer', NULL, 'amd_00000000000000000000000000', 4, FALSE, TRUE),
			('wag_00000000000000000000000002', 'wsp_00000000000000000000000000', 'disabled', NULL, 'amd_00000000000000000000000000', 9, FALSE, FALSE);
		INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision) VALUES
			('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'wag_00000000000000000000000000', TRUE, 5),
			('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'wag_00000000000000000000000001', FALSE, 6);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_agent_group, resource_group_agent_agent, created_at) VALUES ('act_00000000000000000000000000', 'group_agent.create', 'group_agent', 'grp_00000000000000000000000000', 'wag_00000000000000000000000000', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_event_topics (activity, topic) VALUES ('act_00000000000000000000000000', 'wsp_00000000000000000000000000/grp_00000000000000000000000000');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_workspace_agents SET "default" = TRUE WHERE alias = 'reviewer'`); err == nil {
		t.Fatal("pre-upgrade schema accepted multiple defaults")
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct {
		id                 string
		revision           int
		isDefault, enabled bool
	}{
		{"wag_00000000000000000000000000", 7, true, true},
		{"wag_00000000000000000000000001", 4, false, true},
		{"wag_00000000000000000000000002", 9, false, false},
	} {
		var revision int
		var isDefault, enabled bool
		if err := db.QueryRow(`SELECT revision, "default", enabled FROM gatehouse_workspace_agents WHERE id = ?`, want.id).Scan(&revision, &isDefault, &enabled); err != nil || revision != want.revision || isDefault != want.isDefault || enabled != want.enabled {
			t.Fatalf("upgrade changed %s: revision %d default %t enabled %t, %v", want.id, revision, isDefault, enabled, err)
		}
	}
	var label string
	if err := db.QueryRow(`SELECT label FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&label); err != nil || label != "First assistant" {
		t.Fatalf("upgrade changed metadata: %q %v", label, err)
	}
	for _, want := range []struct {
		id       string
		revision int
		enabled  bool
	}{
		{"wag_00000000000000000000000000", 5, true}, {"wag_00000000000000000000000001", 6, false},
	} {
		var revision int
		var enabled bool
		if err := db.QueryRow(`SELECT revision, enabled FROM gatehouse_group_agents WHERE agent_id = ?`, want.id).Scan(&revision, &enabled); err != nil || revision != want.revision || enabled != want.enabled {
			t.Fatalf("upgrade changed membership %s: revision %d enabled %t, %v", want.id, revision, enabled, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events events JOIN gatehouse_activity_event_topics topics ON topics.activity = events.id WHERE events.resource_group_agent_agent = 'wag_00000000000000000000000000' AND topics.topic = 'wsp_00000000000000000000000000/grp_00000000000000000000000000'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("upgrade lost membership history: %d %v", count, err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_workspace_agents SET "default" = TRUE WHERE alias = 'reviewer'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_workspace_agents WHERE "default" = TRUE`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("post-upgrade default count = %d %v", count, err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_workspace_agents SET "default" = TRUE WHERE alias = 'disabled'`); err == nil {
		t.Fatal("disabled binding became default")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("upgrade broke a foreign key")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
