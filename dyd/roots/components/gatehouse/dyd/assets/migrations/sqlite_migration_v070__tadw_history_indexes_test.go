package migrations

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestSQLiteMigrationV070TADWHistoryIndexes(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	if err := migrateSQLite(ctx, store.DB, testSchemaAtVersion(registry, 69)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`
 INSERT INTO gatehouse_workspaces(id,enabled) VALUES('wsp_00000000000000000000000000',TRUE);
 INSERT INTO gatehouse_principals(id,revision,enabled) VALUES('prn_00000000000000000000000000',1,TRUE);
 INSERT INTO gatehouse_sessions(workspace,id,author_principal,enabled,created_at)
 VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000','prn_00000000000000000000000000',TRUE,'00');
 `); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, parent, kind, at string }{
		{"sev_00000000000000000000000001", "", "message.text", "00"},
		{"sev_00000000000000000000000002", "sev_00000000000000000000000001", "message.text", "01"},
		{"sev_00000000000000000000000003", "sev_00000000000000000000000001", "message.text", "02"},
		{"sev_00000000000000000000000004", "sev_00000000000000000000000002", "agent.request", "03"},
		{"sev_00000000000000000000000005", "sev_00000000000000000000000004", "agent.success", "04"},
	} {
		var parent any
		if row.parent != "" {
			parent = row.parent
		}
		if _, err := store.Exec(`INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_principal,payload,created_at)
 VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000',?,?,?,'prn_00000000000000000000000000','{"text":"small inline value"}',?)`, row.id, parent, row.kind, row.at); err != nil {
			t.Fatal(err)
		}
	}
	before := sqliteSchemaDefinitions(t, store.DB)
	for range 2 {
		if err := migrateSQLite(ctx, store.DB, testSchemaAtVersion(registry, 70)); err != nil {
			t.Fatal(err)
		}
	}
	after := sqliteSchemaDefinitions(t, store.DB)
	// Exactly two indexes, no table rewrite or additional covering/weight index.
	var added []string
	for _, value := range before {
		if !slices.Contains(after, value) {
			t.Fatalf("migration changed existing schema object %s", value)
		}
	}
	for _, value := range after {
		if !slices.Contains(before, value) {
			added = append(added, value)
		}
	}
	if len(added) != 2 {
		t.Fatalf("new schema objects=%v", added)
	}
	for name, want := range map[string][]string{
		"gatehouse_session_events_thread_order":    {"workspace", "session", "parent", "created_at", "id"},
		"gatehouse_session_events_agent_terminals": {"workspace", "session", "parent", "kind", "created_at", "id"},
	} {
		rows, err := store.Query(`PRAGMA index_info(` + name + `)`)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var seq, cid int
			var column string
			if err := rows.Scan(&seq, &cid, &column); err != nil {
				t.Fatal(err)
			}
			got = append(got, column)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("index %s=%v want=%v", name, got, want)
		}
	}
	var terminalDDL string
	if err := store.QueryRow(`SELECT sql FROM sqlite_schema WHERE name='gatehouse_session_events_agent_terminals'`).Scan(&terminalDDL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(terminalDDL, "WHERE kind IN ('agent.success', 'agent.failure')") {
		t.Fatal("outcome index is not partial")
	}
	// Exercise the real cursor against the fully migrated schema and preexisting events.
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	anchor := model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}, Id: "sev_00000000000000000000000003"}
	err, cursor := store.SessionTADWHistoryCursor(ctx, tx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close()
	var got []string
	for cursor.Next() {
		event := cursor.Event().Event
		if event.Payload["text"] != "small inline value" {
			t.Fatal("migration changed payload")
		}
		got = append(got, event.Ref.Id)
	}
	if err := cursor.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"sev_00000000000000000000000003", "sev_00000000000000000000000005", "sev_00000000000000000000000004", "sev_00000000000000000000000002", "sev_00000000000000000000000001"}
	if !slices.Equal(got, want) {
		t.Fatalf("migrated cursor=%v want=%v", got, want)
	}
}
