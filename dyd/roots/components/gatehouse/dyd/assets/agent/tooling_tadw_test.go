package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type tadwToolingSliceSource struct{ *tadwSliceSource }

func (source tadwToolingSliceSource) Event() model.SessionEvent {
	return source.tadwSliceSource.Event().Event
}

func tadwToolingEvents() []model.SessionEvent {
	request := tadwTestEvent("agent.request", map[string]any{"agent": "agent"})
	request.Ref.Id = "request"
	tool := tadwTestEvent("tool.request", map[string]any{"name": "lisp", "code": "café<&", "reason": "inspect"})
	tool.Ref.Id, tool.Parent = "tool", &request.Ref
	result := tadwTestEvent("tool.success", map[string]any{"output": ""})
	result.Ref.Id, result.Parent = "result", &tool.Ref
	return []model.SessionEvent{request, tool, result}
}

func TestCollectTADWToolingBoundaries(t *testing.T) {
	events := tadwToolingEvents()
	fragments := tadwAdapterRender(t, events)
	first, total := fragments[0].Bytes(), tadwRenderedBytes(fragments)
	for _, test := range []struct {
		name            string
		budget          int64
		crossing, calls int
	}{
		{"zero", 0, 0, 1}, {"oversized-first", first - 1, 0, 1}, {"exact-first-not-exhausted", first, 1, 2},
		{"last-minus-one", total - 1, 2, 3}, {"exact-all", total, -1, 4}, {"spare", total + 1, -1, 4}, {"max-int", math.MaxInt64, -1, 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := tadwToolingSliceSource{&tadwSliceSource{events: events}}
			fixed := []tadwRenderedEvent{{ID: "history", XML: "<message/>\n"}}
			got, err := collectTADWTooling(context.Background(), source, tadwIdentityHydrate, fixed, test.budget)
			if source.calls != test.calls {
				t.Fatal("unexpected payload lookahead", source.calls, test.calls)
			}
			if test.crossing >= 0 {
				var overflow *tadwToolingBudgetError
				if !errors.As(err, &overflow) || !reflect.DeepEqual(got, tadwTranscript{}) || overflow.BudgetBytes != test.budget || overflow.EventID != fragments[test.crossing].ID || overflow.EventBytes != fragments[test.crossing].Bytes() || overflow.UsedBytes != tadwRenderedBytes(fragments[:test.crossing]) {
					t.Fatal(got, err)
				}
			} else {
				if err != nil || !reflect.DeepEqual(got.Tooling, fragments) || !reflect.DeepEqual(got.History, fixed) || got.Bytes() != int64(len(got.String())) {
					t.Fatal(got, err)
				}
				got.History[0].XML = "changed"
				if fixed[0].XML != "<message/>\n" {
					t.Fatal("history slice aliases frozen input")
				}
			}
		})
	}
}

func TestCollectTADWToolingErrorsAndSkippedKinds(t *testing.T) {
	for _, test := range []string{"source", "hydrate", "render", "duplicate", "overlap", "cancel", "negative-budget", "skipped", "empty"} {
		t.Run(test, func(t *testing.T) {
			events := tadwToolingEvents()
			source := tadwToolingSliceSource{&tadwSliceSource{events: events}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var fixed []tadwRenderedEvent
			hydrate := tadwIdentityHydrate
			budget := int64(10000)
			switch test {
			case "source":
				source.failure = errors.New("source failed")
			case "hydrate":
				hydrate = func(model.SessionEvent) (model.SessionEvent, error) {
					return model.SessionEvent{}, errors.New("hydrate failed")
				}
			case "render":
				events[2].Payload["output"] = 42
			case "duplicate":
				events[2].Ref.Id = events[1].Ref.Id
			case "overlap":
				fixed = []tadwRenderedEvent{{ID: events[0].Ref.Id, XML: "<event/>\n"}}
			case "cancel":
				hydrate = func(event model.SessionEvent) (model.SessionEvent, error) { cancel(); return event, nil }
			case "negative-budget":
				budget = -1
			case "skipped":
				source.events = []model.SessionEvent{tadwTestEvent("thinking.request", map[string]any{"attachments": 42}), tadwTestEvent("message.text", map[string]any{"text": 42}), tadwTestEvent("agent.success", map[string]any{"text": 42})}
				hydrate = func(model.SessionEvent) (model.SessionEvent, error) {
					t.Fatal("hydrated omitted event")
					return model.SessionEvent{}, nil
				}
			case "empty":
				source.events = nil
			}
			got, err := collectTADWTooling(ctx, source, hydrate, fixed, budget)
			if test == "skipped" || test == "empty" {
				if err != nil || len(got.Tooling) != 0 {
					t.Fatal(got, err)
				}
			} else if err == nil || !reflect.DeepEqual(got, tadwTranscript{}) {
				t.Fatal("expected no partial context", got, err)
			}
		})
	}
}

func tadwToolingDB(t *testing.T) (*database.Store, tadwInitialHistory, tadwWindowConfig, model.SessionEventRef, model.WorkspaceAgentRef) {
	t.Helper()
	store, anchor := tadwAdapterDB(t)
	cfg := tadwAdapterConfig(10000)
	initial, err := loadTADWHistory(context.Background(), store, anchor, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref := func(id int) model.SessionEventRef {
		return model.SessionEventRef{Session: anchor.Session, Id: fmt.Sprintf("sev_%026d", id)}
	}
	file := model.SessionFileRef{Session: anchor.Session, Id: "sfi_00000000000000000000000001"}
	for _, row := range []struct {
		id, parent int
		kind, at   string
		payload    any
		author     string
	}{
		{10, 5, "agent.request", "10", map[string]any{"agent": "agent"}, "user"},
		{11, 10, "thinking.request", "11", map[string]any{}, "agent"},
		{12, 11, "thinking.update", "12", map[string]any{}, "agent"},
		{20, 10, "tool.request", "20", map[string]any{"name": "lisp", "code": "café<&", "reason": "inspect"}, "agent"},
		{21, 10, "tool.request", "20", map[string]any{"name": "lisp", "code": "second", "reason": "inspect"}, "agent"},
		{22, 20, "agent.request", "22", map[string]any{"agent": "agent"}, "user"},
		{23, 22, "tool.request", "23", map[string]any{"code": 42}, "agent"},
		{24, 10, "thinking.success", "24", map[string]any{}, "other"},
		{25, 24, "tool.request", "25", map[string]any{"code": 42}, "agent"},
		{26, 20, "approval.request", "26", map[string]any{}, "agent"},
		{27, 26, "input.request", "27", map[string]any{}, "agent"},
		{30, 20, "tool.success", "30", map[string]any{"output_file": file}, "agent"},
		{31, 21, "tool.failure", "31", map[string]any{"code": "cancelled", "output": "not completed<&"}, "agent"},
		{40, 10, "thinking.request", "40", map[string]any{}, "agent"},
		{50, 10, "tool.request", "50", map[string]any{"name": "lisp", "code": "third", "reason": "inspect"}, "agent"},
		{51, 50, "tool.success", "51", map[string]any{"output": "TADW_PRIVATE_TOOLING"}, "agent"},
		{60, 10, "thinking.request", "60", map[string]any{}, "agent"},
	} {
		raw, err := json.Marshal(row.payload)
		if err != nil {
			t.Fatal(err)
		}
		var principal, agent any
		if row.author == "user" {
			principal = "user"
		} else {
			agent = row.author
		}
		if _, err := store.Exec(`INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_principal,author_agent,payload,created_at) VALUES('w','s',?,?,?,?,?,?,?)`, ref(row.id).Id, ref(row.parent).Id, row.kind, principal, agent, string(raw), row.at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id IN (?,?)`, ref(12).Id, ref(26).Id); err != nil {
		t.Fatal(err)
	}
	return store, initial, cfg, ref(10), model.WorkspaceAgentRef{Workspace: anchor.Session.Workspace, Id: "agent"}
}

func tadwToolingThinking(request model.SessionEventRef, id int) model.SessionEventRef {
	return model.SessionEventRef{Session: request.Session, Id: fmt.Sprintf("sev_%026d", id)}
}

func TestLoadTADWContextFixedHistoryGrowingTooling(t *testing.T) {
	store, initial, cfg, request, agent := tadwToolingDB(t)
	before, _ := json.Marshal(initial)
	frozen := tadwTranscript{History: initial.History}
	if _, err := store.Exec(`UPDATE gatehouse_session_files SET name='tooling & renamed.txt'`); err != nil {
		t.Fatal(err)
	}
	var previous []tadwRenderedEvent
	for _, test := range []struct {
		thinking int
		ids      []int
	}{{11, []int{10}}, {40, []int{10, 20, 21, 30, 31}}, {60, []int{10, 20, 21, 30, 31, 50, 51}}} {
		got, err := loadTADWContext(context.Background(), store, initial, request, agent, tadwToolingThinking(request, test.thinking), cfg)
		if err != nil {
			t.Fatal(err)
		}
		var ids, want []string
		for _, event := range got.Tooling {
			ids = append(ids, event.ID)
		}
		for _, id := range test.ids {
			want = append(want, tadwToolingThinking(request, id).Id)
		}
		if !slices.Equal(ids, want) || !reflect.DeepEqual(got.History, initial.History) || got.Bytes() != int64(len(got.String())) || !strings.HasPrefix(got.String(), frozen.String()) || got.Bytes() > cfg.ContextMaxBytes {
			t.Fatal("context order/prefix/accounting", ids, want)
		}
		if len(got.Tooling) < len(previous) || !slices.Equal(got.Tooling[:len(previous)], previous) {
			t.Fatal("earlier tooling prefix changed")
		}
		if test.thinking == 40 && (!strings.Contains(got.Tooling[3].XML, `name="tooling &amp; renamed.txt"`) || !strings.Contains(got.Tooling[3].XML, `size-bytes="123456789"`)) {
			t.Fatal("file metadata not hydrated", got.Tooling[3])
		}
		previous = got.Tooling
	}
	// Repeating the earlier attempt cannot pick up later tooling, or rerender
	// frozen historical file references with their newly changed metadata.
	earlier, err := loadTADWContext(context.Background(), store, initial, request, agent, tadwToolingThinking(request, 40), cfg)
	if err != nil || len(earlier.Tooling) != 5 {
		t.Fatal(earlier, err)
	}
	after, _ := json.Marshal(initial)
	if string(before) != string(after) {
		t.Fatal("mutated frozen history")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := store.PingContext(ctx); err != nil {
		t.Fatal("retained sole connection", err)
	}
}

func TestLoadTADWContextExactBudgetAndFailures(t *testing.T) {
	for _, name := range []string{"exact", "overflow", "zero", "missing-file", "bad-payload", "overlap", "cancelled", "closed-store"} {
		t.Run(name, func(t *testing.T) {
			store, initial, cfg, request, agent := tadwToolingDB(t)
			thinking := tadwToolingThinking(request, 40)
			full, err := loadTADWContext(context.Background(), store, initial, request, agent, thinking, cfg)
			if err != nil {
				t.Fatal(err)
			}
			budget := full.ToolingBytes()
			if name == "overflow" {
				budget--
			}
			if name == "zero" {
				budget = 0
			}
			cfg = tadwWindowConfig{ContextMaxBytes: initial.Window.HistoryBytes + budget, HistoryMaxBytes: initial.Window.HistoryBytes, HistoryMinBytes: 1}
			initial.Window.ToolingBudgetBytes = budget
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var query string
			switch name {
			case "missing-file":
				query = `DELETE FROM gatehouse_session_files`
			case "bad-payload":
				query = `UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id='` + tadwToolingThinking(request, 30).Id + `'`
			case "overlap":
				initial.Window.EventIDs[0] = request.Id
				initial.Window.BaseEventID = request.Id
				initial.History = slices.Clone(initial.History)
				initial.History[0].ID = request.Id
			case "cancelled":
				cancel()
			case "closed-store":
				store.Close()
			}
			if query != "" {
				if _, err := store.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			got, err := loadTADWContext(ctx, store, initial, request, agent, thinking, cfg)
			if name == "exact" {
				if err != nil || got.Bytes() != cfg.ContextMaxBytes {
					t.Fatal(got, err)
				}
			} else {
				if err == nil || !reflect.DeepEqual(got, tadwTranscript{}) {
					t.Fatal("expected no partial context", got, err)
				}
				if name == "overflow" || name == "zero" {
					var overflow *tadwToolingBudgetError
					if !errors.As(err, &overflow) {
						t.Fatal("untyped budget error", err)
					}
				}
			}
			if name != "closed-store" {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := store.PingContext(ctx); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestLoadTADWContextStopsAtFirstCrossing(t *testing.T) {
	store, initial, cfg, request, agent := tadwToolingDB(t)
	thinking := tadwToolingThinking(request, 40)
	all, err := loadTADWContext(context.Background(), store, initial, request, agent, thinking, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id IN (?,?)`, initial.Window.EventIDs[0], tadwToolingThinking(request, 30).Id); err != nil {
		t.Fatal(err)
	}
	// Frozen history must not be fetched again. Metadata sorting must not decode
	// the later payload either; only reaching it through Next reveals corruption.
	first := all.Tooling[0].Bytes()
	for _, budget := range []int64{0, first - 1, first, first + all.Tooling[1].Bytes() - 1} {
		cfg := tadwWindowConfig{ContextMaxBytes: initial.Window.HistoryBytes + budget, HistoryMaxBytes: initial.Window.HistoryBytes, HistoryMinBytes: 1}
		initial.Window.ToolingBudgetBytes = budget
		got, err := loadTADWContext(context.Background(), store, initial, request, agent, thinking, cfg)
		var overflow *tadwToolingBudgetError
		if !errors.As(err, &overflow) || !reflect.DeepEqual(got, tadwTranscript{}) {
			t.Fatal("read beyond crossing event or returned partial context", got, err)
		}
	}
	initial.Window.ToolingBudgetBytes = cfg.ContextMaxBytes - initial.Window.HistoryBytes
	if _, err := loadTADWContext(context.Background(), store, initial, request, agent, thinking, cfg); err == nil || !strings.Contains(err.Error(), "decode session event payload") {
		t.Fatal("unconsumed corruption not reported when reached", err)
	}
}

func TestLoadTADWContextEmptyHistory(t *testing.T) {
	store, _, cfg, request, agent := tadwToolingDB(t)
	empty := tadwInitialHistory{Window: tadwHistoryWindow{ToolingBudgetBytes: cfg.ContextMaxBytes}}
	got, err := loadTADWContext(context.Background(), store, empty, request, agent, tadwToolingThinking(request, 40), cfg)
	if err != nil || len(got.History) != 0 || len(got.Tooling) != 5 || got.Bytes() != got.ToolingBytes() {
		t.Fatal(got, err)
	}
	// The actual request parent remains recorded even outside the prompt.
	if !strings.Contains(got.Tooling[0].XML, `parent="`+tadwToolingThinking(request, 5).Id+`"`) {
		t.Fatal("missing actual parent", got.Tooling[0])
	}
}

func TestTADWToolingWindowPinsRequestAcrossOrderingAndReplay(t *testing.T) {
	store, history, cfg, request, agent := tadwToolingDB(t)
	// Test independently of the normal parent-before-child timestamp invariant.
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET created_at='zz' WHERE id=?`, request.Id); err != nil {
		t.Fatal(err)
	}
	thinking := tadwToolingThinking(request, 40)
	full, err := loadTADWContext(context.Background(), store, history, request, agent, thinking, cfg)
	if err != nil || full.Tooling[0].ID != request.Id {
		t.Fatal("request is not pinned first", full, err)
	}
	history, cfg = tadwTideBudget(history, full.ToolingBytes()-1)
	selected, err := loadTADWToolingWindow(context.Background(), store, history, request, agent, thinking, cfg, "")
	if err != nil || selected.Transcript.Tooling[0].ID != request.Id || selected.FromEventID == "" {
		t.Fatal(selected, err)
	}
	recovered, err := loadTADWContextFrom(context.Background(), store, history, request, agent, thinking, cfg, selected.FromEventID)
	if err != nil || !reflect.DeepEqual(recovered, selected.Transcript) {
		t.Fatal("fresh/replay request position differs", recovered, err)
	}
}

func TestTADWFixedHistoryValidation(t *testing.T) {
	_, initial, cfg, request, agent := tadwToolingDB(t)
	for _, name := range []string{"config", "id-count", "id-order", "duplicate", "empty-id", "empty-fragment", "history-max", "base", "weight", "tooling-budget"} {
		t.Run(name, func(t *testing.T) {
			history := initial
			history.History = slices.Clone(initial.History)
			history.Window.EventIDs = slices.Clone(initial.Window.EventIDs)
			config := cfg
			switch name {
			case "config":
				config = tadwWindowConfig{}
			case "id-count":
				history.Window.EventIDs = nil
			case "id-order":
				slices.Reverse(history.Window.EventIDs)
			case "duplicate":
				history.History[1] = history.History[0]
				history.Window.EventIDs[1] = history.Window.EventIDs[0]
			case "empty-id":
				history.History[0].ID = ""
			case "empty-fragment":
				history.History[0].XML = ""
			case "history-max":
				config.HistoryMaxBytes = 1
				config.HistoryMinBytes = 1
			case "base":
				history.Window.BaseEventID = "other"
			case "weight":
				history.Window.HistoryBytes++
			case "tooling-budget":
				history.Window.ToolingBudgetBytes = math.MaxInt64
			}
			if _, err := loadTADWContext(context.Background(), nil, history, request, agent, tadwToolingThinking(request, 40), config); err == nil {
				t.Fatal("invalid frozen history accepted")
			}
		})
	}
	cfg = tadwWindowConfig{ContextMaxBytes: math.MaxInt64, HistoryMaxBytes: math.MaxInt64, HistoryMinBytes: 1}
	empty := tadwInitialHistory{Window: tadwHistoryWindow{ToolingBudgetBytes: math.MaxInt64}}
	if budget, err := tadwFixedHistoryBudget(empty, cfg); err != nil || budget != math.MaxInt64 {
		t.Fatal(budget, err)
	}
}

func TestTADWToolingAssemblyDoesNotCheckpointContext(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "tadw-tooling-test-worker")
	store, initial, cfg, request, agent := tadwToolingDB(t)
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='{"text":"TADW_PRIVATE_PAYLOAD"}' WHERE id=?`, fmt.Sprintf("sev_%026d", 1)); err != nil {
		t.Fatal(err)
	}
	workflowContext, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "tadw-tooling-test", ApplicationVersion: "tadw-tooling-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(workflowContext, 5*time.Second)
	workflow := func(ctx dbos.Context, request model.SessionEventRef) (bool, error) {
		anchor := tadwToolingThinking(request, 5)
		history, err := checkpointTADWHistory(ctx, store, anchor, nil, cfg)
		if err != nil {
			return false, err
		}
		var priorBytes int64
		for _, id := range []int{11, 40, 60} {
			transcript, err := loadTADWContext(ctx, store, history, request, agent, tadwToolingThinking(request, id), cfg)
			if err != nil {
				return false, err
			}
			if transcript.ToolingBytes() <= priorBytes || transcript.HistoryBytes() != history.Window.HistoryBytes {
				return false, fmt.Errorf("history/tooling changed unexpectedly")
			}
			priorBytes = transcript.ToolingBytes()
			if _, err := dbos.RunAsStep(ctx, func(context.Context) (bool, error) { return true, nil }, dbos.WithStepName("test.tadw-thinking")); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	dbos.RegisterWorkflow(workflowContext, workflow, dbos.WithWorkflowName("test.tadw-tooling"))
	if err := testdbos.Launch(workflowContext); err != nil {
		t.Fatal(err)
	}
	handle, err := dbos.RunWorkflow(workflowContext, workflow, request, dbos.WithWorkflowID("tadw-request"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(10*time.Second)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	assertTADWCheckpointStorage(t, store, initial.Window.EventIDs, 3)
	assertTADWDBOSPayloadAbsent(t, store)
}
