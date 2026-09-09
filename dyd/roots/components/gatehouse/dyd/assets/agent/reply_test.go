package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

var defaultAgentModelCompaction = config.AgentModelCompaction{
	Algorithm:    "mcmtr",
	HistoryBytes: config.DefaultAgentModelHistoryBytes,
	BufferBytes:  config.DefaultAgentModelBufferBytes,
}

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
			{Alias: "automatic", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Automatic reply."}`, Compaction: defaultAgentModelCompaction, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true},
			{Alias: "requested", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Requested reply."}`, Compaction: defaultAgentModelCompaction, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", ModelAlias: "automatic", Revision: 1, Priority: 2, Enabled: true},
			{WorkspaceID: "engineering", ModelAlias: "requested", Revision: 1, Priority: 1, Enabled: true},
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
			Alias: "fallback", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Fallback reply."}`, Compaction: defaultAgentModelCompaction, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true,
		}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "fallback", Revision: 1, Priority: 1, Enabled: true}},
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
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	aliceName := "Alice"
	var completionMutex sync.Mutex
	completionCodes := make(map[string]string)
	completionCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
			http.Error(response, "unexpected completion request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(response, "could not read completion request", http.StatusBadRequest)
			return
		}
		completionMutex.Lock()
		message := ""
		code := ""
		for candidate, candidateCode := range completionCodes {
			if bytes.Contains(body, []byte(candidate)) {
				message = candidate
				code = candidateCode
				break
			}
		}
		if message == "" {
			completionMutex.Unlock()
			http.Error(response, "unexpected completion message", http.StatusBadRequest)
			return
		}
		completionCounts[message]++
		completion := completionCounts[message]
		completionMutex.Unlock()
		response.Header().Set("Content-Type", "application/json")
		if completion == 1 {
			arguments, _ := json.Marshal(map[string]string{"code": code, "reason": "Perform the requested note operation."})
			_, _ = response.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lisp","arguments":%q}}]}}]}`, string(arguments))))
			return
		}
		_, _ = response.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"text\":\"Done.\",\"attachments\":[]}"}}]}`))
	}))
	defer server.Close()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("APPROVAL_TEST_KEYCHAIN", "passphrase")
	t.Setenv("APPROVAL_TEST_API_KEY", "test-key")
	baseURL := server.URL + "/v1"
	keychainID := "default"
	state := config.State{
		Keychains:  []config.Keychain{{ID: keychainID, Sources: []config.KeychainPassphraseSource{"env:APPROVAL_TEST_KEYCHAIN"}}},
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Name: &aliceName, Enabled: true}, {Alias: "member", Enabled: true}},
		AgentProviders: []config.AgentProvider{{
			Alias: "openai", Revision: 1, Protocol: "openai-chat-completions", BaseURL: &baseURL, Keychain: &keychainID, Sources: []config.AgentProviderAPIKeySource{"env:APPROVAL_TEST_API_KEY"}, Enabled: true,
		}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "openai", Model: "test-model", Parameters: `{"reasoning_effort":"none"}`, Compaction: defaultAgentModelCompaction, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "assistant", Revision: 1, Priority: 1, Enabled: true}},
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
	type approvalCase struct {
		name          string
		project       *model.ProjectRef
		projectUpdate bool
		taskUpdate    bool
		taskRemove    bool
		taskCreated   bool
		decision      string
		toolKind      string
		notes         int
		tasks         int
		session       model.SessionRef
		message       model.SessionEvent
	}
	cases := make([]approvalCase, 0, 22)

	for _, test := range []struct {
		name          string
		code          string
		project       bool
		projectUpdate bool
		noteRemove    bool
		taskUpdate    bool
		taskRemove    bool
		taskCreated   bool
		decision      string
		toolKind      string
		notes         int
		tasks         int
	}{
		{name: "session create approved", code: `(session/notes/create "Decision" "" "# Decision")`, decision: "approval.approved", toolKind: "tool.success", notes: 1},
		{name: "session create rejected", code: `(session/notes/create "Decision" "" "# Decision")`, decision: "approval.rejected", toolKind: "tool.failure", notes: 0},
		{name: "project create approved", code: `(project/notes/create "Decision" "" "# Decision")`, project: true, decision: "approval.approved", toolKind: "tool.success", notes: 1},
		{name: "project create rejected", code: `(project/notes/create "Decision" "" "# Decision")`, project: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0},
		{name: "session task create approved", code: `(session/tasks/create "Review design" "# Review" "ready")`, decision: "approval.approved", toolKind: "tool.success", notes: 0, tasks: 1, taskCreated: true},
		{name: "session task create rejected", code: `(session/tasks/create "Review design" "# Review" "ready")`, decision: "approval.rejected", toolKind: "tool.failure", notes: 0, tasks: 0},
		{name: "project task create approved", code: `(project/tasks/create "Review design" "# Review" "ready")`, project: true, decision: "approval.approved", toolKind: "tool.success", notes: 0, tasks: 1, taskCreated: true},
		{name: "project task create rejected", code: `(project/tasks/create "Review design" "# Review" "ready")`, project: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0, tasks: 0},
		{name: "session task update approved", taskUpdate: true, decision: "approval.approved", toolKind: "tool.success", notes: 0, tasks: 1},
		{name: "session task update rejected", taskUpdate: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0, tasks: 1},
		{name: "project task update approved", project: true, taskUpdate: true, decision: "approval.approved", toolKind: "tool.success", notes: 0, tasks: 1},
		{name: "project task update rejected", project: true, taskUpdate: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0, tasks: 1},
		{name: "session task remove approved", taskRemove: true, decision: "approval.approved", toolKind: "tool.success", notes: 0, tasks: 0},
		{name: "session task remove rejected", taskRemove: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0, tasks: 1},
		{name: "project task remove approved", project: true, taskRemove: true, decision: "approval.approved", toolKind: "tool.success", notes: 0, tasks: 0},
		{name: "project task remove rejected", project: true, taskRemove: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0, tasks: 1},
		{name: "project info approved", code: `(project/info/set "Roadmap" "Current priorities")`, project: true, projectUpdate: true, decision: "approval.approved", toolKind: "tool.success", notes: 0},
		{name: "project info rejected", code: `(project/info/set "Roadmap" "Current priorities")`, project: true, projectUpdate: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 0},
		{name: "session remove approved", noteRemove: true, decision: "approval.approved", toolKind: "tool.success", notes: 0},
		{name: "session remove rejected", noteRemove: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 1},
		{name: "project remove approved", project: true, noteRemove: true, decision: "approval.approved", toolKind: "tool.success", notes: 0},
		{name: "project remove rejected", project: true, noteRemove: true, decision: "approval.rejected", toolKind: "tool.failure", notes: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := test.code
			sessionID, err := typed_id.New(typed_id.Session)
			if err != nil {
				t.Fatal(err)
			}
			session := model.SessionRef{Workspace: workspace, Id: sessionID}
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
			if test.noteRemove {
				if project != nil {
					id, err := typed_id.New(typed_id.ProjectNote)
					if err != nil {
						t.Fatal(err)
					}
					if err, _ := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: *project, Id: id}, Title: "Decision", Body: "# Decision"}, principal); err != nil {
						t.Fatal(err)
					}
					code = fmt.Sprintf(`(project/notes/remove "%s")`, id)
				} else {
					id, err := typed_id.New(typed_id.SessionNote)
					if err != nil {
						t.Fatal(err)
					}
					if err, _ := store.SessionNoteCreate(ctx, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: id}, Title: "Decision", Body: "# Decision"}, principal); err != nil {
						t.Fatal(err)
					}
					code = fmt.Sprintf(`(session/notes/remove "%s")`, id)
				}
			}
			if test.taskUpdate || test.taskRemove {
				if project != nil {
					id, err := typed_id.New(typed_id.ProjectTask)
					if err != nil {
						t.Fatal(err)
					}
					if err, _ := store.ProjectTaskCreate(ctx, model.ProjectTask{Ref: model.ProjectTaskRef{Project: *project, Id: id}, Title: "Review design", Status: "ready"}, principal); err != nil {
						t.Fatal(err)
					}
					if test.taskUpdate {
						code = fmt.Sprintf(`(project/tasks/update "%s" "Review design" "# Review" "done")`, id)
					} else {
						code = fmt.Sprintf(`(project/tasks/remove "%s")`, id)
					}
				} else {
					id, err := typed_id.New(typed_id.SessionTask)
					if err != nil {
						t.Fatal(err)
					}
					if err, _ := store.SessionTaskCreate(ctx, model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: id}, Title: "Review design", Status: "ready"}, principal); err != nil {
						t.Fatal(err)
					}
					if test.taskUpdate {
						code = fmt.Sprintf(`(session/tasks/update "%s" "Review design" "# Review" "done")`, id)
					} else {
						code = fmt.Sprintf(`(session/tasks/remove "%s")`, id)
					}
				}
			}
			messageID, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				t.Fatal(err)
			}
			messageText := "Create the decision note " + sessionID
			completionMutex.Lock()
			completionCodes[messageText] = code
			completionCounts[messageText] = 0
			completionMutex.Unlock()
			message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: messageID}, Kind: "message.text", AuthorPrincipal: &principalRecord, Payload: map[string]interface{}{"text": messageText}}
			if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Reconcile(); err != nil {
				t.Fatal(err)
			}

			approval := waitForApprovalRequest(t, ctx, store, session)
			expectedBefore := 0
			if test.noteRemove {
				expectedBefore = 1
			}
			noteCount := 0
			if project != nil {
				err, notes := store.ProjectNotesGet(ctx, *project, principal)
				noteCount = len(notes)
				if err != nil || noteCount != expectedBefore {
					t.Fatalf("notes before decision = (%#v, %v), want %d", notes, err, expectedBefore)
				}
			} else {
				err, notes := store.SessionNotesGet(ctx, session, principal)
				noteCount = len(notes)
				if err != nil || noteCount != expectedBefore {
					t.Fatalf("notes before decision = (%#v, %v), want %d", notes, err, expectedBefore)
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
			cases = append(cases, approvalCase{name: test.name, project: project, projectUpdate: test.projectUpdate, taskUpdate: test.taskUpdate, taskRemove: test.taskRemove, taskCreated: test.taskCreated, decision: test.decision, toolKind: test.toolKind, notes: test.notes, tasks: test.tasks, session: session, message: message})
		})
	}
	// All tool calls are waiting on independent approvals, so deliver their decisions together.
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.name+" result", func(t *testing.T) {
			result := waitForToolResult(t, ctx, store, test.session, test.toolKind)
			if result.Parent == nil {
				t.Fatalf("tool result has no parent: %#v", result)
			}
			noteCount := 0
			if test.project != nil {
				err, notes := store.ProjectNotesGet(ctx, *test.project, principal)
				noteCount = len(notes)
				if err != nil || noteCount != test.notes {
					t.Fatalf("notes after decision = (%#v, %v), want %d", notes, err, test.notes)
				}
			} else {
				err, notes := store.SessionNotesGet(ctx, test.session, principal)
				noteCount = len(notes)
				if err != nil || noteCount != test.notes {
					t.Fatalf("notes after decision = (%#v, %v), want %d", notes, err, test.notes)
				}
			}
			if test.projectUpdate {
				err, updated := store.ProjectGet(ctx, *test.project, principal)
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
			if test.project != nil {
				err, tasks := store.ProjectTasksGet(ctx, *test.project, principal)
				if err != nil || len(tasks) != test.tasks {
					t.Fatalf("project tasks after decision = (%#v, %v), want %d", tasks, err, test.tasks)
				}
				if test.taskCreated && (tasks[0].CreatorAgent == nil || tasks[0].CreatorAgent.Model.Id == "" || tasks[0].UpdaterAgent == nil || tasks[0].UpdaterAgent.Model.Id == "") {
					t.Fatalf("project task agent attribution = %#v", tasks[0])
				}
				if test.taskUpdate && test.decision == "approval.approved" && (tasks[0].UpdaterAgent == nil || tasks[0].UpdaterAgent.Model.Id == "") {
					t.Fatalf("project task updater attribution = %#v", tasks[0])
				}
			} else {
				err, tasks := store.SessionTasksGet(ctx, test.session, principal)
				if err != nil || len(tasks) != test.tasks {
					t.Fatalf("session tasks after decision = (%#v, %v), want %d", tasks, err, test.tasks)
				}
				if test.taskCreated && (tasks[0].CreatorAgent == nil || tasks[0].CreatorAgent.Model.Id == "" || tasks[0].UpdaterAgent == nil || tasks[0].UpdaterAgent.Model.Id == "") {
					t.Fatalf("session task agent attribution = %#v", tasks[0])
				}
				if test.taskUpdate && test.decision == "approval.approved" && (tasks[0].UpdaterAgent == nil || tasks[0].UpdaterAgent.Model.Id == "") {
					t.Fatalf("session task updater attribution = %#v", tasks[0])
				}
			}
			waitForAgentReply(t, ctx, store, test.session, test.message.Ref)
			err, tasks := store.SessionApprovalDecisionTasksGet(ctx, 10)
			if err != nil || len(tasks) != 0 {
				t.Fatalf("approval decision tasks = (%#v, %v)", tasks, err)
			}
		})
	}
	t.Run("member task mutations are unavailable", func(t *testing.T) {
		member := principalRef(t, ctx, store, "member")
		memberRecord := model.Principal{Ref: member, Enabled: true}
		sessionID, err := typed_id.New(typed_id.Session)
		if err != nil {
			t.Fatal(err)
		}
		session := model.SessionRef{Workspace: workspace, Id: sessionID}
		if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principal, Enabled: true}, principal); err != nil {
			t.Fatal(err)
		}
		if err, _ := store.SessionNameSet(ctx, session, "Member authorization test"); err != nil {
			t.Fatal(err)
		}
		projectID, err := typed_id.New(typed_id.Project)
		if err != nil {
			t.Fatal(err)
		}
		project := model.ProjectRef{Workspace: workspace, Id: projectID}
		if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, principal, nil); err != nil {
			t.Fatal(err)
		}
		if err, _ := store.SessionProjectSet(ctx, session, &project, principal); err != nil {
			t.Fatal(err)
		}
		for _, grant := range []struct {
			kind       string
			table     string
			resource  string
			resourceID string
		}{
			{kind: typed_id.SessionGrant, table: "gatehouse_session_grants", resource: "session", resourceID: session.Id},
			{kind: typed_id.ProjectGrant, table: "gatehouse_project_grants", resource: "project", resourceID: project.Id},
		} {
			grantID, err := typed_id.New(grant.kind)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.ExecContext(ctx, "INSERT INTO "+grant.table+" (id, workspace, "+grant.resource+", role, principal, \"group\", enabled) VALUES (?, ?, ?, 'member', ?, NULL, TRUE)", grantID, workspace.Id, grant.resourceID, member.Id); err != nil {
				t.Fatal(err)
			}
		}
		err, sessionRoles := store.SessionRolesGet(ctx, session, member)
		if err != nil || len(sessionRoles) != 1 || sessionRoles[0] != "member" {
			t.Fatalf("SessionRolesGet() for member = (%#v, %v)", sessionRoles, err)
		}
		err, projectRoles := store.ProjectRolesGet(ctx, project, member)
		if err != nil || len(projectRoles) != 1 || projectRoles[0] != "member" {
			t.Fatalf("ProjectRolesGet() for member = (%#v, %v)", projectRoles, err)
		}
		sessionTaskID, err := typed_id.New(typed_id.SessionTask)
		if err != nil {
			t.Fatal(err)
		}
		sessionTaskDescription := "Session task body"
		if err, _ := store.SessionTaskCreate(ctx, model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: sessionTaskID}, Title: "Session task", Description: &sessionTaskDescription, Status: "ready"}, principal); err != nil {
			t.Fatal(err)
		}
		projectTaskID, err := typed_id.New(typed_id.ProjectTask)
		if err != nil {
			t.Fatal(err)
		}
		projectTaskDescription := "Project task body"
		if err, _ := store.ProjectTaskCreate(ctx, model.ProjectTask{Ref: model.ProjectTaskRef{Project: project, Id: projectTaskID}, Title: "Project task", Description: &projectTaskDescription, Status: "ready"}, principal); err != nil {
			t.Fatal(err)
		}
		code := fmt.Sprintf(`(list
  (session/tasks/list)
  (project/tasks/list)
  (error/value (error/catch (session/tasks/create "New session task" "" "ready")))
  (error/value (error/catch (session/tasks/update %q "Session task" "" "done")))
  (error/value (error/catch (session/tasks/remove %q)))
  (error/value (error/catch (project/tasks/create "New project task" "" "ready")))
  (error/value (error/catch (project/tasks/update %q "Project task" "" "done")))
  (error/value (error/catch (project/tasks/remove %q))))`, sessionTaskID, sessionTaskID, projectTaskID, projectTaskID)
		messageText := "Member task authorization " + sessionID
		completionMutex.Lock()
		completionCodes[messageText] = code
		completionCounts[messageText] = 0
		completionMutex.Unlock()
		messageID, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			t.Fatal(err)
		}
		message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: messageID}, Kind: "message.text", AuthorPrincipal: &memberRecord, Payload: map[string]interface{}{"text": messageText}}
		if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Reconcile(); err != nil {
			t.Fatal(err)
		}
		approvals := map[string]bool{}
		for range 6 {
			approval := waitForSessionEvent(t, ctx, store, session, func(event model.SessionEvent) bool {
				return event.Kind == "approval.request" && !approvals[event.Ref.Id]
			})
			approvals[approval.Ref.Id] = true
			decisionID, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				t.Fatal(err)
			}
			decision := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: decisionID}, Parent: &approval.Ref, Kind: "approval.approved", AuthorPrincipal: &memberRecord, Payload: map[string]interface{}{}}
			if err, _ := store.SessionApprovalResponseCreate(ctx, decision); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Reconcile(); err != nil {
				t.Fatal(err)
			}
		}
		result := waitForToolResult(t, ctx, store, session, "tool.success")
		output, _ := result.Payload["output"].(string)
		for _, name := range []string{"session/tasks/create", "session/tasks/update", "session/tasks/remove", "project/tasks/create", "project/tasks/update", "project/tasks/remove"} {
			if !strings.Contains(output, name+" is unavailable") {
				t.Fatalf("member tool output = %q, want %s unavailable", output, name)
			}
		}
		err, sessionTasks := store.SessionTasksGet(ctx, session, principal)
		if err != nil || len(sessionTasks) != 1 || sessionTasks[0].Title != "Session task" || sessionTasks[0].Status != "ready" {
			t.Fatalf("session tasks after member tool call = (%#v, %v)", sessionTasks, err)
		}
		err, projectTasks := store.ProjectTasksGet(ctx, project, principal)
		if err != nil || len(projectTasks) != 1 || projectTasks[0].Title != "Project task" || projectTasks[0].Status != "ready" {
			t.Fatalf("project tasks after member tool call = (%#v, %v)", projectTasks, err)
		}
		waitForAgentReply(t, ctx, store, session, message.Ref)
	})
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
