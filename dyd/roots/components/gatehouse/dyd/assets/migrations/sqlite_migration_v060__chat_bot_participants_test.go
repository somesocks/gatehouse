package migrations

import (
	"context"
	"fmt"
	"testing"
)

func TestSQLiteMigrationV060PreservesGrantsAndBackfillsOnlyHistoricalBots(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSchemaAtVersion(testSQLiteBaselineRegistry(t), 60)
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 59)
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES ('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-01-01');
		INSERT INTO gatehouse_session_grants (id, workspace, session, principal, role, enabled, revision) VALUES ('sgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', 'manager', FALSE, 7);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_session_grant, created_at) VALUES ('act_00000000000000000000000000', 'session_grant.create', 'session_grant', 'sgr_00000000000000000000000000', '2026-01-01');
	`); err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		if _, err := db.Exec(`INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, enabled, "default") VALUES (?, ?, ?, ?, 1, TRUE, FALSE)`, "wsp_00000000000000000000000000", fmt.Sprintf("wag_%026d", index), fmt.Sprintf("bot-%d", index), "amd_00000000000000000000000000"); err != nil {
			t.Fatal(err)
		}
	}
	for index, payload := range []string{
		`{"agents":["wag_00000000000000000000000000","wag_00000000000000000000000000"]}`,
		`{"targets":[{"ref":{"kind":"agent","id":"wag_00000000000000000000000001"}}]}`,
		`{"agent":"wag_00000000000000000000000002"}`,
	} {
		kind := "message.text"
		if index == 2 {
			kind = "agent.request"
		}
		if _, err := db.Exec(`INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, '2026-01-01')`, "wsp_00000000000000000000000000", "ses_00000000000000000000000000", fmt.Sprintf("sev_%026d", index), kind, "prn_00000000000000000000000000", payload); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_agent, payload, created_at) VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'sev_00000000000000000000000003', 'agent.success', 'wag_00000000000000000000000003', '{}', '2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		var count, revision int
		var enabled bool
		if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_grants WHERE agent IS NOT NULL AND role = 'contributor' AND enabled = TRUE`).Scan(&count); err != nil || count != 4 {
			t.Fatalf("bot backfill = %d, %v", count, err)
		}
		if err := db.QueryRow(`SELECT revision, enabled FROM gatehouse_session_grants WHERE id = 'sgr_00000000000000000000000000'`).Scan(&revision, &enabled); err != nil || revision != 7 || enabled {
			t.Fatalf("old grant changed: %d %t %v", revision, enabled, err)
		}
		rows, err := db.Query(`PRAGMA foreign_key_check`)
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			t.Fatal("migration broke a foreign key")
		}
		rows.Close()
	}
	if _, err := db.Exec(`UPDATE gatehouse_session_grants SET principal = 'prn_00000000000000000000000000' WHERE agent IS NOT NULL`); err == nil {
		t.Fatal("grant accepted multiple subject kinds")
	}
}
