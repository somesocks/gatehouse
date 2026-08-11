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
		Workspaces:     []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals:     []config.Principal{{ID: "alice", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{
			ID: "fallback", Revision: 1, Provider: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Fallback reply."}`, Enabled: true,
		}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Model: "fallback", Priority: 1, Enabled: true}},
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

	alice := model.PrincipalRef{Id: "alice"}
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "a8607728-3072-4f6c-9bd4-d9f584b7c892"},
		Kind:            "message.text",
		AuthorPrincipal: &alice,
		Payload:         map[string]interface{}{"text": "hello"},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}

	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	runtimeErr, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store)
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
		if len(events) == 2 {
			reply := events[1]
			if reply.Kind != "message.text" || reply.Parent == nil || *reply.Parent != message.Ref || reply.AuthorAgent == nil || reply.AuthorAgent.Model.Id != "fallback" || reply.Payload["text"] != "Fallback reply." || reply.Ref.Id == "" {
				t.Fatalf("reply event = %#v", reply)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session events = %#v, want message and reply", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}
}
