package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func captureLiveTADWHintWrites(t *testing.T, store *database.Store) {
	t.Helper()
	if _, err := store.Exec(`
  CREATE TABLE test_hint_writes (hint_key TEXT NOT NULL, decision_count INTEGER NOT NULL);
  CREATE TRIGGER test_hint_insert AFTER INSERT ON gatehouse_context_cache_hints
  BEGIN INSERT INTO test_hint_writes SELECT NEW.hint_key,COUNT(*) FROM gatehouse_session_events WHERE kind IN ('thinking.success','thinking.failure'); END;
  CREATE TRIGGER test_hint_update AFTER UPDATE ON gatehouse_context_cache_hints
  BEGIN INSERT INTO test_hint_writes SELECT NEW.hint_key,COUNT(*) FROM gatehouse_session_events WHERE kind IN ('thinking.success','thinking.failure'); END;
 `); err != nil {
		t.Fatal(err)
	}
}

func liveTADWHintWrites(t *testing.T, store *database.Store) int {
	t.Helper()
	var count, premature int
	if err := store.QueryRow(`SELECT COUNT(*),COALESCE(SUM(decision_count < rowid),0) FROM test_hint_writes`).Scan(&count, &premature); err != nil {
		t.Fatal(err)
	}
	if premature != 0 {
		t.Fatal("hint publication must follow its committed provider decision", premature)
	}
	return count
}

func TestTADWNormalReplyHintPublication(t *testing.T) {
	for _, mode := range []string{"final", "tools", "retry", "transport failure", "invalid response", "publication failure", "application failure", "snapshot drift", "empty history", "mcmtr"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			var store *database.Store
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				before := 0
				if mode == "tools" && call == 2 {
					before = 1
				}
				if got := liveTADWHintWrites(t, store); got != before {
					t.Errorf("hint writes before provider response=%d want=%d", got, before)
				}
				if mode == "snapshot drift" {
					if _, err := store.Exec(`UPDATE gatehouse_workspace_agents SET system_prompt='CHANGED_PROMPT_AFTER_SEND'; UPDATE gatehouse_agent_providers SET endpoint='https://changed.invalid',enabled=FALSE`); err != nil {
						t.Error(err)
					}
				}
				if mode == "retry" && call == 1 {
					w.Header().Set("retry-after-ms", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded"}}`))
					return
				}
				if mode == "transport failure" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if mode == "invalid response" {
					_, _ = w.Write([]byte(`{"output":[]}`))
					return
				}
				tool := 0
				if mode == "tools" && call == 1 {
					tool = 1
				}
				liveProviderReply(w, "openai-responses", tool)
			}))
			defer server.Close()
			compaction := config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 4096, HistoryMinBytes: 1024}}
			if mode == "empty history" {
				compaction.TADW.HistoryMaxBytes = 1
				compaction.TADW.HistoryMinBytes = 1
			}
			if mode == "mcmtr" {
				compaction = defaultAgentModelCompaction
			}
			f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "hints.sqlite"), server.URL, "openai-responses", compaction)
			store = f.store
			captureLiveTADWHintWrites(t, store)
			agentID := f.message.Payload["agents"].([]string)[0]
			err, selected := store.WorkspaceAgentModelGet(context.Background(), f.message.Ref.Session.Workspace, agentID)
			if err != nil || selected == nil {
				t.Fatal(selected, err)
			}
			expected, err := agent.TADWHistoryCacheScopeForTest(f.message.Ref.Session, selected, false)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "application failure" {
				if _, err := store.Exec(`CREATE TRIGGER fail_decision BEFORE INSERT ON gatehouse_session_events WHEN NEW.kind='agent.success' BEGIN SELECT RAISE(ABORT,'decision unavailable'); END`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "publication failure" {
				if _, err := store.Exec(`CREATE TRIGGER fail_hint_write BEFORE INSERT ON gatehouse_context_cache_hints BEGIN SELECT RAISE(ABORT,'hint unavailable'); END`); err != nil {
					t.Fatal(err)
				}
			}
			f.submit(t, true)
			outcome := f.outcome(t)
			want := model.SessionEventKindAgentSuccess
			if mode == "transport failure" || mode == "invalid response" || mode == "application failure" {
				want = model.SessionEventKindAgentFailure
			}
			if outcome.Kind != want {
				t.Fatal("hint publication must not alter the provider outcome", outcome.Kind, outcome.Payload)
			}
			wantWrites := 1
			if mode == "tools" {
				wantWrites = 2
			}
			if mode == "transport failure" || mode == "invalid response" || mode == "publication failure" || mode == "application failure" || mode == "empty history" || mode == "mcmtr" {
				wantWrites = 0
			}
			if got := liveTADWHintWrites(t, store); got != wantWrites {
				t.Fatal("successful fresh uses with committed decisions must publish once each", got, wantWrites)
			}
			var rows int
			if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_context_cache_hints`).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if wantWrites == 0 {
				if rows != 0 {
					t.Fatal("unexpected shared hint", rows)
				}
				return
			}
			hints, err := store.ContextCacheHintsList(context.Background(), expected)
			if err != nil || len(hints) != 1 || rows != 1 {
				t.Fatal("publication must use the sent request's scope", hints, rows, err)
			}
			var output string
			if err := store.QueryRow(`SELECT output FROM operation_outputs WHERE function_name='gatehouse.tadw-initial-history' LIMIT 1`).Scan(&output); err != nil {
				t.Fatal(err)
			}
			var ids []string
			if err := json.Unmarshal(liveStepOutput(t, output), &ids); err != nil {
				t.Fatal(err)
			}
			var hint struct {
				Version int    `json:"version"`
				Base    string `json:"base_event_id"`
			}
			if err := json.Unmarshal(hints[0].Hint, &hint); err != nil {
				t.Fatal(err)
			}
			if len(ids) == 0 || hints[0].Key != ids[0] || hint.Base != ids[0] || hint.Version != 1 {
				t.Fatal("hint must identify the selected history base", hint, ids)
			}
			for _, secret := range []string{server.URL, "tadw-test-key", liveTADWMarker, "TADW_PRIVATE_TOOL_"} {
				if strings.Contains(expected.CacheScope+expected.Profile+string(hints[0].Hint), secret) {
					t.Fatal("hint contains provider or transcript data")
				}
			}
			rounds := 1
			if mode == "tools" {
				rounds = 2
			}
			assertLiveTADWState(t, store, rounds)
		})
	}
}

func TestTADWNormalReplyConsumesPublishedHint(t *testing.T) {
	bodies := make(chan []byte, 3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		select {
		case bodies <- body:
		default:
			t.Error("unexpected extra provider call")
		}
		tool := 0
		if calls.Add(1) == 1 {
			tool = 1
		}
		liveProviderReply(w, "openai-responses", tool)
	}))
	defer server.Close()
	f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "reuse.sqlite"), server.URL, "openai-responses", config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 4096, HistoryMinBytes: 1}})
	captureLiveTADWHintWrites(t, f.store)
	f.submit(t, true)
	firstOutcome := f.outcome(t)
	if firstOutcome.Kind != model.SessionEventKindAgentSuccess {
		t.Fatal(firstOutcome.Payload)
	}
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	next := f.message
	next.Ref.Id = id
	next.Parent = &firstOutcome.Ref
	var agentID string
	if err := f.store.QueryRow(`SELECT id FROM gatehouse_workspace_agents LIMIT 1`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	next.Payload = map[string]any{"text": "next request", "agents": []string{agentID}}
	if err, _ := f.runtime.CreateMessage(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	second := *f
	second.message = next
	if outcome := second.outcome(t); outcome.Kind != model.SessionEventKindAgentSuccess {
		t.Fatal(outcome.Payload)
	}
	if calls.Load() != 3 || len(bodies) != 3 {
		t.Fatal("expected two provider calls on the first turn and one on its reply", calls.Load())
	}
	first, afterTool, followUp := liveUserMessages(t, <-bodies), liveUserMessages(t, <-bodies), liveUserMessages(t, <-bodies)
	firstEvents, toolEvents, replyEvents := first[:len(first)-1], afterTool[:len(afterTool)-1], followUp[:len(followUp)-1]
	if len(toolEvents) != len(firstEvents)+2 || !slices.Equal(firstEvents, toolEvents[:len(firstEvents)]) ||
		!strings.HasPrefix(toolEvents[len(firstEvents)], "<tool.request ") || !strings.HasPrefix(toolEvents[len(firstEvents)+1], "<tool.success ") {
		t.Fatal("tool loop did not append separate request/result items after unchanged event messages", afterTool)
	}
	if len(replyEvents) != len(toolEvents)+3 || !slices.Equal(toolEvents, replyEvents[:len(toolEvents)]) ||
		!strings.HasPrefix(replyEvents[len(toolEvents)], "<agent.success ") || !strings.HasPrefix(replyEvents[len(toolEvents)+1], "<message ") ||
		!strings.HasPrefix(replyEvents[len(toolEvents)+2], "<agent.request ") {
		t.Fatal("thread reply did not preserve event messages from the previous tool loop", followUp)
	}
	if first[len(first)-1] != afterTool[len(afterTool)-1] || first[len(first)-1] == followUp[len(followUp)-1] {
		t.Fatal("hints changed within the same request or were not updated for the follow-up")
	}
	rows, err := f.store.Query(`SELECT output FROM operation_outputs WHERE function_name='gatehouse.tadw-initial-history' ORDER BY workflow_uuid`)
	if err != nil {
		t.Fatal(err)
	}
	var histories [][]string
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			t.Fatal(err)
		}
		var ids []string
		if err := json.Unmarshal(liveStepOutput(t, encoded), &ids); err != nil {
			t.Fatal(err)
		}
		histories = append(histories, ids)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(histories) != 2 || len(histories[0]) != 1 || len(histories[1]) <= 1 || histories[1][0] != histories[0][0] {
		t.Fatal("next request must align to the published eligible base", histories)
	}
	if writes := liveTADWHintWrites(t, f.store); writes != 3 {
		t.Fatal("each actual provider use must publish/touch", writes)
	}
	var count int
	if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_context_cache_hints`).Scan(&count); err != nil || count != 1 {
		t.Fatal("aligned requests must refresh the same hint", count, err)
	}
}
