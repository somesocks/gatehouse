package agent_test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"modernc.org/sqlite"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

// Run the first half in a separate process so the test exercises a real crash
// with durable PENDING workflows, rather than graceful shutdown cancellation.
func TestReplyRecoveryProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_TEST_REPLAY_PATH")
	if path == "" {
		t.Skip("subprocess helper")
	}
	ctx := context.Background()
	crashPoint := os.Getenv("GATEHOUSE_TEST_CRASH_POINT")
	if crashPoint != "" {
		if err := sqlite.RegisterScalarFunction("gh_test_crash", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(0)
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_REPLAY_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	workflowContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-input-replay-" + os.Getenv("GATEHOUSE_TEST_REPLAY_PROTOCOL"), ApplicationVersion: "replay-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	if crashPoint != "" {
		trigger := `CREATE TRIGGER crash_at_provider_boundary BEFORE INSERT ON workflow_status
			WHEN NEW.name = 'gatehouse.session-tool-call' BEGIN SELECT gh_test_crash(); END`
		if crashPoint == "final-replay" {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE UPDATE ON workflow_status
				WHEN NEW.name = 'gatehouse.session-event-reply' AND NEW.status = 'SUCCESS'
				BEGIN SELECT gh_test_crash(); END`
		} else if crashPoint == "round-final-replay" || crashPoint == "request-replay" {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE UPDATE ON workflow_status
				WHEN NEW.name = 'gatehouse.session-agent-round' AND NEW.status = 'SUCCESS'
				BEGIN SELECT gh_test_crash(); END`
		} else if crashPoint == "retry-replay" || strings.HasSuffix(crashPoint, "switch") {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE INSERT ON operation_outputs
				WHEN NEW.function_name = 'DBOS.sleep' BEGIN SELECT gh_test_crash(); END`
		} else if crashPoint == "decision-rollback" {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE INSERT ON operation_outputs
				WHEN NEW.function_name = 'gatehouse.agent-provider-decision' BEGIN SELECT gh_test_crash(); END`
		} else if crashPoint == "input-request-rollback" {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE INSERT ON operation_outputs
				WHEN NEW.function_name = 'gatehouse.session-event-agent-persist'
					AND EXISTS (SELECT 1 FROM gatehouse_session_events WHERE kind = 'input.request')
				BEGIN SELECT gh_test_crash(); END`
		}
		if _, err := store.ExecContext(ctx, trigger); err != nil {
			t.Fatal(err)
		}
	}
	err, runtime := agent.NewSessionEventReplyRuntime(workflowContext, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbos.Launch(workflowContext); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestReplyResumesCustomInputAcrossRestart(t *testing.T) {
	for _, testCase := range []struct{ protocol, outcome string }{
		{"openai-responses", "success"},
		{"openai-responses", "cancelled"},
		{"openai-responses", "batch-rollback"},
		{"openai-responses", "retry"},
		{"openai-responses", "batch-unstarted"},
		{"openai-responses", "final-replay"},
		{"openai-responses", "round-final-replay"},
		{"openai-responses", "request-replay"},
		{"openai-responses", "retry-replay"},
		{"openai-responses", "rate-replay"},
		{"openai-responses", "decision-rollback"},
		{"openai-responses", "input-request-rollback"},
		{"openai-responses", "multiple-inputs"},
		{"openai-responses", "provider-failure"},
		{"openai-responses", "invalid-batch"},
		{"openai-responses", "final-rollback"},
		{"openai-chat-completions", "success"},
		{"openai-chat-completions", "cancelled"},
		{"openai-responses", "provider-switch"},
		{"openai-chat-completions", "provider-switch"},
		{"openai-responses", "agent-switch"},
		{"openai-responses", "limit-switch"},
		{"openai-responses", "endpoint-switch"},
		{"openai-responses", "policy-switch"},
		{"openai-responses", "add-policy-switch"},
		{"openai-responses", "prompt-switch"},
		{"openai-chat-completions", "prompt-switch"},
	} {
		protocol, outcome := testCase.protocol, testCase.outcome
		switching := strings.HasSuffix(outcome, "switch")
		successfulInput := outcome == "success" || outcome == "batch-unstarted" || outcome == "rate-replay" || outcome == "input-request-rollback" || outcome == "multiple-inputs"
		t.Run(protocol+"/"+outcome, func(t *testing.T) {
			t.Setenv("DBOS__APPVERSION", "")
			t.Setenv("DBOS__VMID", "gatehouse-input-replay-test")
			t.Setenv("GATEHOUSE_TEST_REPLAY_KEYCHAIN", "test passphrase")
			t.Setenv("GATEHOUSE_TEST_REPLAY_API_KEY", "test-key")
			const providerReplyMarker = "DBOS_PROVIDER_REPLY_MUST_STAY_IN_GATEHOUSE_a91d6e"
			var calls atomic.Int32
			var requestMutex sync.Mutex
			var requestBodies [][]byte
			var providerStore *database.Store
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					http.Error(w, "read request", http.StatusInternalServerError)
					return
				}
				requestMutex.Lock()
				requestBodies = append(requestBodies, body)
				requestMutex.Unlock()
				if outcome == "prompt-switch" {
					var request struct {
						Instructions string                           `json:"instructions"`
						Messages     []struct{ Role, Content string } `json:"messages"`
					}
					if err := json.Unmarshal(body, &request); err != nil {
						t.Error(err)
					}
					prompt := request.Instructions
					if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
						prompt = request.Messages[0].Content
					}
					want, absent := "OLD_CONFIGURED_PROMPT", "NEW_CONFIGURED_PROMPT"
					if calls.Load() > 0 {
						want, absent = absent, want
					}
					if !strings.Contains(prompt, want) || strings.Contains(prompt, absent) {
						t.Errorf("attempt did not use current system prompt: %q", prompt)
					}
				}
				callProtocol := protocol
				if switching && r.URL.Path != "/custom/inference" {
					if r.RequestURI != "/replacement/a%2Fb/?deployment=next" || r.Header.Get("Authorization") != "Bearer next-key" {
						t.Errorf("replacement transport = %s auth=%q", r.RequestURI, r.Header.Get("Authorization"))
					}
					if outcome != "endpoint-switch" {
						if protocol == "openai-responses" {
							callProtocol = "openai-chat-completions"
						} else {
							callProtocol = "openai-responses"
						}
						var request map[string]any
						if err := json.Unmarshal(body, &request); err != nil {
							t.Error(err)
						}
						if request["model"] != "replacement-model" || callProtocol == "openai-chat-completions" && request["messages"] == nil || callProtocol == "openai-responses" && request["input"] == nil {
							t.Errorf("replacement wire request = %s", body)
						}
						if outcome == "limit-switch" && request["max_tokens"] != float64(512) {
							t.Errorf("replacement call ignored lowered output limit: %s", body)
						}
					}
				} else if r.RequestURI != "/custom/inference?deployment=test" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("original transport = %s auth=%q", r.RequestURI, r.Header.Get("Authorization"))
				}
				// A second connection must be able to write while the provider
				// call is outstanding. This catches transactions around HTTP.
				probeContext, stopProbe := context.WithTimeout(r.Context(), 2*time.Second)
				_, probeErr := providerStore.ExecContext(probeContext, `UPDATE gatehouse_sessions SET name = name`)
				stopProbe()
				if probeErr != nil {
					t.Errorf("database write blocked during provider call: %v", probeErr)
					http.Error(w, "database locked", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				code := `(input/ask (input/form "Select" (session-file-picker "file" "File" (json/object))))`
				if outcome == "multiple-inputs" {
					code = `(let ((selected ` + code + `)) (list selected (input/ask (input/form "Select again" (session-file-picker "file" "File" selected)))))`
				}
				arguments, _ := json.Marshal(map[string]string{"code": code, "reason": "Select a session file"})
				callIDs := []string{"call-picker"}
				if outcome == "batch-rollback" || outcome == "batch-unstarted" || outcome == "decision-rollback" || outcome == "invalid-batch" {
					callIDs = []string{"call-first", "call-second"}
				}
				callNumber := calls.Add(1)
				if outcome == "provider-failure" {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(providerReplyMarker))
					return
				}
				if (outcome == "retry" || outcome == "retry-replay" || switching) && callNumber == 1 {
					w.Header().Set("Retry-After-Ms", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded"}}`))
					return
				}
				var response any
				if callProtocol == "openai-responses" {
					if callNumber == 1 && outcome != "final-replay" && outcome != "final-rollback" && outcome != "round-final-replay" && outcome != "request-replay" {
						outputs := make([]any, 0, len(callIDs))
						for _, callID := range callIDs {
							callArguments := arguments
							if callID == "call-first" {
								callArguments, _ = json.Marshal(map[string]string{"code": "42", "reason": "Return a number"})
							}
							name := "lisp"
							if outcome == "invalid-batch" && callID == "call-second" {
								name = "unsupported-tool"
							}
							outputs = append(outputs, map[string]any{"type": "function_call", "call_id": callID, "name": name, "arguments": string(callArguments)})
						}
						response = map[string]any{"output": outputs}
					} else {
						text := "Selected " + providerReplyMarker
						if (outcome == "final-replay" || outcome == "round-final-replay" || outcome == "request-replay") && callNumber > 1 {
							text = "Discard this new provider response"
						}
						final, _ := json.Marshal(map[string]any{"text": text, "attachments": []string{}})
						response = map[string]any{"output": []any{map[string]any{"type": "message", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": string(final)}}}}}
					}
				} else {
					message := map[string]any{"role": "assistant"}
					if callNumber == 1 && outcome != "final-replay" {
						toolCalls := make([]any, 0, len(callIDs))
						for _, callID := range callIDs {
							toolCalls = append(toolCalls, map[string]any{"id": callID, "type": "function", "function": map[string]any{"name": "lisp", "arguments": string(arguments)}})
						}
						message["tool_calls"] = toolCalls
					} else {
						message["content"] = `{"text":"Selected DBOS_PROVIDER_REPLY_MUST_STAY_IN_GATEHOUSE_a91d6e","attachments":[]}`
					}
					response = map[string]any{"choices": []any{map[string]any{"message": message}}}
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			ctx := context.Background()
			configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "gatehouse.sqlite")}
			err, store := database.Open(ctx, configuration)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { store.Close() }()
			chains := []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_REPLAY_KEYCHAIN"}}}
			endpoint := server.URL + "/custom/inference?deployment=test"
			err, ring := keychain.NewKeyring(store, chains, keychain.NewPassphraseSourceResolver())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { ring.Close() }()
			state := config.State{
				Keychains: chains, Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}}, Principals: []config.Principal{{Alias: "alice", Enabled: true}},
				AgentProviders:  []config.AgentProvider{{Alias: "test", Revision: 1, Protocol: protocol, Endpoint: &endpoint, Sources: []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_REPLAY_API_KEY"}, Enabled: true}},
				AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "test", Model: "test-model", Parameters: `{}`, Compaction: defaultAgentModelCompaction, MaxTurns: 4, MaxOutputTokens: 1024, Enabled: true}},
				WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
			}
			if outcome == "prompt-switch" {
				prompt := "OLD_CONFIGURED_PROMPT"
				state.WorkspaceAgents[0].SystemPrompt = &prompt
			}
			if outcome == "rate-replay" || outcome == "policy-switch" || outcome == "limit-switch" {
				state.WorkspaceAgents[0].RateLimits = &config.WorkspaceAgentRateLimits{
					WorkspaceInput:  &config.TokenBucket{MinimumBalance: -1_000_000, MaximumBalance: 1_000_000, RefillPerMinute: 1_000_000},
					WorkspaceOutput: &config.TokenBucket{MinimumBalance: -1_000_000, MaximumBalance: 1_000_000, RefillPerMinute: 1_000_000},
				}
			}
			err, registry := migrations.Build(configuration, admittedAgentTestState(state), ring)
			if err != nil {
				t.Fatal(err)
			}
			if err := migrations.Run(ctx, store, registry); err != nil {
				t.Fatal(err)
			}
			providerStore = store
			if outcome == "batch-rollback" {
				_, err := store.ExecContext(ctx, `
					CREATE TRIGGER reject_second_provider_tool
					BEFORE INSERT ON gatehouse_session_events
					WHEN NEW.kind = 'tool.request' AND json_extract(NEW.payload, '$.call_id') = 'call-second'
					BEGIN SELECT RAISE(ABORT, 'reject second tool request'); END
				`)
				if err != nil {
					t.Fatal(err)
				}
			}
			if outcome == "final-rollback" {
				if _, err := store.ExecContext(ctx, `CREATE TRIGGER reject_provider_final BEFORE INSERT ON gatehouse_session_events
					WHEN NEW.kind = 'agent.success' BEGIN SELECT RAISE(ABORT, 'reject final reply'); END`); err != nil {
					t.Fatal(err)
				}
			}
			workspace := workspaceRef(t, ctx, store, "engineering")
			principal := principalRef(t, ctx, store, "alice")
			var agentID string
			if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
				t.Fatal(err)
			}
			session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
			if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principal, Enabled: true}, principal); err != nil {
				t.Fatal(err)
			}
			if err, _ := store.SessionNameSet(ctx, session, "Recovery test"); err != nil {
				t.Fatal(err)
			}
			message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: map[string]interface{}{"text": "Select a file", "agents": []string{agentID}}}
			if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
				t.Fatal(err)
			}
			newRuntime := func() (dbos.Context, *agent.SessionEventReplyRuntime) {
				t.Helper()
				context, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-input-replay-" + protocol, ApplicationVersion: "replay-v1", SQLiteSystemDB: store.DB})
				if err != nil {
					t.Fatal(err)
				}
				err, runtime := agent.NewSessionEventReplyRuntime(context, store, ring)
				if err != nil {
					t.Fatal(err)
				}
				if err := dbos.Launch(context); err != nil {
					t.Fatal(err)
				}
				return context, runtime
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			process := exec.Command(executable, "-test.run=^TestReplyRecoveryProcess$")
			process.Env = append(os.Environ(), "GATEHOUSE_TEST_REPLAY_PATH="+configuration.Path, "GATEHOUSE_TEST_REPLAY_PROTOCOL="+protocol)
			boundaryCrash := outcome == "batch-unstarted" || outcome == "final-replay" || outcome == "retry-replay" || outcome == "decision-rollback" || outcome == "round-final-replay" || outcome == "request-replay" || outcome == "input-request-rollback" || switching
			var recoveredContext dbos.Context
			var recoveredRuntime *agent.SessionEventReplyRuntime
			if boundaryCrash {
				process.Env = append(process.Env, "GATEHOUSE_TEST_CRASH_POINT="+outcome)
			}
			if err := process.Start(); err != nil {
				t.Fatal(err)
			}
			defer process.Process.Kill()
			if boundaryCrash {
				exited := make(chan error, 1)
				go func() { exited <- process.Wait() }()
				select {
				case err := <-exited:
					if err != nil {
						t.Fatalf("provider boundary crash subprocess: %v", err)
					}
				case <-time.After(15 * time.Second):
					t.Fatal("subprocess did not reach provider boundary crash")
				}
				if _, err := store.ExecContext(ctx, `DROP TRIGGER crash_at_provider_boundary`); err != nil {
					t.Fatal(err)
				}
				if outcome == "round-final-replay" {
					err, root := store.SessionEventChildGet(ctx, message.Ref, model.SessionEventKindAgentRequest)
					if err != nil || root == nil {
						t.Fatalf("round root = %#v (%v)", root, err)
					}
					id, err := typed_id.New(typed_id.SessionEvent)
					if err != nil {
						t.Fatal(err)
					}
					ref := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
					if err, _ := store.SessionEventsCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id},
						Parent: &root.Ref, Kind: model.SessionEventKindMessageText, AuthorAgent: &ref,
						Payload: map[string]any{"text": "LATER_ACTIVE_EVENT_MUST_NOT_ENTER_REQUEST"}}); err != nil {
						t.Fatal(err)
					}
					id, err = typed_id.New(typed_id.SessionEvent)
					if err != nil {
						t.Fatal(err)
					}
					if err, _ := store.SessionEventsCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id},
						Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true},
						Payload: map[string]any{"text": "LATER_USER_MESSAGE_MUST_NOT_ENTER_REQUEST"}}); err != nil {
						t.Fatal(err)
					}
				}
				if switching {
					if outcome == "policy-switch" {
						state.WorkspaceAgents[0].RateLimits = nil
					} else {
						state.WorkspaceAgents[0].RateLimits = &config.WorkspaceAgentRateLimits{
							WorkspaceOutput: &config.TokenBucket{MinimumBalance: -1_000_000, MaximumBalance: 2_000_000, RefillPerMinute: 2_000_000},
						}
					}
					state.WorkspaceAgents[0].Revision++
					if outcome == "prompt-switch" {
						prompt := "NEW_CONFIGURED_PROMPT"
						state.WorkspaceAgents[0].SystemPrompt = &prompt
					}
					if outcome != "policy-switch" && outcome != "add-policy-switch" && outcome != "prompt-switch" {
						t.Setenv("GATEHOUSE_TEST_REPLAY_NEXT_KEY", "next-key")
						nextEndpoint := server.URL + "/replacement/a%2Fb/?deployment=next"
						if outcome == "endpoint-switch" {
							state.AgentProviders[0].Revision++
							state.AgentProviders[0].Endpoint = &nextEndpoint
							state.AgentProviders[0].Sources = []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_REPLAY_NEXT_KEY"}
						} else {
							nextProtocol := "openai-chat-completions"
							if protocol == nextProtocol {
								nextProtocol = "openai-responses"
							}
							state.AgentProviders[0].Revision++
							state.AgentProviders[0].Enabled = false
							state.AgentProviders = append(state.AgentProviders, config.AgentProvider{Alias: "next", Revision: 1, Protocol: nextProtocol,
								Endpoint: &nextEndpoint, Sources: []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_REPLAY_NEXT_KEY"}, Enabled: true})
							if outcome == "agent-switch" {
								replacement := state.AgentModels[0]
								replacement.Alias, replacement.Revision = "replacement", 1
								replacement.ProviderAlias, replacement.Model, replacement.MaxOutputTokens = "next", "replacement-model", 2048
								state.AgentModels[0].Revision++
								state.AgentModels[0].Enabled = false
								state.AgentModels = append(state.AgentModels, replacement)
								state.WorkspaceAgents[0].ModelAlias = "replacement"
							} else {
								state.AgentModels[0].Revision++
								state.AgentModels[0].ProviderAlias, state.AgentModels[0].Model = "next", "replacement-model"
								state.AgentModels[0].MaxOutputTokens = 2048
								if outcome == "limit-switch" {
									state.AgentModels[0].MaxOutputTokens = 512
								}
							}
						}
					}
					err, upgrade := migrations.Build(configuration, admittedAgentTestState(state), ring)
					if err != nil {
						t.Fatal(err)
					}
					if err := migrations.Run(ctx, store, upgrade); err != nil {
						t.Fatal(err)
					}
				}
				if outcome == "request-replay" {
					if _, err := store.ExecContext(ctx, `UPDATE gatehouse_workspace_agents SET enabled = FALSE, "default" = FALSE;
						UPDATE gatehouse_agent_contexts SET state = '{"user_high_from":17}'`); err != nil {
						t.Fatal(err)
					}
				}
				var thinkingSuccess, toolRequests int
				if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_session_events WHERE kind = 'thinking.success'`).Scan(&thinkingSuccess); err != nil {
					t.Fatal(err)
				}
				if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_session_events WHERE kind = 'tool.request'`).Scan(&toolRequests); err != nil {
					t.Fatal(err)
				}
				wantSuccess := 1
				if outcome == "retry-replay" || outcome == "decision-rollback" || switching {
					wantSuccess = 0
				}
				wantTools := 0
				if outcome == "batch-unstarted" {
					wantTools = 2
				} else if outcome == "input-request-rollback" {
					wantTools = 1
					var inputs int
					if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_session_events WHERE kind = 'input.request'`).Scan(&inputs); err != nil || inputs != 0 {
						t.Fatalf("uncommitted input request survived crash: %d (%v)", inputs, err)
					}
				}
				if thinkingSuccess != wantSuccess || toolRequests != wantTools {
					t.Fatalf("decision at boundary crash: thinking.success=%d tool.request=%d", thinkingSuccess, toolRequests)
				}
				second, runtime := newRuntime()
				recoveredContext, recoveredRuntime = second, runtime
				defer dbos.Shutdown(second, 10*time.Second)
				if err := runtime.Reconcile(); err != nil {
					t.Fatal(err)
				}
				if outcome != "batch-unstarted" && outcome != "input-request-rollback" {
					err, request := store.SessionEventChildGet(ctx, message.Ref, model.SessionEventKindAgentRequest)
					if err != nil || request == nil {
						t.Fatalf("agent request = (%#v, %v)", request, err)
					}
					handle, err := dbos.RetrieveWorkflow[model.SessionEventRef](second, "session-event-reply:"+request.Ref.Id)
					if err != nil {
						t.Fatal(err)
					}
					result, err := handle.GetResult()
					wantCalls := int32(2)
					if outcome == "final-replay" {
						wantCalls = 1
					}
					if outcome == "request-replay" {
						wantCalls = 1 // Disabled binding cannot make another HTTP call.
					}
					if outcome == "retry-replay" || switching {
						wantCalls = 3
					}
					if err != nil || result.Id == "" || calls.Load() != wantCalls {
						t.Fatalf("final reply replay = (%#v, %v), provider calls=%d", result, err, calls.Load())
					}
					if outcome == "round-final-replay" {
						requestMutex.Lock()
						identical := len(requestBodies) == 2 && bytes.Equal(requestBodies[0], requestBodies[1])
						requestMutex.Unlock()
						if !identical {
							t.Fatal("reconstructed round input changed after later session events")
						}
					}
					if err, final := store.SessionEventGet(ctx, result); err != nil || final == nil || final.Payload["text"] != "Selected "+providerReplyMarker {
						t.Fatalf("committed final reply = (%#v, %v)", final, err)
					}
					if switching {
						rows, err := store.QueryContext(ctx, `SELECT output FROM operation_outputs WHERE function_name = 'gatehouse.agent-rate-limit-claim' ORDER BY function_id`)
						if err != nil {
							t.Fatal(err)
						}
						var claims []database.AgentRateLimitClaim
						for rows.Next() {
							var encoded string
							var claim database.AgentRateLimitClaim
							if err := rows.Scan(&encoded); err != nil {
								t.Fatal(err)
							}
							decoded, err := base64.StdEncoding.DecodeString(encoded)
							if err != nil {
								t.Fatal(err)
							}
							if err := json.Unmarshal(decoded, &claim); err != nil {
								t.Fatal(err)
							}
							claims = append(claims, claim)
						}
						rows.Close()
						if len(claims) != 2 {
							t.Fatalf("got %d claims, want 2", len(claims))
						}
						last := claims[1]
						if outcome == "policy-switch" {
							if last.Workspace.Id != "" {
								t.Fatal("removed policy still reserved tokens")
							}
						} else if last.WorkspaceOutput == nil || last.WorkspaceOutput.MaximumBalance != 2_000_000 {
							t.Fatal("new claim did not use current rate-limit policy")
						}
						var settlements int
						if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_outputs WHERE function_name = 'gatehouse.agent-rate-limit-settle'`).Scan(&settlements); err != nil || settlements != 2 {
							t.Fatalf("settlements = %d (%v), want one per logical attempt", settlements, err)
						}
					}
					var count int
					if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_session_events WHERE kind = 'agent.success'`).Scan(&count); err != nil || count != 1 {
						t.Fatalf("final reply count = %d, err=%v", count, err)
					}
					assertDBOSDoesNotContain(t, store, providerReplyMarker)
					assertMinimalThinkingRequests(t, ctx, store, session)
					assertToolResultsResolveParent(t, ctx, store, session)
					return
				}
			}
			var input model.SessionEventRef
			var inputToolCall model.SessionEventRef
			var agentFailure *model.SessionEvent
			var agentSuccess *model.SessionEvent
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				err, events := store.SessionEventsGet(ctx, session)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if event.Kind == model.SessionEventKindAgentFailure {
						failed := event
						agentFailure = &failed
					}
					if event.Kind == model.SessionEventKindAgentSuccess {
						succeeded := event
						agentSuccess = &succeeded
					}
					if event.Kind == model.SessionEventKindInputRequest {
						if event.Parent == nil {
							t.Fatal("input request has no owning tool")
						}
						var exists bool
						if err := store.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM workflow_status WHERE workflow_uuid = ?)`, "session-tool-call:"+event.Parent.Id).Scan(&exists); err != nil || !exists {
							t.Fatalf("input was exposed before its destination workflow: %v (%v)", exists, err)
						}
						input = event.Ref
						if event.Parent != nil {
							inputToolCall = *event.Parent
						}
					}
				}
				if input.Id != "" || agentFailure != nil || outcome == "retry" && agentSuccess != nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if outcome == "batch-rollback" || outcome == "invalid-batch" || outcome == "final-rollback" || outcome == "provider-failure" {
				if agentFailure == nil {
					t.Fatalf("provider batch did not fail after injected insert error; input request=%q", input.Id)
				}
				if err := process.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = process.Wait()
				err, events := store.SessionEventsGet(ctx, session)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if event.Kind == model.SessionEventKindThinkingSuccess || event.Kind == model.SessionEventKindToolRequest || event.Kind == model.SessionEventKindInputRequest {
						t.Fatalf("failed provider decision left a partial batch event: %#v", event)
					}
				}
				if input.Id != "" || calls.Load() != 1 {
					t.Fatalf("failed provider batch left input=%q after %d provider calls", input.Id, calls.Load())
				}
				if outcome == "provider-failure" {
					var message string
					if err := store.QueryRowContext(ctx, `SELECT json_extract(payload, '$.message') FROM gatehouse_session_events WHERE kind = 'thinking.failure'`).Scan(&message); err != nil || !strings.Contains(message, providerReplyMarker) {
						t.Fatalf("persisted provider failure = (%q, %v)", message, err)
					}
				}
				assertDBOSDoesNotContain(t, store, providerReplyMarker)
				return
			}
			if outcome == "retry" {
				if agentSuccess == nil || agentSuccess.Payload["text"] != "Selected "+providerReplyMarker || calls.Load() != 2 {
					t.Fatalf("reply after provider retry = (%#v, calls=%d)", agentSuccess, calls.Load())
				}
				err, events := store.SessionEventsGet(ctx, session)
				if err != nil {
					t.Fatal(err)
				}
				updates, successes, failures := 0, 0, 0
				for _, event := range events {
					switch event.Kind {
					case model.SessionEventKindThinkingUpdate:
						if event.Payload["reason"] == "rate_limit" {
							updates++
						}
					case model.SessionEventKindThinkingSuccess:
						successes++
					case model.SessionEventKindThinkingFailure:
						failures++
					}
				}
				if updates != 1 || successes != 1 || failures != 0 {
					t.Fatalf("provider retry thinking events: updates=%d successes=%d failures=%d", updates, successes, failures)
				}
				assertDBOSDoesNotContain(t, store, providerReplyMarker)
				if err := process.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = process.Wait()
				return
			}
			if input.Id == "" {
				t.Fatalf("custom input did not start; agent failure=%#v", agentFailure)
			}
			if !boundaryCrash {
				if err := process.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = process.Wait()
				ring.Close()
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				err, store = database.Open(ctx, configuration)
				if err != nil {
					t.Fatal(err)
				}
				providerStore = store
				err, ring = keychain.NewKeyring(store, chains, keychain.NewPassphraseSourceResolver())
				if err != nil {
					t.Fatal(err)
				}
				recoveredContext, recoveredRuntime = newRuntime()
				defer dbos.Shutdown(recoveredContext, 10*time.Second)
			}
			runtime := recoveredRuntime
			var inputWorkflows int
			if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_status WHERE name = 'gatehouse.session-input'`).Scan(&inputWorkflows); err != nil || inputWorkflows != 0 {
				t.Fatalf("unexpected input child workflows: %d (%v)", inputWorkflows, err)
			}
			const answerMarker = "DBOS_INPUT_PAYLOAD_MUST_STAY_IN_GATEHOUSE_7f3c2a"
			const secondAnswerMarker = "DBOS_SECOND_INPUT_PAYLOAD_MUST_STAY_IN_GATEHOUSE_a93f2c"
			firstAnswer := answerMarker
			if outcome == "multiple-inputs" {
				firstAnswer += strings.Repeat("x", 128<<10)
			}
			responseKind := model.SessionEventKindInputSuccess
			responsePayload := map[string]interface{}{}
			if successfulInput {
				if err, _ := store.SessionInputDraftSet(ctx, input, []string{"file"}, json.RawMessage(`{"file_id":"`+firstAnswer+`"}`)); err != nil {
					t.Fatal(err)
				}
			} else {
				responseKind = model.SessionEventKindInputFailure
				responsePayload = map[string]interface{}{"code": "cancelled", "message": answerMarker}
			}
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				t.Fatal(err)
			}
			if err, _ := runtime.RespondToInput(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &input, Kind: responseKind, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: responsePayload}); err != nil {
				t.Fatal(err)
			}
			deadline = time.Now().Add(15 * time.Second)
			var inputResponse model.SessionEventRef
			var secondInput model.SessionEventRef
			var inputToolOutcome *model.SessionEvent
			for time.Now().Before(deadline) {
				err, events := store.SessionEventsGet(ctx, session)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if outcome == "multiple-inputs" && event.Kind == model.SessionEventKindInputRequest && event.Ref != input && secondInput.Id == "" {
						if event.Parent == nil || *event.Parent != inputToolCall {
							t.Fatal("second form belongs to a different tool call")
						}
						encoded, err := json.Marshal(event.Payload)
						if err != nil || !bytes.Contains(encoded, []byte(answerMarker)) {
							t.Fatal("second form did not reuse the first response as its inputs")
						}
						assertDBOSDoesNotContain(t, store, answerMarker)
						secondInput = event.Ref
						if err, _ := store.SessionInputDraftSet(ctx, secondInput, []string{"file"}, json.RawMessage(`{"file_id":"`+secondAnswerMarker+`"}`)); err != nil {
							t.Fatal(err)
						}
						id, err := typed_id.New(typed_id.SessionEvent)
						if err != nil {
							t.Fatal(err)
						}
						if err, _ := runtime.RespondToInput(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &secondInput, Kind: model.SessionEventKindInputSuccess, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}}); err != nil {
							t.Fatal(err)
						}
					}
					if (event.Kind == model.SessionEventKindInputSuccess || event.Kind == model.SessionEventKindInputFailure) && event.Parent != nil && *event.Parent == input {
						inputResponse = event.Ref
					}
					if (event.Kind == model.SessionEventKindToolSuccess || event.Kind == model.SessionEventKindToolFailure) && event.Parent != nil && *event.Parent == inputToolCall {
						terminal := event
						inputToolOutcome = &terminal
					}
					if event.Kind == model.SessionEventKindAgentFailure {
						t.Fatalf("reply failed after input: %#v", event.Payload)
					}
					if event.Kind == model.SessionEventKindAgentSuccess {
						if event.Payload["text"] != "Selected "+providerReplyMarker || calls.Load() != 2 {
							t.Fatalf("reply = %#v, provider calls = %d", event.Payload, calls.Load())
						}
						requestMutex.Lock()
						requestCount := len(requestBodies)
						requestMutex.Unlock()
						if requestCount != 2 {
							t.Fatal("completed round repeated a provider request")
						}
						if inputResponse.Id == "" || inputToolOutcome == nil {
							t.Fatal("input response or its tool outcome was not persisted")
						}
						if successfulInput {
							if inputToolOutcome.Kind != model.SessionEventKindToolSuccess {
								t.Fatalf("tool outcome after submitted input = %q, want success", inputToolOutcome.Kind)
							}
							output := testToolResultOutput(t, ctx, store, ring, *inputToolOutcome, principal)
							if !strings.Contains(output, answerMarker) {
								t.Fatalf("Gatehouse tool output did not retain the submitted answer: %q", output)
							}
							if outcome == "multiple-inputs" {
								if secondInput.Id == "" || !strings.Contains(output, secondAnswerMarker) {
									t.Fatal("tool call did not retain both form responses")
								}
								var forms, waits int
								if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_session_events WHERE kind = 'input.request'`).Scan(&forms); err != nil || forms != 2 {
									t.Fatalf("forms after recovery: %d (%v), want 2", forms, err)
								}
								if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_status WHERE name = 'gatehouse.session-input'`).Scan(&waits); err != nil || waits != 0 {
									t.Fatalf("unexpected input child workflows: %d (%v)", waits, err)
								}
							}
							err, answer := store.SessionInputResponseResultGet(ctx, inputResponse)
							if err != nil || answer != `{"file":{"file_id":"`+firstAnswer+`"}}` {
								t.Fatalf("stored input result: %d bytes (%v)", len(answer), err)
							}
						} else {
							toolOutput, _ := inputToolOutcome.Payload["output"].(string)
							if inputToolOutcome.Kind != model.SessionEventKindToolFailure || inputToolOutcome.Payload["code"] != "execution_failed" || !strings.Contains(toolOutput, "input failed: cancelled") {
								t.Fatalf("tool outcome after cancelled input = %#v", inputToolOutcome)
							}
							if err, response := store.SessionEventGet(ctx, inputResponse); err != nil || response == nil || response.Payload["message"] != answerMarker {
								t.Fatalf("stored cancellation response = (%#v, %v)", response, err)
							}
						}
						assertDBOSDoesNotContain(t, store, answerMarker)
						assertDBOSDoesNotContain(t, store, secondAnswerMarker)
						assertDBOSDoesNotContain(t, store, providerReplyMarker)
						assertMinimalThinkingRequests(t, ctx, store, session)
						assertToolResultsResolveParent(t, ctx, store, session)
						return
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("reply did not resume after submitted input; provider calls = %d", calls.Load())
		})
	}
}

func assertMinimalThinkingRequests(t *testing.T, ctx context.Context, store *database.Store, session model.SessionRef) {
	t.Helper()
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Kind != model.SessionEventKindThinkingRequest {
			continue
		}
		count++
		if event.Parent == nil || event.AuthorAgent == nil || event.Payload["turn"] == nil {
			t.Fatalf("thinking request lost lifecycle metadata: %#v", event)
		}
		for field := range event.Payload {
			if field != "turn" && field != "previous" {
				t.Fatalf("thinking request contains non-lifecycle field %q", field)
			}
		}
	}
	if count == 0 {
		t.Fatal("no thinking requests were captured")
	}
}

func assertToolResultsResolveParent(t *testing.T, ctx context.Context, store *database.Store, session model.SessionRef) {
	t.Helper()
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind != model.SessionEventKindToolSuccess && event.Kind != model.SessionEventKindToolFailure {
			continue
		}
		if event.Parent == nil || (event.Payload["output"] == nil) == (event.Payload["output_file"] == nil) {
			t.Fatalf("tool result lost output or parent: %#v", event)
		}
		for _, field := range []string{"name", "call_id"} {
			if _, exists := event.Payload[field]; exists {
				t.Fatalf("tool result copied %s from its parent", field)
			}
		}
		err, parent := store.SessionEventGet(ctx, *event.Parent)
		if err != nil || parent == nil || parent.Kind != model.SessionEventKindToolRequest || parent.Payload["name"] != "lisp" || parent.Payload["call_id"] == nil {
			t.Fatalf("tool result cannot resolve its request identity: %#v (%v)", parent, err)
		}
	}
}

func assertDBOSDoesNotContain(t *testing.T, store *database.Store, marker string) {
	t.Helper()
	rows, err := store.QueryContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if !strings.HasPrefix(name, "gatehouse_") && !strings.HasPrefix(name, "sqlite_") {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		stored, err := store.QueryContext(context.Background(), `SELECT * FROM `+quoted)
		if err != nil {
			t.Fatalf("read DBOS table %q: %v", table, err)
		}
		columns, err := stored.Columns()
		if err != nil {
			stored.Close()
			t.Fatalf("read DBOS columns for %q: %v", table, err)
		}
		for stored.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for index := range values {
				destinations[index] = &values[index]
			}
			if err := stored.Scan(destinations...); err != nil {
				stored.Close()
				t.Fatalf("scan DBOS table %q: %v", table, err)
			}
			for index, value := range values {
				var contents []byte
				switch value := value.(type) {
				case []byte:
					contents = value
				case string:
					contents = []byte(value)
				}
				if dbosPayloadContains(contents, marker) {
					stored.Close()
					t.Fatalf("DBOS table %q column %q contains application payload marker", table, columns[index])
				}
			}
		}
		if err := stored.Err(); err != nil {
			stored.Close()
			t.Fatalf("iterate DBOS table %q: %v", table, err)
		}
		if err := stored.Close(); err != nil {
			t.Fatalf("close DBOS table %q: %v", table, err)
		}
	}
}

func dbosPayloadContains(contents []byte, marker string) bool {
	if bytes.Contains(contents, []byte(marker)) {
		return true
	}
	// DBOS_JSON and DBOS_GOB both base64-encode their stored outputs.
	decoded, err := base64.StdEncoding.DecodeString(string(contents))
	return err == nil && bytes.Contains(decoded, []byte(marker))
}

func TestDBOSPayloadInspectionDecodesStoredJSON(t *testing.T) {
	const marker = "PAYLOAD_INSPECTION_SENTINEL"
	payload, err := json.Marshal(map[string]string{"result": marker})
	if err != nil {
		t.Fatal(err)
	}
	for _, contents := range [][]byte{payload, []byte(base64.StdEncoding.EncodeToString(payload))} {
		if !dbosPayloadContains(contents, marker) {
			t.Fatal("payload inspection missed serialized application data")
		}
	}
	if dbosPayloadContains([]byte(base64.StdEncoding.EncodeToString([]byte(`{"id":"sev_ref"}`))), marker) {
		t.Fatal("payload inspection reported a reference as application data")
	}
}
