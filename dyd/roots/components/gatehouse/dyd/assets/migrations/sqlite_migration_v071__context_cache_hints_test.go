package migrations

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestSQLiteMigrationV071ContextCacheHints(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	if err := migrateSQLite(ctx, store.DB, testSchemaAtVersion(registry, 70)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO gatehouse_workspaces(id,enabled) VALUES('wsp_00000000000000000000000000',TRUE);
 INSERT INTO gatehouse_principals(id,revision,enabled) VALUES('prn_00000000000000000000000000',1,TRUE);
 INSERT INTO gatehouse_sessions(workspace,id,author_principal,enabled,created_at) VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000','prn_00000000000000000000000000',TRUE,'00');
 INSERT INTO gatehouse_session_events(workspace,session,id,kind,author_principal,payload,created_at) VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000','sev_00000000000000000000000001','message.text','prn_00000000000000000000000000','{"text":"preserved"}','00');`); err != nil {
		t.Fatal(err)
	}
	before := sqliteSchemaDefinitions(t, store.DB)
	for range 2 {
		if err := migrateSQLite(ctx, store.DB, registry); err != nil {
			t.Fatal(err)
		}
	}
	after := sqliteSchemaDefinitions(t, store.DB)
	for _, old := range before {
		if !slices.Contains(after, old) {
			t.Fatal("changed existing object", old)
		}
	}
	var added []string
	for _, value := range after {
		if !slices.Contains(before, value) {
			added = append(added, value)
		}
	}
	if len(added) != 2 {
		t.Fatal("want one table and one explicit index", added)
	}
	var payload string
	if err := store.QueryRow(`SELECT payload FROM gatehouse_session_events WHERE id='sev_00000000000000000000000001'`).Scan(&payload); err != nil || payload != `{"text":"preserved"}` {
		t.Fatal(payload, err)
	}
	scope := database.ContextCacheHintScope{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}, CacheScope: "provider/model", Algorithm: "tadw-history-v1", Profile: "profile"}
	if err := store.ContextCacheHintPut(ctx, scope, "base", json.RawMessage(`{"version":1,"base_event_id":"stale-is-allowed"}`)); err != nil {
		t.Fatal(err)
	}
	hints, err := store.ContextCacheHintsList(ctx, scope)
	if err != nil || len(hints) != 1 || hints[0].Key != "base" {
		t.Fatal(hints, err)
	}
	// Constraints belong to the real migration, not only API validation.
	for _, bad := range []string{`null`, `[]`, `invalid`, strings.Repeat(" ", 4096) + `{}`} {
		if _, err := store.Exec(`UPDATE gatehouse_context_cache_hints SET hint=?`, bad); err == nil {
			t.Fatal("invalid hint accepted", bad)
		}
	}
	if _, err := store.Exec(`UPDATE gatehouse_context_cache_hints SET profile=' '`); err == nil {
		t.Fatal("blank profile accepted")
	}
	if _, err := store.Exec(`UPDATE gatehouse_context_cache_hints SET session='missing'`); err == nil {
		t.Fatal("foreign session accepted")
	}
	rows, err := store.Query(`PRAGMA index_info(gatehouse_context_cache_hints_recent)`)
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for rows.Next() {
		var seq, cid int
		var name string
		if err := rows.Scan(&seq, &cid, &name); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !reflect.DeepEqual(cols, []string{"workspace", "session", "cache_scope", "algorithm", "profile", "last_used_at", "hint_key"}) {
		t.Fatal(cols)
	}
	// Session lifetime owns hints; no event FK prevents stale-base hints blocking
	// event maintenance. The authoritative event above is removed explicitly first.
	if _, err := store.Exec(`DELETE FROM gatehouse_session_events; DELETE FROM gatehouse_sessions`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.QueryRow(`SELECT count(*) FROM gatehouse_context_cache_hints`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
