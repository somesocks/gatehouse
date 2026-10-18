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

func TestPostgresMigrationV054DropToolResultIdentifiers(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_TOOL_RESULTS_POSTGRES_URL") == "" {
		t.Skip("requires a disposable PostgreSQL database for tool-result cleanup")
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_TOOL_RESULTS_POSTGRES_URL"})
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
		if migration.Index < 54 {
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
	cases := []struct {
		id, kind      string
		payload, want map[string]any
	}{
		{"sev_00000000000000000000000000", "tool.request", map[string]any{"name": "lisp", "call_id": "original-call", "code": "(int/add 1 2)", "reason": "test"}, map[string]any{"name": "lisp", "call_id": "original-call", "code": "(int/add 1 2)", "reason": "test"}},
		{"sev_00000000000000000000000001", "tool.success", map[string]any{"name": "lisp", "call_id": "original-call", "output": "3\n"}, map[string]any{"output": "3\n"}},
		{"sev_00000000000000000000000002", "tool.failure", map[string]any{"name": "lisp", "call_id": "original-call", "code": "cancelled", "output": "execution cancelled", "message": "keep summary"}, map[string]any{"code": "cancelled", "output": "execution cancelled", "message": "keep summary"}},
		{"sev_00000000000000000000000003", "tool.failure", map[string]any{"name": "lisp", "output": "legacy failure"}, map[string]any{"output": "legacy failure"}},
		{"sev_00000000000000000000000004", "tool.success", map[string]any{"name": nil, "call_id": nil, "output": "empty identifiers"}, map[string]any{"output": "empty identifiers"}},
		{"sev_00000000000000000000000005", "tool.success", map[string]any{"output": "modern result"}, map[string]any{"output": "modern result"}},
		{"sev_00000000000000000000000006", "gateway.notice", map[string]any{"name": "keep", "call_id": "keep"}, map[string]any{"name": "keep", "call_id": "keep"}},
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
			VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', $1, $2, $3, 'wag_00000000000000000000000000', $4, '{"output_tokens":42}', '2026-10-14T00:00:01.000Z')`, test.id, parent, test.kind, string(payload)); err != nil {
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
			if !reflect.DeepEqual(actual, want) || kind != test.kind || author != "wag_00000000000000000000000000" || usage["output_tokens"] != float64(42) || stamp.UTC().Format(time.RFC3339Nano) != "2026-10-14T00:00:01Z" || index == 0 && parent != nil || index > 0 && parent != cases[0].id {
				t.Fatalf("cleanup changed result data or retained identifiers: %s %s", test.id, payload)
			}
		}
	}
}
