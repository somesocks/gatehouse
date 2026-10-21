package agent_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type codexProxyTransport struct {
	target     *url.URL
	base       http.RoundTripper
	unexpected *atomic.Int32
}

func (transport codexProxyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != config.OpenAICodexEndpoint {
		transport.unexpected.Add(1)
		return nil, errors.New("unexpected outbound request")
	}
	request := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = transport.target.Scheme, transport.target.Host
	request.URL, request.Host = &u, transport.target.Host
	return transport.base.RoundTrip(request)
}

type codexAgentFixture struct {
	store     *database.Store
	ring      *keychain.Keyring
	runtime   *agent.SessionEventReplyRuntime
	ctx       dbos.Context
	provider  *database.SystemAgentProvider
	session   model.SessionRef
	principal model.Principal
	message   model.SessionEvent
	request   model.SessionEvent
}

func newCodexAgentFixture(t *testing.T, named bool) *codexAgentFixture {
	t.Helper()
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "codex-agent-test")
	t.Setenv("GATEHOUSE_TEST_CODEX_KEYCHAIN", "test passphrase")
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: "pbkdf2-hmac-sha256-v0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	chains := []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_CODEX_KEYCHAIN"}}}
	err, ring := keychain.NewKeyring(store, chains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	endpoint := config.OpenAICodexEndpoint
	state := config.State{
		Keychains: chains, Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}}, Principals: []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "chatgpt", Revision: 1, Protocol: config.AgentProviderProtocolOpenAICodex, Endpoint: &endpoint, Enabled: true}},
		AgentModels:    []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "chatgpt", Model: "gpt-codex", Parameters: `{"reasoning_effort":"low"}`, Compaction: defaultAgentModelCompaction, MaxTurns: 4, MaxOutputTokens: 100, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true,
			RateLimits: &config.WorkspaceAgentRateLimits{WorkspaceOutput: &config.TokenBucket{MinimumBalance: 0, MaximumBalance: 1000, RefillPerMinute: 1}}}},
	}
	err, registry := migrations.Build(configuration, admittedAgentTestState(state), ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	err, providers := store.SystemAgentProvidersGet(ctx)
	if err != nil || len(providers) != 1 {
		t.Fatal("provider unavailable")
	}
	f := &codexAgentFixture{store: store, ring: ring, provider: &providers[0]}
	text := f.encrypt(t, "INFERENCE_ACCESS_SENTINEL")
	err, f.provider = store.SystemAgentProviderCodexCredentialSet(ctx, f.provider.ID, f.provider.Revision, f.provider.Keychain, &text, &database.SystemAgentProviderAccount{ID: "account"})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	principal := principalRef(t, ctx, store, "alice")
	f.principal = model.Principal{Ref: principal, Enabled: true}
	f.session = model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: f.session, AuthorPrincipal: &principal, Enabled: true}, principal); err != nil {
		t.Fatal(err)
	}
	if named {
		if err, _ := store.SessionNameSet(ctx, f.session, "Already named"); err != nil {
			t.Fatal(err)
		}
	}
	var agentID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	f.message = model.SessionEvent{Ref: model.SessionEventRef{Session: f.session, Id: "sev_00000000000000000000000000"}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &f.principal, Payload: map[string]any{"text": "Calculate 1 + 2", "agents": []string{agentID}}}
	f.ctx, err = dbos.NewContext(ctx, dbos.Config{AppName: "codex-agent-test", ApplicationVersion: "codex-agent-v1", ExecutorID: "codex-agent-test", SQLiteSystemDB: store.DB, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	err, f.runtime = agent.NewSessionEventReplyRuntime(f.ctx, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := f.runtime.CreateMessage(ctx, f.message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, f.message.Ref, model.SessionEventKindAgentRequest)
	if err != nil || request == nil {
		t.Fatal("agent request missing")
	}
	f.request = *request
	t.Cleanup(func() {
		if err := dbos.Shutdown(f.ctx, 2*time.Second); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f *codexAgentFixture) encrypt(t *testing.T, access string) string {
	t.Helper()
	err, keys := f.ring.Get(context.Background(), []model.KeychainRef{*f.provider.Keychain})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(keys[*f.provider.Keychain])
	plain, _ := json.Marshal(codexlogin.Credential{Version: 1, AccessToken: access, RefreshToken: "INFERENCE_REFRESH_SENTINEL", AccountID: "account"})
	defer clear(plain)
	err, sealed := keychain.Seal(rand.Reader, keys[*f.provider.Keychain], []byte("gh=v1|agent-provider|id="+f.provider.ID), plain)
	if err != nil {
		t.Fatal(err)
	}
	sealed.AAD = keychain.AADID
	return sealed.String()
}

func (f *codexAgentFixture) await(t *testing.T, kind string) []model.SessionEvent {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		err, events := f.store.SessionEventsGet(context.Background(), f.session)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind == kind {
				return events
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missing event %s", kind)
	return nil
}

func codexWireEvent(w io.Writer, kind string, fields map[string]any) {
	fields["type"] = kind
	encoded, _ := json.Marshal(fields)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, encoded)
}

func codexWireReply(w http.ResponseWriter, item map[string]any, outputTokens int) {
	w.Header().Set("Content-Type", "text/event-stream")
	codexWireEvent(w, "response.output_item.done", map[string]any{"output_index": 0, "item": item})
	codexWireEvent(w, "response.completed", map[string]any{"response": map[string]any{"id": "resp_test", "status": "completed", "usage": map[string]any{"input_tokens": 50, "output_tokens": outputTokens, "total_tokens": 50 + outputTokens}}})
}

func codexWireMessage(text string) map[string]any {
	return map[string]any{"type": "message", "phase": "final_answer", "content": []any{map[string]any{"type": "output_text", "text": text}}}
}

func TestCodexAgentInference(t *testing.T) {
	for _, mode := range []string{"tool-and-title", "unauthorized", "json-rejection", "truncated-tool", "output-overage", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var turns, titles, unexpected atomic.Int32
			started, cancelled := make(chan struct{}), make(chan struct{})
			var f *codexAgentFixture
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Model        string
					Instructions string
					Input        []map[string]any
					Tools        []any
					Stream       bool
					Store        bool
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || !request.Stream || request.Store || request.Model != "gpt-codex" || request.Instructions == "" {
					t.Error("incorrect inference request")
					return
				}
				if r.URL.Path != "/backend-api/codex/responses" || r.Header.Get("ChatGPT-Account-ID") != "account" {
					t.Error("incorrect account or route")
				}
				if len(request.Tools) == 0 {
					titles.Add(1)
					codexWireReply(w, codexWireMessage("Simple calculation"), 5)
					return
				}
				turn := turns.Add(1)
				access := "INFERENCE_ACCESS_SENTINEL"
				if turn == 2 {
					access = "NEW_INFERENCE_ACCESS_SENTINEL"
				}
				if r.Header.Get("Authorization") != "Bearer "+access {
					t.Error("inference did not use the current access token")
				}
				if mode == "unauthorized" {
					w.WriteHeader(http.StatusUnauthorized)
					io.WriteString(w, "INFERENCE_ACCESS_SENTINEL INFERENCE_REFRESH_SENTINEL")
					return
				}
				if mode == "json-rejection" {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Request-ID", "req_local_dev_test")
					io.WriteString(w, `{"error":{"code":"model_not_found","param":"model","message":"INFERENCE_ACCESS_SENTINEL INFERENCE_REFRESH_SENTINEL"}}`)
					return
				}
				if mode == "output-overage" {
					codexWireReply(w, codexWireMessage(`{"text":"Oversized","attachments":[]}`), 1200)
					return
				}
				if mode == "cancel" {
					w.Header().Set("Content-Type", "text/event-stream")
					codexWireEvent(w, "response.created", map[string]any{"response": map[string]any{"id": "resp_wait"}})
					w.(http.Flusher).Flush()
					close(started)
					<-r.Context().Done()
					close(cancelled)
					return
				}
				if turn == 1 {
					arguments, _ := json.Marshal(map[string]string{"code": "(int/add 1 2)", "reason": "Calculate"})
					tool := map[string]any{"type": "function_call", "call_id": "call_calculate", "name": "lisp", "arguments": string(arguments)}
					if mode == "truncated-tool" {
						w.Header().Set("Content-Type", "text/event-stream")
						codexWireEvent(w, "response.output_item.done", map[string]any{"output_index": 0, "item": tool})
						return
					}
					// Simulate an hourly refresh while inference is in flight. No
					// transaction may span the network; the next turn must reload it.
					err, original := f.store.AgentProviderCredentialGet(r.Context(), f.provider.ID)
					if err != nil {
						t.Error(err)
						return
					}
					next := f.encrypt(t, "NEW_INFERENCE_ACCESS_SENTINEL")
					if err, changed := f.store.AgentProviderMaintenanceCredentialSet(r.Context(), f.provider.ID, *original.Encrypted, *f.provider.Keychain, *f.provider.Keychain, &next, &database.SystemAgentProviderAccount{ID: "account"}); err != nil || !changed {
						t.Error("could not simulate maintenance")
						return
					}
					codexWireReply(w, tool, 20)
					return
				}
				foundCall, foundResult := false, false
				for _, item := range request.Input {
					if item["type"] == "function_call" && item["call_id"] == "call_calculate" {
						foundCall = true
					}
					if item["type"] == "function_call_output" && item["call_id"] == "call_calculate" && strings.Contains(fmt.Sprint(item["output"]), "3") {
						foundResult = true
					}
				}
				if !foundCall || !foundResult {
					t.Error("tool conversation was not preserved")
				}
				codexWireReply(w, codexWireMessage(`{"text":"The answer is 3.","attachments":[]}`), 30)
			}))
			t.Cleanup(server.Close)
			target, _ := url.Parse(server.URL)
			originalTransport := http.DefaultTransport
			http.DefaultTransport = codexProxyTransport{target: target, base: originalTransport, unexpected: &unexpected}
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			f = newCodexAgentFixture(t, mode != "tool-and-title")
			err, before := f.store.AgentProviderCredentialGet(context.Background(), f.provider.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := dbos.Launch(f.ctx); err != nil {
				t.Fatal(err)
			}
			terminal := model.SessionEventKindAgentFailure
			if mode == "tool-and-title" {
				terminal = model.SessionEventKindAgentSuccess
			}
			if mode == "cancel" {
				select {
				case <-started:
				case <-time.After(10 * time.Second):
					t.Fatal("inference did not start")
				}
				id, err := typed_id.New(typed_id.SessionEvent)
				if err != nil {
					t.Fatal(err)
				}
				if err, _ := f.runtime.CancelReply(context.Background(), model.SessionEvent{Ref: model.SessionEventRef{Session: f.session, Id: id}, Parent: &f.request.Ref, Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: &f.principal, Payload: map[string]any{}}); err != nil {
					t.Fatal(err)
				}
				terminal = model.SessionEventKindCancelSuccess
			}
			events := f.await(t, terminal)
			tools := 0
			usefulDiagnostic := false
			for _, event := range events {
				if event.Kind == model.SessionEventKindToolRequest {
					tools++
				}
				if event.Kind == model.SessionEventKindAgentSuccess && event.Payload["text"] != "The answer is 3." {
					t.Fatal("wrong final reply")
				}
				if mode == "json-rejection" && event.Kind == model.SessionEventKindThinkingFailure {
					message, _ := event.Payload["message"].(string)
					usefulDiagnostic = strings.Contains(message, "HTTP 200") && strings.Contains(message, "body=json") && strings.Contains(message, "code=model_not_found") && strings.Contains(message, "param=model")
				}
				encoded, _ := json.Marshal(event)
				if strings.Contains(string(encoded), "INFERENCE_ACCESS_SENTINEL") || strings.Contains(string(encoded), "INFERENCE_REFRESH_SENTINEL") {
					t.Fatal("credential leaked into events")
				}
			}
			if mode == "json-rejection" && !usefulDiagnostic {
				t.Fatal("session failure omitted upstream diagnostics")
			}
			if mode == "tool-and-title" {
				if turns.Load() != 2 || tools != 1 {
					t.Fatal("tool conversation did not complete exactly once")
				}
				deadline := time.Now().Add(10 * time.Second)
				for {
					err, name := f.store.SessionNameGet(context.Background(), f.session)
					if err != nil {
						t.Fatal(err)
					}
					if name != nil {
						if *name != "Simple calculation" {
							t.Fatal("incorrect title")
						}
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("title not generated")
					}
					time.Sleep(20 * time.Millisecond)
				}
				if titles.Load() != 1 {
					t.Fatal("title call repeated")
				}
			} else {
				if turns.Load() != 1 || tools != 0 {
					t.Fatal("failed inference retried or executed a partial tool")
				}
				err, after := f.store.AgentProviderCredentialGet(context.Background(), f.provider.ID)
				if err != nil || *before.Encrypted != *after.Encrypted || before.Provider.Revision != after.Provider.Revision {
					t.Fatal("inference modified credentials")
				}
			}
			if mode == "cancel" {
				select {
				case <-cancelled:
				case <-time.After(3 * time.Second):
					t.Fatal("HTTP stream was not cancelled")
				}
			}
			if mode == "tool-and-title" || mode == "output-overage" {
				var balance float64
				if err := f.store.QueryRow(`SELECT available_output_tokens FROM gatehouse_workspace_agent_rate_buckets WHERE principal_id IS NULL`).Scan(&balance); err != nil {
					t.Fatal(err)
				}
				want := 950.0
				if mode == "output-overage" {
					want = -200
				}
				if balance < want || balance > want+1 {
					t.Fatalf("output balance %v, want approximately %v", balance, want)
				}
			}
			if mode == "output-overage" {
				// An upstream overage must debit the quota, and a new request
				// must fail its reservation before making another network call.
				next := f.message
				id, err := typed_id.New(typed_id.SessionEvent)
				if err != nil {
					t.Fatal(err)
				}
				next.Ref.Id = id
				if err, _ := f.runtime.CreateMessage(context.Background(), next); err != nil {
					t.Fatal(err)
				}
				err, root := f.store.SessionEventChildGet(context.Background(), next.Ref, model.SessionEventKindAgentRequest)
				if err != nil || root == nil {
					t.Fatal("second request missing")
				}
				deadline := time.Now().Add(10 * time.Second)
				for {
					err, failure := f.store.SessionEventChildGet(context.Background(), root.Ref, model.SessionEventKindAgentFailure)
					if err != nil {
						t.Fatal(err)
					}
					if failure != nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("next request did not observe quota debit")
					}
					time.Sleep(20 * time.Millisecond)
				}
				if turns.Load() != 1 {
					t.Fatal("request exceeded the depleted output quota")
				}
			}
			if unexpected.Load() != 0 {
				t.Fatal("inference attempted an unexpected endpoint (including token refresh)")
			}
			for _, marker := range []string{"INFERENCE_ACCESS_SENTINEL", "INFERENCE_REFRESH_SENTINEL", *before.Encrypted} {
				assertDBOSDoesNotContain(t, f.store, marker)
			}
		})
	}
}
