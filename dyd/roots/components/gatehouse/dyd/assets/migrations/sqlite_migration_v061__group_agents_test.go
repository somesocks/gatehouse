package migrations

import (
	"context"
	"testing"
)

func TestSQLiteGroupAgentUpgradePreservesActivityAndTopics(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 60)
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE), ('wsp_00000000000000000000000001', TRUE);
		INSERT INTO gatehouse_principals (id, enabled, revision) VALUES ('prn_00000000000000000000000000', TRUE, 1);
		INSERT INTO gatehouse_groups (workspace_id, id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', TRUE, 7);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', FALSE, 3);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES ('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (id, workspace_id, alias, model_id, revision, "default", enabled) VALUES ('wag_00000000000000000000000000', 'wsp_00000000000000000000000000', 'assistant', 'amd_00000000000000000000000000', 1, TRUE, TRUE), ('wag_00000000000000000000000001', 'wsp_00000000000000000000000001', 'assistant', 'amd_00000000000000000000000000', 1, FALSE, TRUE);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_member_group, resource_group_member_principal, created_at) VALUES ('act_00000000000000000000000000', 'group_member.create', 'group_member', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_event_topics (activity, topic) VALUES ('act_00000000000000000000000000', 'prn_00000000000000000000000000/grp_00000000000000000000000000');
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	var group, principal, topic string
	if err := db.QueryRow(`SELECT resource_group_member_group, resource_group_member_principal, topic FROM gatehouse_activity_events JOIN gatehouse_activity_event_topics ON activity = id`).Scan(&group, &principal, &topic); err != nil || topic != principal+"/"+group {
		t.Fatalf("history lost in upgrade: %s %s %s %v", group, principal, topic, err)
	}
	var enabled bool
	var revision, count int
	if err := db.QueryRow(`SELECT enabled, revision FROM gatehouse_group_members`).Scan(&enabled, &revision); err != nil || enabled || revision != 3 {
		t.Fatalf("human membership changed: %t %d %v", enabled, revision, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_group_agents`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unexpected backfilled bots: %d %v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'wag_00000000000000000000000001', TRUE, 1)`); err == nil {
		t.Fatal("database accepted cross-workspace membership")
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'wag_00000000000000000000000000', TRUE, 0)`); err == nil {
		t.Fatal("database accepted nonpositive membership revision")
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'wag_00000000000000000000000000', TRUE, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_agent_group, resource_group_agent_agent, created_at) VALUES ('act_00000000000000000000000001', 'group_agent.create', 'group_agent', 'grp_00000000000000000000000000', 'wag_00000000000000000000000000', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_activity_events SET resource_group = 'grp_00000000000000000000000000' WHERE resource_kind = 'group_agent'`); err == nil {
		t.Fatal("activity accepted mixed resource kinds")
	}
	if _, err := db.Exec(`UPDATE gatehouse_activity_events SET resource_group_agent_agent = NULL WHERE resource_kind = 'group_agent'`); err == nil {
		t.Fatal("activity accepted incomplete membership identity")
	}
	if _, err := db.Exec(`UPDATE gatehouse_activity_events SET resource_group_agent_group = 'grp_00000000000000000000000000' WHERE resource_kind = 'group_member'`); err == nil {
		t.Fatal("old activity accepted bot membership targets")
	}
}
