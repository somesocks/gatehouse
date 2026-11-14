package database

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/model"
)

// Production constraints/index shape, reduced parent schema for collision tests.
const contextCacheHintsTestSchema = `
 CREATE TABLE gatehouse_sessions(workspace TEXT,id TEXT,PRIMARY KEY(workspace,id));
 INSERT INTO gatehouse_sessions VALUES('w','s'),('w','other'),('other','s');
 CREATE TABLE gatehouse_context_cache_hints(
 workspace TEXT NOT NULL, session TEXT NOT NULL,
 cache_scope TEXT NOT NULL CHECK(length(trim(cache_scope))>0 AND length(CAST(cache_scope AS BLOB))<=1024),
 algorithm TEXT NOT NULL CHECK(length(trim(algorithm))>0 AND length(CAST(algorithm AS BLOB))<=1024),
 profile TEXT NOT NULL CHECK(length(trim(profile))>0 AND length(CAST(profile AS BLOB))<=1024),
 hint_key TEXT NOT NULL CHECK(length(trim(hint_key))>0 AND length(CAST(hint_key AS BLOB))<=1024),
 hint TEXT NOT NULL CHECK(length(CAST(hint AS BLOB))<=4096) CHECK(json_valid(hint)) CHECK(json_type(hint)='object'),
 last_used_at INTEGER NOT NULL,
 PRIMARY KEY(workspace,session,cache_scope,algorithm,profile,hint_key),
 FOREIGN KEY(workspace,session) REFERENCES gatehouse_sessions(workspace,id) ON DELETE CASCADE) STRICT;
 CREATE INDEX gatehouse_context_cache_hints_recent ON gatehouse_context_cache_hints
 (workspace,session,cache_scope,algorithm,profile,last_used_at DESC,hint_key DESC);`

func cacheHintScope() ContextCacheHintScope {
	return ContextCacheHintScope{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}, CacheScope: "provider/model", Algorithm: "test-v1", Profile: "profile"}
}
func cacheHintStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hints.sqlite")
	err, store := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.Exec(`PRAGMA journal_mode=WAL;` + contextCacheHintsTestSchema); err != nil {
		t.Fatal(err)
	}
	return store, path
}
func putCacheHint(t *testing.T, store *Store, scope ContextCacheHintScope, key string) {
	t.Helper()
	if err := store.ContextCacheHintPut(context.Background(), scope, key, json.RawMessage(`{"version":1,"id":"`+key+`"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestContextCacheHintsScopeOrderingAndUpsert(t *testing.T) {
	store, _ := cacheHintStore(t)
	scope := cacheHintScope()
	variants := []ContextCacheHintScope{scope, scope, scope, scope, scope, scope}
	variants[1].Session.Workspace.Id = "other"
	variants[2].Session.Id = "other"
	variants[3].CacheScope = "different/model"
	variants[4].Algorithm = "mcmtr-v4"
	variants[5].Profile = "different-profile"
	for i, s := range variants {
		putCacheHint(t, store, s, fmt.Sprint(i))
	}
	for i, s := range variants {
		got, err := store.ContextCacheHintsList(context.Background(), s)
		if err != nil || len(got) != 1 || got[0].Key != fmt.Sprint(i) {
			t.Fatal("scope leak", i, got, err)
		}
	}
	putCacheHint(t, store, scope, "a")
	putCacheHint(t, store, scope, "z")
	if _, err := store.Exec(`UPDATE gatehouse_context_cache_hints SET last_used_at=1`); err != nil {
		t.Fatal(err)
	}
	got, err := store.ContextCacheHintsList(context.Background(), scope)
	if err != nil || len(got) != 3 || got[0].Key != "z" || got[1].Key != "a" || got[2].Key != "0" {
		t.Fatal(got, err)
	}
	replacement := json.RawMessage(`{"version":2,"changed":true}`)
	if err := store.ContextCacheHintPut(context.Background(), scope, "0", replacement); err != nil {
		t.Fatal(err)
	}
	replacement[0] = 'x'
	got, err = store.ContextCacheHintsList(context.Background(), scope)
	if err != nil || len(got) != 3 || got[0].Key != "0" || string(got[0].Hint) != `{"version":2,"changed":true}` {
		t.Fatal(got, err)
	}
	// A late/stale wall clock must not move an existing publication backward.
	if _, err := store.Exec(`UPDATE gatehouse_context_cache_hints SET last_used_at=9223372036854775807 WHERE hint_key='0'`); err != nil {
		t.Fatal(err)
	}
	putCacheHint(t, store, scope, "0")
	got, err = store.ContextCacheHintsList(context.Background(), scope)
	if err != nil || got[0].LastUsedAt != 9223372036854775807 {
		t.Fatal(got, err)
	}
}

func TestContextCacheHintsCapPlanAndAtomicRollback(t *testing.T) {
	store, _ := cacheHintStore(t)
	scope := cacheHintScope()
	other := scope
	other.Profile = "other"
	putCacheHint(t, store, other, "protected")
	for i := 0; i < ContextCacheHintsPerScope+8; i++ {
		putCacheHint(t, store, scope, fmt.Sprintf("%03d", i))
	}
	got, err := store.ContextCacheHintsList(context.Background(), scope)
	if err != nil || len(got) != ContextCacheHintsPerScope {
		t.Fatal(len(got), err)
	}
	var count int
	if err := store.QueryRow(`SELECT count(*) FROM gatehouse_context_cache_hints WHERE profile='profile'`).Scan(&count); err != nil || count != ContextCacheHintsPerScope {
		t.Fatal(count, err)
	}
	rows, err := store.Query(`EXPLAIN QUERY PLAN `+contextCacheHintsListSQL, append(scope.args(), ContextCacheHintsPerScope)...)
	if err != nil {
		t.Fatal(err)
	}
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !strings.Contains(plan, "USING INDEX gatehouse_context_cache_hints_recent") || !strings.Contains(plan, "workspace=? AND session=? AND cache_scope=? AND algorithm=? AND profile=?") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatal(plan)
	}
	// A pruning failure must roll back the preceding upsert as well.
	if _, err := store.Exec(`CREATE TRIGGER fail_hint_prune BEFORE DELETE ON gatehouse_context_cache_hints BEGIN SELECT RAISE(ABORT,'injected pruning failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.ContextCacheHintPut(context.Background(), scope, "new", json.RawMessage(`{}`)); err == nil {
		t.Fatal("prune failure ignored")
	}
	var found int
	if err := store.QueryRow(`SELECT count(*) FROM gatehouse_context_cache_hints WHERE hint_key='new'`).Scan(&found); err != nil || found != 0 {
		t.Fatal(found, err)
	}
	old, err := store.ContextCacheHintsList(context.Background(), other)
	if err != nil || len(old) != 1 || old[0].Key != "protected" {
		t.Fatal(old, err)
	}
}

func TestContextCacheHintsConcurrentPublishers(t *testing.T) {
	store, path := cacheHintStore(t)
	err, second := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errors := make(chan error, 100)
	var wg sync.WaitGroup
	for worker, s := range []*Store{store, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 45; i++ {
				if err := s.ContextCacheHintPut(ctx, cacheHintScope(), fmt.Sprintf("%d/%d", worker, i), json.RawMessage(`{"version":1}`)); err != nil {
					errors <- err
					return
				}
				if err := s.ContextCacheHintPut(ctx, cacheHintScope(), "shared", json.RawMessage(`{"version":1}`)); err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	got, err := store.ContextCacheHintsList(ctx, cacheHintScope())
	if err != nil || len(got) != 32 {
		t.Fatal(len(got), err)
	}
	var count int
	if err := store.QueryRow(`SELECT count(*) FROM gatehouse_context_cache_hints`).Scan(&count); err != nil || count != 32 {
		t.Fatal(count, err)
	}
}

func TestContextCacheHintsUTF8LimitsAndUnsupportedBackend(t *testing.T) {
	store, _ := cacheHintStore(t)
	scope := cacheHintScope()
	scope.Profile = strings.Repeat("é", 512) // 1,024 bytes, not characters.
	payload := json.RawMessage(`{"x":"` + strings.Repeat("é", 2044) + `"}`)
	if len(payload) != ContextCacheHintMaxBytes {
		t.Fatal(len(payload))
	}
	if err := store.ContextCacheHintPut(context.Background(), scope, "key", payload); err != nil {
		t.Fatal(err)
	}
	got, err := store.ContextCacheHintsList(context.Background(), scope)
	if err != nil || len(got) != 1 || string(got[0].Hint) != string(payload) {
		t.Fatal(got, err)
	}
	oversized := json.RawMessage(`{"x":"` + strings.Repeat("é", 2044) + `x"}`)
	if err := store.ContextCacheHintPut(context.Background(), scope, "key", oversized); err == nil {
		t.Fatal("4,097-byte object accepted")
	}
	scope.Profile += "x"
	if _, err := store.ContextCacheHintsList(context.Background(), scope); err == nil {
		t.Fatal("1,025-byte scope accepted")
	}
	unsupported := &Store{kind: config.DatabaseKindPostgres}
	if _, err := unsupported.ContextCacheHintsList(context.Background(), cacheHintScope()); err == nil {
		t.Fatal("unsupported backend accepted")
	}
	if err := unsupported.ContextCacheHintPut(context.Background(), cacheHintScope(), "key", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unsupported backend accepted")
	}
}

func TestContextCacheHintsValidationCancellationAndRelease(t *testing.T) {
	store, _ := cacheHintStore(t)
	scope := cacheHintScope()
	for _, payload := range []string{"", `null`, `[]`, `"text"`, `1`, `invalid`, strings.Repeat(" ", 4096) + `{}`, "{\"x\":\"\xff\"}"} {
		if err := store.ContextCacheHintPut(context.Background(), scope, "key", json.RawMessage(payload)); err == nil {
			t.Fatal("invalid hint accepted", payload)
		}
	}
	if err := store.ContextCacheHintPut(context.Background(), scope, "", json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty key")
	}
	invalid := []ContextCacheHintScope{scope, scope, scope, scope, scope}
	invalid[0].Session.Id = ""
	invalid[1].Session.Workspace.Id = ""
	invalid[2].CacheScope = " "
	invalid[3].Algorithm = ""
	invalid[4].Profile = strings.Repeat("x", 1025)
	for _, s := range invalid {
		if _, err := store.ContextCacheHintsList(context.Background(), s); err == nil {
			t.Fatal("invalid scope")
		}
		if err := store.ContextCacheHintPut(context.Background(), s, "k", json.RawMessage(`{}`)); err == nil {
			t.Fatal("invalid scope")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ContextCacheHintsList(ctx, scope); err == nil {
		t.Fatal("canceled list")
	}
	if err := store.ContextCacheHintPut(ctx, scope, "k", json.RawMessage(`{}`)); err == nil {
		t.Fatal("canceled write")
	}
	missing := scope
	missing.Session.Id = "missing"
	if err := store.ContextCacheHintPut(context.Background(), missing, "k", json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing session accepted")
	}
	if _, err := store.Exec(`DROP TABLE gatehouse_context_cache_hints`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ContextCacheHintsList(context.Background(), scope); err == nil {
		t.Fatal("missing table")
	}
	if err := store.ContextCacheHintPut(context.Background(), scope, "k", json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing table")
	}
	check, cancelCheck := context.WithTimeout(context.Background(), time.Second)
	defer cancelCheck()
	if err := store.PingContext(check); err != nil {
		t.Fatal("connection retained", err)
	}
}
