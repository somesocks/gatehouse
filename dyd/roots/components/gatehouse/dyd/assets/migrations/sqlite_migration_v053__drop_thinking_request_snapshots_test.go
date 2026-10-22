package migrations

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestSQLiteMigrationV053DropThinkingRequestSnapshots(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 52)
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled)
			VALUES ('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, "default", enabled)
			VALUES ('wsp_00000000000000000000000000', 'wag_00000000000000000000000000', 'test', 'amd_00000000000000000000000000', 1, FALSE, TRUE);
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at)
			VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-10-14T00:00:00.000Z');
	`); err != nil {
		t.Fatal(err)
	}
	previous := map[string]any{"id": "sev_00000000000000000000000000", "session": map[string]any{"id": "ses_00000000000000000000000000", "workspace": map[string]any{"id": "wsp_00000000000000000000000000"}}}
	cases := []struct {
		id, kind      string
		payload, want map[string]any
	}{
		{"sev_00000000000000000000000000", "thinking.request", map[string]any{"turn": 0, "provider_request": map[string]any{"protocol": "builtin", "rate_limits": map[string]any{"workspace_output": map[string]any{"maximum_balance": 100}}, "builtin_parameters": "old"}}, map[string]any{"turn": 0}},
		{"sev_00000000000000000000000001", "thinking.request", map[string]any{"turn": 1, "previous": previous, "provider_request": map[string]any{"protocol": "openai-responses", "responses": map[string]any{"instructions": "old prompt", "input": []any{"old context"}}}}, map[string]any{"turn": 1, "previous": previous}},
		{"sev_00000000000000000000000002", "thinking.request", map[string]any{"turn": 1, "previous": previous, "round_input": map[string]any{"system_prompt": "copied prompt", "messages": []any{"copied context"}}}, map[string]any{"turn": 1, "previous": previous}},
		{"sev_00000000000000000000000003", "thinking.request", map[string]any{"turn": 0, "provider_request": nil, "round_input": nil}, map[string]any{"turn": 0}},
		{"sev_00000000000000000000000004", "thinking.request", map[string]any{"turn": 0}, map[string]any{"turn": 0}},
		{"sev_00000000000000000000000005", "thinking.success", map[string]any{"provider_request": "keep", "round_input": "keep"}, map[string]any{"provider_request": "keep", "round_input": "keep"}},
	}
	for index, test := range cases {
		payload, err := json.Marshal(test.payload)
		if err != nil {
			t.Fatal(err)
		}
		var parent any
		if index > 0 {
			parent = cases[0].id
		}
		if _, err := db.Exec(`INSERT INTO gatehouse_session_events (workspace, session, id, parent, kind, author_agent, payload, metrics, created_at)
			VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', ?, ?, ?, 'wag_00000000000000000000000000', ?, '{"input_tokens":42}', '2026-10-14T00:00:01.000Z')`, test.id, parent, test.kind, string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		for index, test := range cases {
			var payload, kind, author, metrics, stamp string
			var parent any
			if err := db.QueryRow(`SELECT payload, kind, author_agent, parent, metrics, created_at FROM gatehouse_session_events WHERE id = ?`, test.id).Scan(&payload, &kind, &author, &parent, &metrics, &stamp); err != nil {
				t.Fatal(err)
			}
			var actual, want map[string]any
			if err := json.Unmarshal([]byte(payload), &actual); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(test.want)
			if err := json.Unmarshal(encoded, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, want) || kind != test.kind || author != "wag_00000000000000000000000000" || metrics != `{"input_tokens":42}` || stamp != "2026-10-14T00:00:01.000Z" || index == 0 && parent != nil || index > 0 && parent != cases[0].id {
				t.Fatalf("cleanup changed event metadata or retained snapshots: %s %s", test.id, payload)
			}
		}
	}
}
