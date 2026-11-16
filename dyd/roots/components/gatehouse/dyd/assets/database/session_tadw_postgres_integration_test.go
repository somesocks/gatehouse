package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestPostgresTADWHistoryCursorIntegration(t *testing.T) {
	store := newPostgresIntegrationTestStore(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	// A history read owns the only SQL connection, including file hydration.
	store.SetMaxOpenConns(1)
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: newTypedID(t, typed_id.Workspace)}, Id: newTypedID(t, typed_id.Session)}
	principal := newTypedID(t, typed_id.Principal)
	provider := newTypedID(t, typed_id.StorageProvider)
	object := newTypedID(t, typed_id.StorageObject)
	file := newTypedID(t, typed_id.SessionFile)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ($1, 'history', TRUE)`, session.Workspace.Id)
	exec(`INSERT INTO gatehouse_principals (id, alias, revision, enabled) VALUES ($1, 'reader', 1, TRUE)`, principal)
	exec(`INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES ($1, $2, $3, TRUE, $4)`, session.Workspace.Id, session.Id, principal, at)
	exec(`INSERT INTO gatehouse_storage_providers (id, alias, revision, protocol, enabled) VALUES ($1, 'history-files', 1, 'embedded', TRUE)`, provider)
	exec(`INSERT INTO gatehouse_storage_objects (id, provider, object, state, sha256, size, created_at)
 VALUES ($1, $2, 'notes', 'success', decode(repeat('00', 32), 'hex'), $3, $4)`, object, provider, int64(9007199254740993), at)
	exec(`INSERT INTO gatehouse_session_files (workspace, session, id, storage_object, name, media_type, enabled, created_at)
 VALUES ($1, $2, $3, $4, 'notes.xml', 'text/xml', FALSE, $5)`, session.Workspace.Id, session.Id, file, object, at)

	id := func(n int) string { return fmt.Sprintf("sev_%026d", n) }
	// Anchor 11 has an eligible same-time higher-ID sibling (12). Owner 3
	// previews only requests 4/5/6, with outcomes 8/9 outside those slots.
	// Newer siblings 13/14 and the anchor's generic children 16/17 are excluded.
	nodes := []struct {
		id, parent, second int
		kind               string
	}{
		{1, 0, 0, "message.text"},
		{2, 1, 2, "agent.request"},
		{3, 1, 4, "message.text"},
		{4, 3, 5, "agent.request"},
		{5, 3, 5, "agent.request"},
		{6, 3, 6, "agent.request"},
		{7, 3, 7, "agent.request"},
		{8, 4, 20, "agent.success"},
		{9, 5, 21, "agent.failure"},
		{10, 7, 22, "agent.failure"},
		{11, 1, 8, "message.text"},
		{12, 1, 8, "message.text"},
		{13, 1, 9, "message.text"},
		{14, 0, 1, "message.text"},
		{15, 0, 0, "message.text"},
		{16, 11, 9, "agent.request"},
		{17, 16, 10, "agent.success"},
	}
	for _, node := range nodes {
		var parent any
		if node.parent != 0 {
			parent = id(node.parent)
		}
		payload := `{"text":"history"}`
		if node.id == 12 {
			payload = fmt.Sprintf(`{"text":"attached","attachments":[%q]}`, file)
		}
		exec(`INSERT INTO gatehouse_session_events (workspace, session, id, parent, kind, author_principal, payload, created_at)
 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, session.Workspace.Id, session.Id, id(node.id), parent, node.kind, principal, payload, at.Add(time.Duration(node.second)*time.Second))
	}

	tx, err := store.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	anchor := model.SessionEventRef{Session: session, Id: id(11)}
	var want []string
	for _, n := range []int{12, 11, 6, 9, 5, 8, 4, 3, 2, 15, 1} {
		want = append(want, id(n))
	}
	// Reopen after early closure, then after exhaustion, on the same transaction.
	for _, limit := range []int{1, len(want) + 1, len(want) + 1} {
		err, cursor := store.SessionTADWHistoryCursor(ctx, tx, anchor)
		if err != nil {
			t.Fatal(err)
		}
		defer cursor.Close()
		var got []string
		for len(got) < limit && cursor.Next() {
			event := cursor.Event().Event
			got = append(got, event.Ref.Id)
			if event.Ref.Id == id(12) {
				err, projected := store.SessionEventFilesHydrateInTransaction(ctx, tx, event)
				if err != nil {
					t.Fatal(err)
				}
				wantFiles := []interface{}{map[string]interface{}{"id": file, "name": "notes.xml", "media_type": "text/xml", "size": int64(9007199254740993)}}
				if !reflect.DeepEqual(projected.Payload["attachments"], wantFiles) {
					t.Fatalf("hydrated attachments = %#v, want %#v", projected.Payload["attachments"], wantFiles)
				}
				if !reflect.DeepEqual(event.Payload["attachments"], []interface{}{file}) {
					t.Fatal("hydration changed canonical attachment IDs")
				}
			}
		}
		if err := cursor.Err(); err != nil {
			t.Fatal(err)
		}
		if err := cursor.Close(); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want[:min(limit, len(want))]) {
			t.Fatalf("limit %d: history = %v, want %v", limit, got, want[:min(limit, len(want))])
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal("cursor closure ended the caller transaction:", err)
	}
}
