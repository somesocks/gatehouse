package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type tadwSliceSource struct {
	events    []model.SessionEvent
	at, calls int
	exhausted bool
	failure   error
}

func (source *tadwSliceSource) Next() bool {
	source.calls++
	if source.at == len(source.events) {
		source.exhausted = true
		return false
	}
	source.at++
	return true
}
func (source *tadwSliceSource) Event() database.TADWHistoryEvent {
	return database.TADWHistoryEvent{Event: source.events[source.at-1]}
}
func (source *tadwSliceSource) Err() error {
	if source.exhausted {
		return source.failure
	}
	return nil
}
func tadwIdentityHydrate(event model.SessionEvent) (model.SessionEvent, error) { return event, nil }
func tadwAdapterConfig(maximum int64) tadwWindowConfig {
	return tadwWindowConfig{ContextMaxBytes: maximum + 1000, HistoryMaxBytes: maximum, HistoryMinBytes: maximum}
}
func tadwAdapterEvents() []model.SessionEvent {
	message := tadwTestEvent("message.text", map[string]interface{}{"text": "café<&"})
	message.Ref.Id = "m"
	message.Parent = nil
	request := tadwTestEvent("agent.request", map[string]interface{}{"agent": "agent"})
	request.Ref.Id = "q"
	request.Parent = &message.Ref
	success := tadwTestEvent("agent.success", map[string]interface{}{"text": "reply<&"})
	success.Ref.Id = "s"
	success.Parent = &request.Ref
	return []model.SessionEvent{success, request, message}
}
func tadwAdapterRender(t *testing.T, events []model.SessionEvent) []tadwRenderedEvent {
	t.Helper()
	var rendered []tadwRenderedEvent
	for _, event := range events {
		fragment, include, err := renderTADWEvent(event)
		if err != nil || !include {
			t.Fatalf("fixture: %v", err)
		}
		rendered = append(rendered, fragment)
	}
	return rendered
}
func assertTADWInitialHistory(t *testing.T, result tadwInitialHistory) {
	t.Helper()
	var ids []string
	for _, event := range result.History {
		ids = append(ids, event.ID)
	}
	if !slices.Equal(ids, result.Window.EventIDs) || tadwRenderedBytes(result.History) != result.Window.HistoryBytes {
		t.Fatalf("history/accounting mismatch: %+v", result)
	}
}

func TestCollectTADWHistoryExactBoundaries(t *testing.T) {
	events := tadwAdapterEvents()
	fragments := tadwAdapterRender(t, events)
	first, two, total := fragments[0].Bytes(), fragments[0].Bytes()+fragments[1].Bytes(), tadwRenderedBytes(fragments)
	for _, test := range []struct {
		name         string
		quota        int64
		count, calls int
	}{
		{"oversized-head", first - 1, 0, 1}, {"exact-first", first, 1, 1}, {"crossing-second", two - 1, 1, 2},
		{"exact-two", two, 2, 2}, {"crossing-last", total - 1, 2, 3}, {"exact-all", total, 3, 3}, {"exhausted", total + 1, 3, 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &tadwSliceSource{events: events}
			hydrated := 0
			result, err := collectTADWHistory(context.Background(), source, func(event model.SessionEvent) (model.SessionEvent, error) { hydrated++; return event, nil }, nil, tadwAdapterConfig(test.quota))
			if err != nil {
				t.Fatal(err)
			}
			assertTADWInitialHistory(t, result)
			want := slices.Clone(fragments[:test.count])
			slices.Reverse(want)
			if !slices.Equal(result.History, want) || source.calls != test.calls || hydrated != min(test.calls, len(events)) {
				t.Fatalf("history=%v calls=%d hydrated=%d", result.History, source.calls, hydrated)
			}
			if result.Window.ToolingBudgetBytes != test.quota+1000-result.Window.HistoryBytes {
				t.Fatal("wrong tooling budget")
			}
		})
	}
}

func TestCollectTADWHistorySkipsBeforeHydration(t *testing.T) {
	valid := tadwAdapterEvents()[0]
	for _, kind := range []string{"thinking.failure", "approval.success", "input.request", "cancel.success", "unknown", "agent.failure"} {
		skipped := tadwTestEvent(kind, map[string]interface{}{"code": "cancelled", "attachments": 42, "output_file": "not a reference"})
		skipped.Ref.Id = ""
		source := &tadwSliceSource{events: []model.SessionEvent{skipped, valid}}
		hydrated := 0
		result, err := collectTADWHistory(context.Background(), source, func(event model.SessionEvent) (model.SessionEvent, error) {
			hydrated++
			if event.Ref.Id == "" {
				t.Fatal("hydrated skipped kind")
			}
			return event, nil
		}, nil, tadwAdapterConfig(10000))
		if err != nil || hydrated != 1 || len(result.History) != 1 {
			t.Fatalf("kind=%s hydrated=%d err=%v", kind, hydrated, err)
		}
	}
	result, err := collectTADWHistory(context.Background(), &tadwSliceSource{}, tadwIdentityHydrate, nil, tadwAdapterConfig(100))
	if err != nil || len(result.History) != 0 || result.Window.ToolingBudgetBytes != 1100 {
		t.Fatal("empty history", result, err)
	}
}

func TestCollectTADWHistoryUsesWindowPolicy(t *testing.T) {
	events := tadwAdapterEvents()
	fragments := tadwAdapterRender(t, events)
	total := tadwRenderedBytes(fragments)
	cfg := tadwAdapterConfig(total)
	cfg.HistoryMinBytes = 1
	for _, test := range []struct {
		hints   []string
		want    []string
		matched bool
	}{
		{nil, []string{"s"}, false}, {[]string{"stale", "q"}, []string{"q", "s"}, true},
		{[]string{"s", "m", "q"}, []string{"m", "q", "s"}, true},
	} {
		result, err := collectTADWHistory(context.Background(), &tadwSliceSource{events: events}, tadwIdentityHydrate, test.hints, cfg)
		if err != nil {
			t.Fatal(err)
		}
		assertTADWInitialHistory(t, result)
		if !slices.Equal(result.Window.EventIDs, test.want) || result.Window.CacheBaseMatched != test.matched {
			t.Fatal(result.Window)
		}
	}
}

func TestCollectTADWHistoryErrorsReturnNoPartialResult(t *testing.T) {
	events := tadwAdapterEvents()
	sentinel := errors.New("source/hydration failure")
	for _, mode := range []string{"source", "hydrate", "render", "duplicate", "cancel", "config"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source := &tadwSliceSource{events: slices.Clone(events)}
			hydrate := tadwIdentityHydrate
			cfg := tadwAdapterConfig(10000)
			switch mode {
			case "source":
				source.failure = sentinel
			case "hydrate":
				hydrate = func(model.SessionEvent) (model.SessionEvent, error) { return model.SessionEvent{}, sentinel }
			case "render":
				source.events[1].Payload = map[string]interface{}{"agent": 42}
			case "duplicate":
				source.events[1].Ref.Id = source.events[0].Ref.Id
			case "cancel":
				hydrate = func(event model.SessionEvent) (model.SessionEvent, error) { cancel(); return event, nil }
			case "config":
				cfg.HistoryMinBytes = 0
			}
			result, err := collectTADWHistory(ctx, source, hydrate, nil, cfg)
			if err == nil || !reflect.DeepEqual(result, tadwInitialHistory{}) {
				t.Fatalf("partial result: %+v err=%v", result, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if (mode == "source" || mode == "hydrate") && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if mode == "config" && source.calls != 0 {
				t.Fatal("invalid config read history")
			}
		})
	}
}

// No content tables or storage providers exist in this reduced schema. The
// adapter must use the transaction and metadata only, even for large files.
const tadwAdapterSchema = `
 CREATE TABLE gatehouse_principals(id TEXT PRIMARY KEY,alias TEXT,name TEXT,enabled INTEGER);
 INSERT INTO gatehouse_principals VALUES('user',NULL,NULL,1);
 CREATE TABLE gatehouse_session_events(workspace TEXT,session TEXT,id TEXT,parent TEXT,kind TEXT,
 author_principal TEXT,author_agent TEXT,author_gateway TEXT,payload TEXT NOT NULL,created_at TEXT NOT NULL,metrics TEXT,
 PRIMARY KEY(workspace,session,id));
 CREATE INDEX gatehouse_session_events_thread_order ON gatehouse_session_events(workspace,session,parent,created_at DESC,id DESC);
 CREATE INDEX gatehouse_session_events_agent_terminals ON gatehouse_session_events(workspace,session,parent,kind,created_at,id) WHERE kind IN ('agent.success','agent.failure');
 CREATE TABLE gatehouse_storage_objects(id TEXT PRIMARY KEY,size INTEGER);
 CREATE TABLE gatehouse_session_files(workspace TEXT,session TEXT,id TEXT,storage_object TEXT,name TEXT,media_type TEXT,enabled INTEGER,PRIMARY KEY(workspace,session,id));
 INSERT INTO gatehouse_storage_objects VALUES('object',123456789);
 INSERT INTO gatehouse_session_files VALUES('w','s','sfi_00000000000000000000000001','object','résumé<&.txt',NULL,0);
`

func tadwAdapterAt(seconds int) string {
	return time.Date(2026, 11, 1, 0, 0, seconds, 0, time.UTC).Format("2006-01-02T15:04:05.000Z")
}

func tadwAdapterDB(t *testing.T) (*database.Store, model.SessionEventRef) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "history.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.Exec(tadwAdapterSchema); err != nil {
		t.Fatal(err)
	}
	scope := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	ref := func(id int) model.SessionEventRef {
		return model.SessionEventRef{Session: scope, Id: fmt.Sprintf("sev_%026d", id)}
	}
	output := model.SessionFileRef{Session: scope, Id: "sfi_00000000000000000000000001"}
	for _, row := range []struct {
		id, parent int
		kind, at   string
		payload    map[string]interface{}
	}{
		{1, 0, "message.text", tadwAdapterAt(0), map[string]interface{}{"text": "base"}},
		{2, 1, "message.text", tadwAdapterAt(1), map[string]interface{}{"text": "older"}},
		{3, 2, "agent.request", tadwAdapterAt(3), map[string]interface{}{"agent": "agent"}},
		{4, 3, "agent.success", tadwAdapterAt(4), map[string]interface{}{"text": "reply"}},
		{5, 1, "message.text", tadwAdapterAt(2), map[string]interface{}{"text": "anchor café<&", "attachments": []string{output.Id}}},
		{6, 1, "tool.request", "2026-11-01T00:00:01.500Z", map[string]interface{}{"name": "tool", "code": "return 1", "reason": "test"}},
		{7, 6, "tool.success", tadwAdapterAt(3), map[string]interface{}{"output_file": output}},
	} {
		var parent interface{}
		if row.parent != 0 {
			parent = ref(row.parent).Id
		}
		raw, err := json.Marshal(row.payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Exec(`INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_principal,payload,created_at) VALUES('w','s',?,?,?,'user',?,?)`, ref(row.id).Id, parent, row.kind, string(raw), row.at); err != nil {
			t.Fatal(err)
		}
	}
	return store, ref(5)
}
func TestLoadTADWHistorySQLiteMetadataAndPromptOrder(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	cfg := tadwAdapterConfig(10000)
	result, err := loadTADWHistory(context.Background(), store, anchor, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertTADWInitialHistory(t, result)
	var want []string
	for _, id := range []int{1, 2, 3, 4, 6, 7, 5} {
		want = append(want, fmt.Sprintf("sev_%026d", id))
	}
	if !slices.Equal(result.Window.EventIDs, want) {
		t.Fatal(result.Window.EventIDs)
	}
	transcript := tadwTranscript{History: result.History}
	if !strings.Contains(transcript.String(), `name="résumé&lt;&amp;.txt" media-type="" size-bytes="123456789"`) || !strings.Contains(transcript.String(), "<output-file") || !strings.Contains(transcript.String(), "<attachment") {
		t.Fatal(transcript.String())
	}
	if result.Window.HistoryBytes != int64(len(transcript.String())) {
		t.Fatal("not exact XML bytes")
	}
	// The transaction must have released the store's sole connection.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var one int
	if err := store.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTADWHistoryStopsBeforeUnconsumedPayload(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	all, err := loadTADWHistory(context.Background(), store, anchor, nil, tadwAdapterConfig(10000))
	if err != nil {
		t.Fatal(err)
	}
	first := all.History[len(all.History)-1].Bytes()
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, fmt.Sprintf("sev_%026d", 7)); err != nil {
		t.Fatal(err)
	}
	for _, quota := range []int64{first - 1, first} {
		result, err := loadTADWHistory(context.Background(), store, anchor, nil, tadwAdapterConfig(quota))
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if quota == first {
			want = 1
		}
		if len(result.History) != want {
			t.Fatal(result)
		}
	}
	if result, err := loadTADWHistory(context.Background(), store, anchor, nil, tadwAdapterConfig(first+1)); err == nil || !reflect.DeepEqual(result, tadwInitialHistory{}) {
		t.Fatal("unconsumed corruption not detected when reached", err)
	}
}

func TestLoadTADWHistoryMissingFileAndValidation(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	if _, err := store.Exec(`DELETE FROM gatehouse_session_files`); err != nil {
		t.Fatal(err)
	}
	result, err := loadTADWHistory(context.Background(), store, anchor, nil, tadwAdapterConfig(10000))
	if err == nil || !reflect.DeepEqual(result, tadwInitialHistory{}) {
		t.Fatal("missing file accepted", err)
	}
	if _, err := loadTADWHistory(context.Background(), nil, anchor, nil, tadwWindowConfig{}); err == nil {
		t.Fatal("invalid config accepted")
	}
}
