package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func liveTADWRatePolicy() *config.WorkspaceAgentRateLimits {
	return &config.WorkspaceAgentRateLimits{WorkspaceOutput: &config.TokenBucket{MinimumBalance: -2000, MaximumBalance: 1, RefillPerMinute: 102400}}
}

func TestTADWNormalReplyRateLimitWait(t *testing.T) {
	var body []byte
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoded, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = encoded
		mu.Unlock()
		liveProviderReply(w, "openai-responses", 0)
	}))
	defer server.Close()
	f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "wait.sqlite"), server.URL, "openai-responses", config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 4096, HistoryMinBytes: 1024}}, liveTADWRatePolicy())
	f.submit(t, true)
	deadline := time.Now().Add(5 * time.Second)
	delayed := false
	for time.Now().Before(deadline) {
		var count int
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_events WHERE kind='thinking.update' AND json_extract(payload,'$.reason')='rate_limit'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			delayed = true
			break
		}
		time.Sleep(testdbos.PollInterval)
	}
	if !delayed {
		t.Fatal("provider attempt did not wait for its rate-limit claim")
	}
	var earlyHints int
	if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_context_cache_hints`).Scan(&earlyHints); err != nil || earlyHints != 0 {
		t.Fatal("estimation and rate-limit waiting must not publish hints", earlyHints, err)
	}
	if _, err := f.store.Exec(`UPDATE gatehouse_session_events SET payload=json_object('text','HISTORY_UPDATED_DURING_RATE_WAIT') WHERE id=?`, f.older.Id); err != nil {
		t.Fatal(err)
	}
	if outcome := f.outcome(t); outcome.Kind != model.SessionEventKindAgentSuccess {
		t.Fatal(outcome.Payload)
	}
	mu.Lock()
	sent := append([]byte(nil), body...)
	mu.Unlock()
	if !strings.Contains(liveUserContent(t, sent), "HISTORY_UPDATED_DURING_RATE_WAIT") {
		t.Fatal("provider preparation did not read history after waiting")
	}
	var claims, settlements, sleeps int
	if err := f.store.QueryRow(`SELECT SUM(function_name='gatehouse.agent-rate-limit-claim'),SUM(function_name='gatehouse.agent-rate-limit-settle'),SUM(function_name='DBOS.sleep') FROM operation_outputs`).Scan(&claims, &settlements, &sleeps); err != nil {
		t.Fatal(err)
	}
	if claims != 1 || settlements != 1 || sleeps != 1 {
		t.Fatal("unexpected rate-limit checkpoint sequence", claims, settlements, sleeps)
	}
	var encoded string
	if err := f.store.QueryRow(`SELECT output FROM operation_outputs WHERE function_name='gatehouse.agent-rate-limit-claim'`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var claim struct{ Wait time.Duration }
	if err := json.Unmarshal(liveStepOutput(t, encoded), &claim); err != nil || claim.Wait <= 0 {
		t.Fatal("missing durable wait", claim, err)
	}
	assertLiveTADWState(t, f.store, 1)
}

func TestTADWNormalReplyRequiresSuccessfulEstimation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		liveProviderReply(w, "openai-responses", 0)
	}))
	defer server.Close()
	f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "estimate.sqlite"), server.URL, "openai-responses", config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 4096, HistoryMinBytes: 1024}})
	// Provider availability changes between the declared preparation boundaries.
	if _, err := f.store.Exec(`
		CREATE TRIGGER unavailable_at_estimate AFTER INSERT ON operation_outputs
		WHEN NEW.function_name='gatehouse.tadw-tooling-window'
		BEGIN UPDATE gatehouse_agent_providers SET enabled=FALSE; END;
		CREATE TRIGGER available_after_claim AFTER INSERT ON operation_outputs
		WHEN NEW.function_name='gatehouse.agent-rate-limit-claim'
		BEGIN UPDATE gatehouse_agent_providers SET enabled=TRUE; END;
	`); err != nil {
		t.Fatal(err)
	}
	f.submit(t, true)
	if outcome := f.outcome(t); outcome.Kind != model.SessionEventKindAgentFailure {
		t.Fatal("failed estimation must fail the attempt", outcome)
	}
	if calls.Load() != 0 {
		t.Fatal("provider execution requires successful estimation", calls.Load())
	}
	var hints int
	if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_context_cache_hints`).Scan(&hints); err != nil || hints != 0 {
		t.Fatal("failed preparation must not publish a hint", hints, err)
	}
}

func TestTADWNormalReplyAdmissionAndCancellation(t *testing.T) {
	for _, mode := range []string{"deny admission", "revoke during final", "revoke during tools", "cancel provider"} {
		t.Run(mode, func(t *testing.T) {
			started, resume := make(chan struct{}, 1), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(resume) }) }
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-resume:
				case <-r.Context().Done():
					return
				}
				tool := 0
				if mode == "revoke during tools" {
					tool = 1
				}
				liveProviderReply(w, "openai-responses", tool)
			}))
			defer server.Close()
			defer release()
			f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "lifecycle.sqlite"), server.URL, "openai-responses", config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 4096, HistoryMinBytes: 1024}})
			revoke := func() {
				if _, err := f.store.Exec(`UPDATE gatehouse_workspace_grants SET enabled=FALSE; UPDATE gatehouse_session_grants SET enabled=FALSE`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "deny admission" {
				f.submit(t, false)
				revoke()
				if err := testdbos.Launch(f.ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				f.submit(t, true)
				select {
				case <-started:
				case <-time.After(10 * time.Second):
					t.Fatal("provider was not reached")
				}
				if mode == "cancel provider" {
					err, request := f.store.SessionEventChildGet(context.Background(), f.message.Ref, model.SessionEventKindAgentRequest)
					if err != nil || request == nil {
						t.Fatal(request, err)
					}
					id, err := typed_id.New(typed_id.SessionEvent)
					if err != nil {
						t.Fatal(err)
					}
					cancel := model.SessionEvent{Ref: model.SessionEventRef{Session: f.message.Ref.Session, Id: id}, Parent: &request.Ref, Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: f.message.AuthorPrincipal, Payload: map[string]any{}}
					if err, _ := f.runtime.CancelReply(context.Background(), cancel); err != nil {
						t.Fatal(err)
					}
				} else {
					revoke()
					release()
				}
			}
			outcome := f.outcome(t)
			want := model.SessionEventKindAgentFailure
			if mode == "revoke during final" {
				want = model.SessionEventKindAgentSuccess
			}
			if outcome.Kind != want {
				t.Fatal("unexpected lifecycle outcome", outcome.Kind, outcome.Payload)
			}
			wantCalls := int32(1)
			if mode == "deny admission" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatal("provider calls", calls.Load(), wantCalls)
			}
			if mode == "cancel provider" && outcome.Payload["code"] != "cancelled" {
				t.Fatal("missing cancellation classification", outcome.Payload)
			}
			if mode == "cancel provider" || mode == "deny admission" {
				var hints int
				if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_context_cache_hints`).Scan(&hints); err != nil || hints != 0 {
					t.Fatal("an unused prefix must not be published", mode, hints, err)
				}
			}
			if mode == "revoke during tools" {
				var completed int
				if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_events WHERE kind='tool.success'`).Scan(&completed); err != nil || completed != 1 {
					t.Fatal("admitted tool did not finish", completed, err)
				}
			}
			if mode == "deny admission" {
				var histories int
				if err := f.store.QueryRow(`SELECT COUNT(*) FROM operation_outputs WHERE function_name='gatehouse.tadw-initial-history'`).Scan(&histories); err != nil || histories != 0 {
					t.Fatal("history selected before admission", histories, err)
				}
			}
		})
	}
}
