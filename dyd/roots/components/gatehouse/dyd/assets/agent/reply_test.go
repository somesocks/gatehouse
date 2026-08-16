package agent_test

import (
	"context"
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
)

func TestSessionEventReplyRuntimeCreatesOneBuiltinReply(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Key: "engineering", Enabled: true}},
		Principals:     []config.Principal{{ID: "alice", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{ID: "automatic", Revision: 1, Provider: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Automatic reply."}`, Enabled: true},
			{ID: "requested", Revision: 1, Provider: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Requested reply."}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Model: "automatic", Priority: 2, MaxTurns: config.DefaultWorkspaceAgentMaxTurns, Enabled: true},
			{WorkspaceID: "engineering", Model: "requested", Priority: 1, MaxTurns: config.DefaultWorkspaceAgentMaxTurns, Enabled: true},
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
	alice := model.PrincipalRef{Id: "alice"}
	session := model.SessionRef{Workspace: workspace, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "a8607728-3072-4f6c-9bd4-d9f584b7c892"},
		Kind:            "message.text",
		AuthorPrincipal: &alice,
		Payload:         map[string]interface{}{"text": "hello", "agent": "requested"},
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
			if started.Kind != "thinking.started" || started.Parent == nil || *started.Parent != message.Ref || started.AuthorAgent == nil || started.AuthorAgent.Model.Id != "requested" || started.Payload["turn"] != float64(0) || started.Ref.Id == "" {
				t.Fatalf("thinking start event = %#v", started)
			}
			if completed.Kind != "thinking.completed" || completed.Parent == nil || *completed.Parent != started.Ref || completed.AuthorAgent == nil || completed.AuthorAgent.Model.Id != "requested" || completed.Ref.Id == "" {
				t.Fatalf("thinking completion event = %#v", completed)
			}
			if reply.Kind != "message.text" || reply.Parent == nil || *reply.Parent != message.Ref || reply.AuthorAgent == nil || reply.AuthorAgent.Model.Id != "requested" || reply.Payload["text"] != "Requested reply." || reply.Ref.Id == "" {
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
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Key: "engineering", Enabled: true}},
		Principals:     []config.Principal{{ID: "alice", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{
			ID: "fallback", Revision: 1, Provider: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Fallback reply."}`, Enabled: true,
		}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Model: "fallback", Priority: 1, MaxTurns: config.DefaultWorkspaceAgentMaxTurns, Enabled: true}},
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
	alice := model.PrincipalRef{Id: "alice"}
	session := model.SessionRef{Workspace: workspace, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "a8607728-3072-4f6c-9bd4-d9f584b7c893"},
		Kind:            "message.text",
		AuthorPrincipal: &alice,
		Payload:         map[string]interface{}{"text": "hello"},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "a8607728-3072-4f6c-9bd4-d9f584b7c894"},
		Parent:          &message.Ref,
		Kind:            "cancel.request",
		AuthorPrincipal: &alice,
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
			if success.Kind != "cancel.success" || success.Parent == nil || *success.Parent != cancellation.Ref || success.AuthorPrincipal == nil || *success.AuthorPrincipal != alice {
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

func workspaceRef(t *testing.T, ctx context.Context, store *database.Store, key string) model.WorkspaceRef {
	t.Helper()
	err, workspace := store.WorkspaceRefGetByKey(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if workspace == nil {
		t.Fatalf("workspace key %q was not found", key)
	}
	return *workspace
}
