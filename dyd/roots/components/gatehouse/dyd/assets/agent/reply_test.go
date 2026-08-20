package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestSessionEventReplyRuntimeCreatesOneBuiltinReply(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	aliceName := "Alice"
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:     []config.Principal{{Alias: "alice", Name: &aliceName, Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{Alias: "automatic", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Automatic reply."}`, Enabled: true},
			{Alias: "requested", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Requested reply."}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", ModelAlias: "automatic", Priority: 2, MaxTurns: config.DefaultWorkspaceAgentMaxTurns, Enabled: true},
			{WorkspaceID: "engineering", ModelAlias: "requested", Priority: 1, MaxTurns: config.DefaultWorkspaceAgentMaxTurns, Enabled: true},
		},
	}
	keyringErr, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if keyringErr != nil {
		t.Fatal(keyringErr)
	}
	defer keyring.Close()
	buildErr, set := migrations.Build(configuration, state, keyring)
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	var requestedID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'requested'`).Scan(&requestedID); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello", "agent": requestedID},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}

	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	runtimeErr, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
	if runtimeErr != nil {
		t.Fatal(runtimeErr)
	}
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 4 {
			started := events[1]
			completed := events[2]
			reply := events[3]
			if started.Kind != "thinking.started" || started.Parent == nil || *started.Parent != message.Ref || started.AuthorAgent == nil || started.AuthorAgent.Model.Id != requestedID || started.Payload["turn"] != float64(0) || started.Ref.Id == "" {
				t.Fatalf("thinking start event = %#v", started)
			}
			if completed.Kind != "thinking.completed" || completed.Parent == nil || *completed.Parent != started.Ref || completed.AuthorAgent == nil || completed.AuthorAgent.Model.Id != requestedID || completed.Ref.Id == "" {
				t.Fatalf("thinking completion event = %#v", completed)
			}
			if reply.Kind != "message.text" || reply.Parent == nil || *reply.Parent != message.Ref || reply.AuthorAgent == nil || reply.AuthorAgent.Model.Id != requestedID || reply.Payload["text"] != "Requested reply." || reply.Ref.Id == "" {
				t.Fatalf("reply event = %#v", reply)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session events = %#v, want message, thinking lifecycle, and reply", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}
	for {
		err, name := store.SessionNameGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		if name != nil {
			if *name != "Requested reply." {
				t.Fatalf("session name = %q, want selected agent title", *name)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session name was not generated")
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, nameTasks := store.SessionNameTasksGet(ctx, 10)
	if err != nil || len(nameTasks) != 0 {
		t.Fatalf("SessionNameTasksGet() = (%#v, %v)", nameTasks, err)
	}
}

func TestSessionEventReplyRuntimeCancelsQueuedReply(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	aliceName := "Alice"
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:     []config.Principal{{Alias: "alice", Name: &aliceName, Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{
			Alias: "fallback", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Fallback reply."}`, Enabled: true,
		}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "fallback", Priority: 1, MaxTurns: config.DefaultWorkspaceAgentMaxTurns, Enabled: true}},
	}
	keyringErr, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if keyringErr != nil {
		t.Fatal(keyringErr)
	}
	defer keyring.Close()
	buildErr, set := migrations.Build(configuration, state, keyring)
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello"},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"},
		Parent:          &message.Ref,
		Kind:            "cancel.request",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{},
	}
	if err, _ := store.SessionEventsCreate(ctx, cancellation); err != nil {
		t.Fatal(err)
	}

	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-cancel-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	runtimeErr, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
	if runtimeErr != nil {
		t.Fatal(runtimeErr)
	}
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 3 {
			success := events[2]
			if success.Kind != "cancel.success" || success.Parent == nil || *success.Parent != cancellation.Ref || success.AuthorPrincipal == nil || success.AuthorPrincipal.Ref != alice || success.AuthorPrincipal.Name == nil || *success.AuthorPrincipal.Name != aliceName {
				t.Fatalf("cancellation success = %#v", success)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session events = %#v, want message, cancellation request, and cancellation success", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}
}

func TestSessionEventReplyRuntimeDeliversApprovalDecisions(t *testing.T) {
	for _, test := range []struct {
		name                string
		code                string
		approvalDescription string
		project             bool
		projectUpdate       bool
		decision            string
		toolKind            string
		notes               int
	}{
		{name: "session approved", code: `(session/notes/create "Decision" "" "# Decision")`, approvalDescription: "Create a session note", decision: "approval.approved", toolKind: "tool.success", notes: 1},
		{name: "session rejected", code: `(session/notes/create "Decision" "" "# Decision")`, approvalDescription: "Create a session note", decision: "approval.rejected", toolKind: "tool.failure", notes: 0},
		{name: "project approved", code: `(project/notes/create "Decision" "" "# Decision")`, approvalDescription: "Create a project note", project: true, decision: "approval.approved", toolKind: "tool.success", notes: 1},
		{name: "project rejected", code: `(project/notes/create "Decision" "" "# Decision")`, approvalDescription: "Create a project note", project: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0},
		{name: "project info approved", code: `(project/info/set "Roadmap" "Current priorities")`, approvalDescription: "Update project details", project: true, projectUpdate: true, decision: "approval.approved", toolKind: "tool.success", notes: 0},
		{name: "project info rejected", code: `(project/info/set "Roadmap" "Current priorities")`, approvalDescription: "Update project details", project: true, projectUpdate: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mutex sync.Mutex
			completions := 0
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
					http.Error(response, "unexpected completion request", http.StatusBadRequest)
					return
				}
				mutex.Lock()
				completions++
				completion := completions
				mutex.Unlock()
				response.Header().Set("Content-Type", "application/json")
				if completion == 1 {
					arguments, _ := json.Marshal(map[string]string{"code": test.code, "reason": "Create the requested decision note."})
					_, _ = response.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lisp","arguments":%q}}]}}]}`, string(arguments))))
					return
				}
				_, _ = response.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Done."}}]}`))
			}))
			defer server.Close()

			ctx := context.Background()
			configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
			err, store := database.Open(ctx, configuration)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			t.Setenv("APPROVAL_TEST_KEYCHAIN", "passphrase")
			t.Setenv("APPROVAL_TEST_API_KEY", "test-key")
			baseURL := server.URL + "/v1"
			keychainID := "default"
			aliceName := "Alice"
			state := config.State{
				Keychains: []config.Keychain{{ID: keychainID, Sources: []config.KeychainPassphraseSource{"env:APPROVAL_TEST_KEYCHAIN"}}},
				Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
				Principals: []config.Principal{{Alias: "alice", Name: &aliceName, Enabled: true}},
				AgentProviders: []config.AgentProvider{{
					Alias: "openai", Revision: 1, Protocol: "openai-chat-completions", BaseURL: &baseURL, Keychain: &keychainID, Sources: []config.AgentProviderAPIKeySource{"env:APPROVAL_TEST_API_KEY"}, Enabled: true,
				}},
				AgentModels:    []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "openai", Model: "test-model", Parameters: `{"reasoning_effort":"none"}`, Enabled: true}},
				WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "assistant", Priority: 1, MaxTurns: 1, Enabled: true}},
			}
			keyringErr, keyring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
			if keyringErr != nil {
				t.Fatal(keyringErr)
			}
			defer keyring.Close()
			buildErr, set := migrations.Build(configuration, state, keyring)
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			if err := migrations.Run(ctx, store, set); err != nil {
				t.Fatal(err)
			}

			workspace := workspaceRef(t, ctx, store, "engineering")
			principal := principalRef(t, ctx, store, "alice")
			principalRecord := model.Principal{Ref: principal, Name: &aliceName, Enabled: true}
			session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
			if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principal, Enabled: true}, principal); err != nil {
				t.Fatal(err)
			}
			if err, _ := store.SessionNameSet(ctx, session, "Approval test"); err != nil {
				t.Fatal(err)
			}
			var project *model.ProjectRef
			if test.project {
				id, err := typed_id.New(typed_id.Project)
				if err != nil {
					t.Fatal(err)
				}
				project = &model.ProjectRef{Workspace: workspace, Id: id}
				if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: *project, Enabled: true}, principal, nil); err != nil {
					t.Fatal(err)
				}
				if err, _ := store.SessionProjectSet(ctx, session, project, principal); err != nil {
					t.Fatal(err)
				}
			}
			message := model.SessionEvent{
				Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &principalRecord, Payload: map[string]interface{}{"text": "Create the decision note."},
			}
			if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
				t.Fatal(err)
			}

			dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-approval-test", SQLiteSystemDB: store.DB})
			if err != nil {
				t.Fatal(err)
			}
			runtimeErr, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
			if runtimeErr != nil {
				t.Fatal(runtimeErr)
			}
			if err := dbos.Launch(dbosContext); err != nil {
				t.Fatal(err)
			}
			defer dbos.Shutdown(dbosContext, time.Second)
			if err := runtime.Reconcile(); err != nil {
				t.Fatal(err)
			}

			approval := waitForApprovalRequest(t, ctx, store, session)
			if approval.Payload["description"] != test.approvalDescription {
				t.Fatalf("approval description = %#v", approval.Payload["description"])
			}
			noteCount := 0
			if project != nil {
				err, notes := store.ProjectNotesGet(ctx, *project, principal)
				noteCount = len(notes)
				if err != nil || noteCount != 0 {
					t.Fatalf("notes before decision = (%#v, %v)", notes, err)
				}
			} else {
				err, notes := store.SessionNotesGet(ctx, session, principal)
				noteCount = len(notes)
				if err != nil || noteCount != 0 {
					t.Fatalf("notes before decision = (%#v, %v)", notes, err)
				}
			}
			decisionID, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				t.Fatal(err)
			}
			decision := model.SessionEvent{
				Ref: model.SessionEventRef{Session: session, Id: decisionID}, Parent: &approval.Ref, Kind: test.decision, AuthorPrincipal: &principalRecord, Payload: map[string]interface{}{},
			}
			if err, _ := store.SessionApprovalResponseCreate(ctx, decision); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Reconcile(); err != nil {
				t.Fatal(err)
			}

			result := waitForToolResult(t, ctx, store, session, test.toolKind)
			if result.Parent == nil {
				t.Fatalf("tool result has no parent: %#v", result)
			}
			noteCount = 0
			if project != nil {
				err, notes := store.ProjectNotesGet(ctx, *project, principal)
				noteCount = len(notes)
				if err != nil || noteCount != test.notes {
					t.Fatalf("notes after decision = (%#v, %v), want %d", notes, err, test.notes)
				}
			} else {
				err, notes := store.SessionNotesGet(ctx, session, principal)
				noteCount = len(notes)
				if err != nil || noteCount != test.notes {
					t.Fatalf("notes after decision = (%#v, %v), want %d", notes, err, test.notes)
				}
			}
			if test.projectUpdate {
				err, updated := store.ProjectGet(ctx, *project, principal)
				if err != nil || updated == nil {
					t.Fatalf("ProjectGet() = (%#v, %v)", updated, err)
				}
				if test.decision == "approval.approved" && (updated.Name == nil || *updated.Name != "Roadmap" || updated.Description == nil || *updated.Description != "Current priorities") {
					t.Fatalf("updated project = %#v", updated)
				}
				if test.decision == "approval.rejected" && (updated.Name != nil || updated.Description != nil) {
					t.Fatalf("rejected project update = %#v", updated)
				}
			}
			waitForAgentReply(t, ctx, store, session, message.Ref)
			err, tasks := store.SessionApprovalDecisionTasksGet(ctx, 10)
			if err != nil || len(tasks) != 0 {
				t.Fatalf("approval decision tasks = (%#v, %v)", tasks, err)
			}
		})
	}
}

func waitForApprovalRequest(t *testing.T, ctx context.Context, store *database.Store, session model.SessionRef) model.SessionEvent {
	t.Helper()
	return waitForSessionEvent(t, ctx, store, session, func(event model.SessionEvent) bool {
		return event.Kind == "approval.request"
	})
}

func waitForToolResult(t *testing.T, ctx context.Context, store *database.Store, session model.SessionRef, kind string) model.SessionEvent {
	t.Helper()
	return waitForSessionEvent(t, ctx, store, session, func(event model.SessionEvent) bool {
		return event.Kind == kind
	})
}

func waitForAgentReply(t *testing.T, ctx context.Context, store *database.Store, session model.SessionRef, parent model.SessionEventRef) model.SessionEvent {
	t.Helper()
	return waitForSessionEvent(t, ctx, store, session, func(event model.SessionEvent) bool {
		return event.Kind == "message.text" && event.Parent != nil && *event.Parent == parent && event.AuthorAgent != nil && event.Payload["text"] == "Done."
	})
}

func waitForSessionEvent(t *testing.T, ctx context.Context, store *database.Store, session model.SessionRef, match func(model.SessionEvent) bool) model.SessionEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if match(event) {
				return event
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("matching session event was not created: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func workspaceRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.WorkspaceRef {
	t.Helper()
	err, workspace := store.WorkspaceRefGetByAlias(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	if workspace == nil {
		t.Fatalf("workspace alias %q was not found", alias)
	}
	return *workspace
}

func principalRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.PrincipalRef {
	t.Helper()
	var id string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = ?`, alias).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return model.PrincipalRef{Id: id}
}
