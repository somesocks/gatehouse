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
	err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: "replay", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_REPLAY_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
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
		} else if crashPoint == "retry-replay" {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE INSERT ON operation_outputs
				WHEN NEW.function_name = 'DBOS.sleep' BEGIN SELECT gh_test_crash(); END`
		} else if crashPoint == "decision-rollback" {
			trigger = `CREATE TRIGGER crash_at_provider_boundary BEFORE INSERT ON operation_outputs
				WHEN NEW.function_name = 'gatehouse.agent-provider-decision' BEGIN SELECT gh_test_crash(); END`
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
		{"openai-responses", "provider-failure"},
		{"openai-responses", "invalid-batch"},
		{"openai-responses", "final-rollback"},
		{"openai-chat-completions", "success"},
		{"openai-chat-completions", "cancelled"},
	} {
		protocol, outcome := testCase.protocol, testCase.outcome
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
				arguments, _ := json.Marshal(map[string]string{"code": `(input/ask (input/form "Select" (session-file-picker "file" "File" (json/object))))`, "reason": "Select a session file"})
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
				if (outcome == "retry" || outcome == "retry-replay") && callNumber == 1 {
					w.Header().Set("Retry-After-Ms", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded"}}`))
					return
				}
				var response any
				if protocol == "openai-responses" {
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
			chains := []config.Keychain{{ID: "replay", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_REPLAY_KEYCHAIN"}}}
			chainID, baseURL := "replay", server.URL+"/v1"
			err, ring := keychain.NewKeyring(store, chains, keychain.NewPassphraseSourceResolver())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { ring.Close() }()
			state := config.State{
				Keychains: chains, Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}}, Principals: []config.Principal{{Alias: "alice", Enabled: true}},
				AgentProviders:  []config.AgentProvider{{Alias: "test", Revision: 1, Protocol: protocol, BaseURL: &baseURL, Keychain: &chainID, Sources: []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_REPLAY_API_KEY"}, Enabled: true}},
				AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "test", Model: "test-model", Parameters: `{}`, Compaction: defaultAgentModelCompaction, MaxTurns: 4, MaxOutputTokens: 1024, Enabled: true}},
				WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
			}
			if outcome == "rate-replay" {
				state.WorkspaceAgents[0].RateLimits = &config.WorkspaceAgentRateLimits{
					WorkspaceInput:  &config.TokenBucket{MinimumBalance: -1_000_000, MaximumBalance: 1_000_000, RefillPerMinute: 1_000_000},
					WorkspaceOutput: &config.TokenBucket{MinimumBalance: -1_000_000, MaximumBalance: 1_000_000, RefillPerMinute: 1_000_000},
				}
			}
			err, registry := migrations.Build(configuration, state, ring)
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
			boundaryCrash := outcome == "batch-unstarted" || outcome == "final-replay" || outcome == "retry-replay" || outcome == "decision-rollback" || outcome == "round-final-replay" || outcome == "request-replay"
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
				if outcome == "retry-replay" || outcome == "decision-rollback" {
					wantSuccess = 0
				}
				if thinkingSuccess != wantSuccess || outcome == "batch-unstarted" && toolRequests != 2 || outcome != "batch-unstarted" && toolRequests != 0 {
					t.Fatalf("decision at boundary crash: thinking.success=%d tool.request=%d", thinkingSuccess, toolRequests)
				}
				second, runtime := newRuntime()
				recoveredContext, recoveredRuntime = second, runtime
				defer dbos.Shutdown(second, 10*time.Second)
				if err := runtime.Reconcile(); err != nil {
					t.Fatal(err)
				}
				if outcome != "batch-unstarted" {
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
					if outcome == "retry-replay" {
						wantCalls = 3
					}
					if err != nil || result.Id == "" || calls.Load() != wantCalls {
						t.Fatalf("final reply replay = (%#v, %v), provider calls=%d", result, err, calls.Load())
					}
					if outcome == "round-final-replay" || outcome == "request-replay" {
						requestMutex.Lock()
						identical := len(requestBodies) == 2 && bytes.Equal(requestBodies[0], requestBodies[1])
						requestMutex.Unlock()
						if !identical {
							t.Fatal("unfinished round did not reuse its saved provider request")
						}
					}
					if err, final := store.SessionEventGet(ctx, result); err != nil || final == nil || final.Payload["text"] != "Selected "+providerReplyMarker {
						t.Fatalf("committed final reply = (%#v, %v)", final, err)
					}
					var count int
					if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_session_events WHERE kind = 'agent.success'`).Scan(&count); err != nil || count != 1 {
						t.Fatalf("final reply count = %d, err=%v", count, err)
					}
					assertDBOSDoesNotContain(t, store, providerReplyMarker)
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
			inputWorkflowID := "session-input:" + input.Id
			inputWorkflowDeadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(inputWorkflowDeadline) {
				var exists bool
				if err := store.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM workflow_status WHERE workflow_uuid = ?)`, inputWorkflowID).Scan(&exists); err != nil {
					t.Fatal(err)
				}
				if exists {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			var inputWorkflowExists bool
			if err := store.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM workflow_status WHERE workflow_uuid = ?)`, inputWorkflowID).Scan(&inputWorkflowExists); err != nil || !inputWorkflowExists {
				t.Fatalf("input workflow %q was not restarted: exists=%v err=%v", inputWorkflowID, inputWorkflowExists, err)
			}
			const answerMarker = "DBOS_INPUT_PAYLOAD_MUST_STAY_IN_GATEHOUSE_7f3c2a"
			responseKind := model.SessionEventKindInputSuccess
			responsePayload := map[string]interface{}{}
			if outcome == "success" || outcome == "batch-unstarted" || outcome == "rate-replay" {
				if err, _ := store.SessionInputDraftSet(ctx, input, []string{"file"}, json.RawMessage(`{"file_id":"`+answerMarker+`"}`)); err != nil {
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
			if err, _ := store.SessionInputResponseCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &input, Kind: responseKind, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: responsePayload}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Reconcile(); err != nil {
				t.Fatal(err)
			}
			deadline = time.Now().Add(15 * time.Second)
			var inputResponse model.SessionEventRef
			var inputToolOutcome *model.SessionEvent
			for time.Now().Before(deadline) {
				err, events := store.SessionEventsGet(ctx, session)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
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
						if outcome == "success" || outcome == "batch-unstarted" || outcome == "rate-replay" {
							if inputToolOutcome.Kind != model.SessionEventKindToolSuccess {
								t.Fatalf("tool outcome after submitted input = %q, want success", inputToolOutcome.Kind)
							}
							if output, _ := inputToolOutcome.Payload["output"].(string); !strings.Contains(output, answerMarker) {
								t.Fatalf("Gatehouse tool output did not retain the submitted answer: %q", output)
							}
							err, answer := store.SessionInputResponseResultGet(ctx, inputResponse)
							if err != nil || answer != `{"file":{"file_id":"`+answerMarker+`"}}` {
								t.Fatalf("stored input result = (%q, %v)", answer, err)
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
						assertDBOSDoesNotContain(t, store, providerReplyMarker)
						return
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("reply did not resume after submitted input; provider calls = %d", calls.Load())
		})
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
