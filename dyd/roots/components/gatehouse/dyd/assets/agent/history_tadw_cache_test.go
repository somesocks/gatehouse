package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"gatehouse/database"
	"gatehouse/model"
)

// Reduced hint schema for the existing one-connection history fixture. Real
// constraints/FKs and upgrade behavior are covered in database/migration tests.
func tadwCacheTestTable(t *testing.T, store *database.Store) {
	t.Helper()
	if _, err := store.Exec(`CREATE TABLE gatehouse_context_cache_hints(
 workspace TEXT,session TEXT,cache_scope TEXT,algorithm TEXT,profile TEXT,hint_key TEXT,hint TEXT,last_used_at INTEGER,
 PRIMARY KEY(workspace,session,cache_scope,algorithm,profile,hint_key));
 CREATE INDEX gatehouse_context_cache_hints_recent ON gatehouse_context_cache_hints(workspace,session,cache_scope,algorithm,profile,last_used_at DESC,hint_key DESC)`); err != nil {
		t.Fatal(err)
	}
}
func tadwCacheTestScope(session model.SessionRef) tadwHistoryCacheScope {
	return tadwHistoryCacheScope{Session: session, CacheScope: "provider/model/cache-namespace", Profile: "prompt-rendering-profile"}
}

func TestTADWHistoryCacheHintsSelectionAndPublication(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	tadwCacheTestTable(t, store)
	ctx := context.Background()
	scope := tadwCacheTestScope(anchor.Session)
	cfg := tadwAdapterConfig(10000)
	cfg.HistoryMinBytes = 1
	// A fresh lookup with no hints falls back, but never publishes automatically.
	ids, err := loadTADWHistoryCacheHints(ctx, store, scope)
	if err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
	initial, err := loadTADWHistory(ctx, store, anchor, ids, cfg)
	if err != nil || initial.Window.CacheBaseMatched {
		t.Fatal(initial, err)
	}
	records, err := store.ContextCacheHintsList(ctx, scope.databaseScope())
	if err != nil || len(records) != 0 {
		t.Fatal("selection published unused base", records, err)
	}
	oldBase := model.SessionEventRef{Session: anchor.Session, Id: fmt.Sprintf("sev_%026d", 1)}
	if err := publishTADWHistoryCacheHint(ctx, store, scope, oldBase); err != nil {
		t.Fatal(err)
	}
	if err := publishTADWHistoryCacheHint(ctx, store, scope, anchor); err != nil {
		t.Fatal(err)
	}
	// The most recently used hint is not necessarily the best eligible base.
	ids, err = loadTADWHistoryCacheHints(ctx, store, scope)
	if err != nil {
		t.Fatal(err)
	}
	aligned, err := loadTADWHistory(ctx, store, anchor, ids, cfg)
	if err != nil || !aligned.Window.CacheBaseMatched || aligned.Window.BaseEventID != oldBase.Id {
		t.Fatal(aligned, err)
	}
	assertTADWInitialHistory(t, aligned)
	if len(aligned.History) <= len(initial.History) {
		t.Fatal("did not retain aligned older prefix")
	}
	// History selection does not mutate shared hint contents or touch timestamps.
	before, err := store.ContextCacheHintsList(ctx, scope.databaseScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadTADWHistory(ctx, store, anchor, ids, cfg); err != nil {
		t.Fatal(err)
	}
	after, err := store.ContextCacheHintsList(ctx, scope.databaseScope())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("selection touched hints", before, after, err)
	}
}

func TestTADWHistoryCacheHintsStaleMalformedAndIsolation(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	tadwCacheTestTable(t, store)
	ctx := context.Background()
	scope := tadwCacheTestScope(anchor.Session)
	good := fmt.Sprintf("sev_%026d", 1)
	if err := publishTADWHistoryCacheHint(ctx, store, scope, model.SessionEventRef{Session: anchor.Session, Id: good}); err != nil {
		t.Fatal(err)
	}
	// Stale IDs are cheap misses: no point-fetch/hydration is done for hints.
	stale := fmt.Sprintf("sev_%026d", 999)
	if err := publishTADWHistoryCacheHint(ctx, store, scope, model.SessionEventRef{Session: anchor.Session, Id: stale}); err != nil {
		t.Fatal(err)
	}
	for i, raw := range []string{`{}`, `{"version":2,"base_event_id":"` + good + `"}`, `{"version":1,"base_event_id":"invalid"}`, `{"version":1,"base_event_id":42}`, `{"version":1,"base_event_id":"` + good + `"}`} {
		if err := store.ContextCacheHintPut(ctx, scope.databaseScope(), fmt.Sprintf("bad-%d", i), json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	other := scope.databaseScope()
	other.Algorithm = "mcmtr-v4"
	if err := store.ContextCacheHintPut(ctx, other, "mcmtr-state", json.RawMessage(`{"version":1,"user_high_from":"unrelated","agent_high_from":"a","tool_high_from":"t"}`)); err != nil {
		t.Fatal(err)
	}
	other = scope.databaseScope()
	other.Profile = "wrong-profile"
	if err := store.ContextCacheHintPut(ctx, other, "other", json.RawMessage(`{"version":1,"base_event_id":"invalid"}`)); err != nil {
		t.Fatal(err)
	}
	ids, err := loadTADWHistoryCacheHints(ctx, store, scope)
	if err != nil || len(ids) != 2 || !slices.Contains(ids, good) || !slices.Contains(ids, stale) {
		t.Fatal(ids, err)
	}
	cfg := tadwAdapterConfig(10000)
	cfg.HistoryMinBytes = 1
	selected, err := loadTADWHistory(ctx, store, anchor, ids, cfg)
	if err != nil || selected.Window.BaseEventID != good || !selected.Window.CacheBaseMatched {
		t.Fatal(selected, err)
	}
	// Only a stale hint: normal fallback, not an attempted event read.
	if _, err := store.Exec(`DELETE FROM gatehouse_context_cache_hints WHERE hint_key<>?`, stale); err != nil {
		t.Fatal(err)
	}
	ids, err = loadTADWHistoryCacheHints(ctx, store, scope)
	if err != nil {
		t.Fatal(err)
	}
	selected, err = loadTADWHistory(ctx, store, anchor, ids, cfg)
	if err != nil || selected.Window.CacheBaseMatched {
		t.Fatal(selected, err)
	}
}

func TestTADWHistoryCacheHintsValidation(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	tadwCacheTestTable(t, store)
	scope := tadwCacheTestScope(anchor.Session)
	wrong := anchor
	wrong.Session.Id = "other"
	for _, base := range []model.SessionEventRef{wrong, {Session: anchor.Session, Id: "invalid"}, {Session: anchor.Session}} {
		if err := publishTADWHistoryCacheHint(context.Background(), nil, scope, base); err == nil {
			t.Fatal("bad base accepted")
		}
	}
	invalid := scope
	invalid.Profile = ""
	if _, err := loadTADWHistoryCacheHints(context.Background(), nil, invalid); err == nil {
		t.Fatal("bad profile")
	}
	if _, err := checkpointTADWHistoryWithCacheHints(nil, nil, wrong, scope, tadwAdapterConfig(100)); err == nil {
		t.Fatal("scope mismatch")
	}
	if _, err := checkpointTADWHistoryWithCacheHints(nil, nil, anchor, scope, tadwWindowConfig{}); err == nil {
		t.Fatal("bad config")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadTADWHistoryCacheHints(ctx, store, scope); err == nil {
		t.Fatal("cancellation")
	}
	if err := publishTADWHistoryCacheHint(ctx, store, scope, anchor); err == nil {
		t.Fatal("cancellation")
	}
}
