package migrations

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
)

func TestPostgresMigrationV053DropThinkingRequestSnapshots(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_THINKING_SNAPSHOTS_POSTGRES_URL") == "" {
		t.Skip("requires a disposable PostgreSQL database separate from the endpoint-upgrade fixture")
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_THINKING_SNAPSHOTS_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	err, registry := postgresMigrations(config.State{}, ring)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 53 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	db := store.DB
	if err := migratePostgres(ctx, db, old); err != nil {
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
		{"sev_00000000000000000000000001", "thinking.request", map[string]any{"turn": 1, "previous": previous, "provider_request": map[string]any{"protocol": "openai-chat-completions", "chat": map[string]any{"messages": []any{"old prompt", "old context"}}}}, map[string]any{"turn": 1, "previous": previous}},
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
			VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', $1, $2, $3, 'wag_00000000000000000000000000', $4, '{"input_tokens":42}', '2026-10-14T00:00:01.000Z')`, test.id, parent, test.kind, string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := migratePostgres(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		for index, test := range cases {
			var payload, kind, author, metrics string
			var parent any
			var stamp time.Time
			if err := db.QueryRow(`SELECT payload, kind, author_agent, parent, metrics, created_at FROM gatehouse_session_events WHERE id = $1`, test.id).Scan(&payload, &kind, &author, &parent, &metrics, &stamp); err != nil {
				t.Fatal(err)
			}
			var actual, want, usage map[string]any
			if err := json.Unmarshal([]byte(payload), &actual); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(test.want)
			if err := json.Unmarshal(encoded, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(metrics), &usage); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, want) || kind != test.kind || author != "wag_00000000000000000000000000" || usage["input_tokens"] != float64(42) || stamp.UTC().Format(time.RFC3339Nano) != "2026-10-14T00:00:01Z" || index == 0 && parent != nil || index > 0 && parent != cases[0].id {
				t.Fatalf("cleanup changed event metadata or retained snapshots: %s %s", test.id, payload)
			}
		}
	}
}
