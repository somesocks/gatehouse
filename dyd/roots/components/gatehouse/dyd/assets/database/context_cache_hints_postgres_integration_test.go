package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func newPostgresContextCacheHintFixture(t *testing.T) (*database.Store, database.ContextCacheHintScope) {
	t.Helper()
	store := newPostgresIntegrationTestStore(t)
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: newTypedID(t, typed_id.Workspace)}, Id: newTypedID(t, typed_id.Session)}
	principal := newTypedID(t, typed_id.Principal)
	if _, err := store.Exec(`INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ($1, 'hints', TRUE)`, session.Workspace.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO gatehouse_principals (id, alias, revision, enabled) VALUES ($1, 'publisher', 1, TRUE)`, principal); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES ($1, $2, $3, TRUE, $4)`, session.Workspace.Id, session.Id, principal, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return store, database.ContextCacheHintScope{Session: session, CacheScope: "provider/model", Algorithm: "tadw", Profile: "profile"}
}

// Observe actual server lock waits, rather than assuming a goroutine has
// reached PostgreSQL after a fixed sleep. The database belongs to this test.
func waitForPostgresHintLockWaiters(t *testing.T, ctx context.Context, store *database.Store, want int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		if err := store.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
 WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %d PostgreSQL hint publishers to block: %v", want, ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestPostgresContextCacheHintsConcurrentPublishers(t *testing.T) {
	store, scope := newPostgresContextCacheHintFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	other := scope
	other.Profile = "other-profile"
	if err := store.ContextCacheHintPut(ctx, other, "protected", json.RawMessage(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.ContextCacheHintPut(ctx, scope, "shared", json.RawMessage(`{"publisher":-1}`)); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).UnixMicro()
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_context_cache_hints SET last_used_at=$1 WHERE hint_key='shared'`, future); err != nil {
		t.Fatal(err)
	}

	lock, err := store.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	var session string
	if err := lock.QueryRowContext(ctx, `SELECT id FROM gatehouse_sessions WHERE workspace=$1 AND id=$2 FOR NO KEY UPDATE`, scope.Session.Workspace.Id, scope.Session.Id).Scan(&session); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 2)
	for worker := range 2 {
		go func() {
			payload := json.RawMessage(fmt.Sprintf(`{"publisher":%d}`, worker))
			for i := range 20 {
				if err := store.ContextCacheHintPut(ctx, scope, "shared", payload); err != nil {
					finished <- err
					return
				}
				if err := store.ContextCacheHintPut(ctx, scope, fmt.Sprintf("%d/%02d", worker, i), payload); err != nil {
					finished <- err
					return
				}
			}
			finished <- nil
		}()
	}
	waitForPostgresHintLockWaiters(t, ctx, store, 2)
	if err := lock.Rollback(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}
	hints, err := store.ContextCacheHintsList(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != database.ContextCacheHintsPerScope {
		t.Fatalf("listed %d hints, want %d", len(hints), database.ContextCacheHintsPerScope)
	}
	// Listing has its own LIMIT, so also check the actual stored row count.
	var count int
	if err := store.QueryRowContext(ctx, `SELECT count(*) FROM gatehouse_context_cache_hints
 WHERE workspace=$1 AND session=$2 AND cache_scope=$3 AND algorithm=$4 AND profile=$5`, scope.Session.Workspace.Id, scope.Session.Id, scope.CacheScope, scope.Algorithm, scope.Profile).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != database.ContextCacheHintsPerScope {
		t.Fatalf("stored %d hints, want %d", count, database.ContextCacheHintsPerScope)
	}
	shared := hints[0]
	if shared.Key != "shared" || shared.LastUsedAt != future || (string(shared.Hint) != `{"publisher":0}` && string(shared.Hint) != `{"publisher":1}`) {
		t.Fatalf("concurrent upserts lost the payload or moved recency backward: %+v", shared)
	}
	protected, err := store.ContextCacheHintsList(ctx, other)
	if err != nil || len(protected) != 1 || protected[0].Key != "protected" || string(protected[0].Hint) != `{"version":1}` {
		t.Fatalf("pruning changed another scope: %+v, %v", protected, err)
	}
}

func TestPostgresContextCacheHintsCancelLockWait(t *testing.T) {
	store, scope := newPostgresContextCacheHintFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := store.ContextCacheHintPut(ctx, scope, "key", json.RawMessage(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	before, err := store.ContextCacheHintsList(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := store.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	var session string
	if err := lock.QueryRowContext(ctx, `SELECT id FROM gatehouse_sessions WHERE workspace=$1 AND id=$2 FOR NO KEY UPDATE`, scope.Session.Workspace.Id, scope.Session.Id).Scan(&session); err != nil {
		t.Fatal(err)
	}
	publication, cancelPublication := context.WithCancel(ctx)
	defer cancelPublication()
	finished := make(chan error, 1)
	go func() {
		finished <- store.ContextCacheHintPut(publication, scope, "key", json.RawMessage(`{"version":2}`))
	}()
	waitForPostgresHintLockWaiters(t, ctx, store, 1)
	cancelPublication()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled publication returned %v, want context.Canceled", err)
		}
	case <-ctx.Done():
		t.Fatal("publication did not return while the session lock remained held:", ctx.Err())
	}
	after, err := store.ContextCacheHintsList(ctx, scope)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("canceled publication changed hints: before=%+v, after=%+v, err=%v", before, after, err)
	}
	if err := lock.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := store.ContextCacheHintPut(ctx, scope, "key", json.RawMessage(`{"version":3}`)); err != nil {
		t.Fatal("publication after cancellation failed:", err)
	}
	hints, err := store.ContextCacheHintsList(ctx, scope)
	if err != nil || len(hints) != 1 || hints[0].Key != "key" || string(hints[0].Hint) != `{"version":3}` {
		t.Fatalf("publication after cancellation = %+v, %v", hints, err)
	}
}
