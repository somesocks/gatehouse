package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func TestRoundAccessRevocationAllowsCurrentRoundToFinish(t *testing.T) {
	for _, removal := range []string{"bot", "group-bot", "workspace-principal", "chat-principal", "project-principal"} {
		for _, tools := range []bool{false, true} {
			outcome := "final"
			if tools {
				outcome = "tool-batch"
			}
			t.Run(removal+"/"+outcome, func(t *testing.T) {
				started, resume := make(chan struct{}, 1), make(chan struct{})
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
					w.Header().Set("Content-Type", "application/json")
					output := []map[string]any{{"type": "message", "status": "completed", "content": []map[string]string{{"type": "output_text", "text": `{"text":"Finished the admitted round","attachments":[]}`}}}}
					if tools {
						output = nil
						for _, id := range []string{"call-1", "call-2"} {
							arguments, _ := json.Marshal(map[string]string{"code": "(int/add 1 2)", "reason": "test admitted batch"})
							output = append(output, map[string]any{"type": "function_call", "call_id": id, "name": "lisp", "arguments": string(arguments)})
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"output": output})
				}))
				defer server.Close()
				var once sync.Once
				release := func() { once.Do(func() { close(resume) }) }
				defer release()

				store, _, request, revoke := roundAccessFixture(t, server.URL, removal)
				select {
				case <-started:
				case <-time.After(10 * time.Second):
					t.Fatal("thinking did not reach the provider")
				}
				revoke()
				release()
				deadline := time.Now().Add(10 * time.Second)
				for {
					err, events := store.SessionEventsGet(context.Background(), request.Session)
					if err != nil {
						t.Fatal(err)
					}
					thinking, thinkingSuccess, toolRequests, toolSuccess := 0, 0, 0, 0
					var success, failure *model.SessionEvent
					for index := range events {
						event := &events[index]
						switch event.Kind {
						case model.SessionEventKindCancelRequest, model.SessionEventKindCancelSuccess, model.SessionEventKindCancelFailure:
							t.Fatal("access revocation created a synthetic cancellation")
						case model.SessionEventKindThinkingRequest:
							thinking++
						case model.SessionEventKindThinkingSuccess:
							thinkingSuccess++
						case model.SessionEventKindToolRequest:
							toolRequests++
						case model.SessionEventKindToolSuccess:
							toolSuccess++
							if event.Payload["output"] != "3" {
								t.Fatalf("tool did not finish the admitted batch: %#v", event.Payload)
							}
						case model.SessionEventKindAgentSuccess:
							success = event
						case model.SessionEventKindAgentFailure:
							failure = event
						}
					}
					if success != nil || failure != nil {
						if thinking != 1 || thinkingSuccess != 1 || calls.Load() != 1 {
							t.Fatalf("rounds continued after revocation: thinking=%d success=%d provider calls=%d", thinking, thinkingSuccess, calls.Load())
						}
						if tools {
							if success != nil || failure == nil || failure.Payload["code"] == "cancelled" || toolRequests != 2 || toolSuccess != 2 {
								t.Fatalf("batch must finish before next round fails: success=%#v failure=%#v tool requests=%d tool successes=%d", success, failure, toolRequests, toolSuccess)
							}
						} else if failure != nil || success == nil || success.Payload["text"] != "Finished the admitted round" {
							t.Fatalf("admitted final answer was rejected: success=%#v failure=%#v", success, failure)
						}
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("round did not finish: %#v", events)
					}
					time.Sleep(testdbos.PollInterval)
				}
			})
		}
	}
}

func roundAccessFixture(t *testing.T, provider, removal string) (*database.Store, *agent.SessionEventReplyRuntime, model.SessionEventRef, func()) {
	t.Helper()
	ctx := context.Background()
	t.Setenv("GATEHOUSE_TEST_ROUND_KEYCHAIN", "test passphrase")
	t.Setenv("GATEHOUSE_TEST_ROUND_API_KEY", "test-key")
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: "pbkdf2-hmac-sha256-v0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	keychains := []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_ROUND_KEYCHAIN"}}}
	err, ring := keychain.NewKeyring(store, keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	state := config.State{
		Keychains:       keychains,
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "test", Revision: 1, Protocol: "openai-responses", Endpoint: &provider, Sources: []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_ROUND_API_KEY"}, Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "test", Model: "test-model", Parameters: `{}`, Compaction: defaultAgentModelCompaction, MaxTurns: 4, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	if removal == "group-bot" {
		state.WorkspaceAgents[0].Default = false
		state.Groups = []config.Group{{WorkspaceID: "engineering", Alias: "team", Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true, Revision: 1}}, Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 1}}}}
	}
	err, registry := migrations.Build(configuration, admittedAgentTestState(state), ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	title := "Existing title"
	session := model.Session{Ref: model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}, AuthorPrincipal: &alice, Name: &title, Enabled: true}
	if removal == "project-principal" {
		id, err := typed_id.New(typed_id.Project)
		if err != nil {
			t.Fatal(err)
		}
		project := model.Project{Ref: model.ProjectRef{Workspace: workspace, Id: id}, Enabled: true}
		if err, _ := store.ProjectsCreate(ctx, project, alice, nil); err != nil {
			t.Fatal(err)
		}
		session.Project = &project.Ref
	}
	if err, _ := store.SessionsCreate(ctx, session, alice); err != nil {
		t.Fatal(err)
	}
	if removal == "group-bot" {
		var group string
		if err := store.QueryRow(`SELECT id FROM gatehouse_groups WHERE alias = 'team'`).Scan(&group); err != nil {
			t.Fatal(err)
		}
		if err, _ := store.SessionParticipantGrantCreate(ctx, session.Ref, alice, nil, &group, "member"); err != nil {
			t.Fatal(err)
		}
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session.Ref, Id: "sev_00000000000000000000000000"}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "Calculate", "agents": []string{bot}}}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "round-access-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntimeWithOptions(dbosContext, store, ring, agent.SessionEventReplyOptions{PollingInterval: testdbos.PollInterval})
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := runtime.CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, message.Ref, model.SessionEventKindAgentRequest)
	if err != nil || request == nil {
		t.Fatalf("agent request = %#v (%v)", request, err)
	}
	if err := testdbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbos.Shutdown(dbosContext, 5*time.Second) })
	revoke := func() {
		t.Helper()
		var err error
		switch removal {
		case "bot":
			_, err = store.Exec(`UPDATE gatehouse_session_grants SET enabled = FALSE WHERE workspace = ? AND session = ? AND agent = ?`, workspace.Id, session.Ref.Id, bot)
		case "group-bot":
			_, err = store.Exec(`UPDATE gatehouse_group_agents SET enabled = FALSE WHERE workspace_id = ? AND agent_id = ?`, workspace.Id, bot)
		case "workspace-principal":
			_, err = store.Exec(`UPDATE gatehouse_workspace_grants SET enabled = FALSE WHERE workspace = ? AND principal = ?`, workspace.Id, alice.Id)
		case "chat-principal":
			_, err = store.Exec(`UPDATE gatehouse_session_grants SET enabled = FALSE WHERE workspace = ? AND session = ? AND principal = ?`, workspace.Id, session.Ref.Id, alice.Id)
		case "project-principal":
			_, err = store.Exec(`UPDATE gatehouse_project_grants SET enabled = FALSE WHERE workspace = ? AND project = ? AND principal = ?`, workspace.Id, session.Project.Id, alice.Id)
		default:
			t.Fatalf("unknown revocation %q", removal)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return store, runtime, request.Ref, revoke
}
