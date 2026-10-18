package agent_test

import (
	"context"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func TestBotRemovalCancelsQueuedReplyBeforeExecution(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Should not run"}`, Compaction: defaultAgentModelCompaction, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Enabled: true}},
	}
	err, registry := migrations.Build(configuration, state, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "Hello", "agents": []string{bot}}}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, message.Ref, model.SessionEventKindAgentRequest)
	if err != nil || request == nil {
		t.Fatalf("request = %#v %v", request, err)
	}
	err, roster, _ := store.SessionParticipantsSearch(ctx, session, database.SessionParticipantSearch{Kind: "agent", Limit: 50}, false)
	if err != nil || len(roster) != 1 {
		t.Fatalf("roster = %#v %v", roster, err)
	}
	grant := roster[0].Grants[0]
	disabled := false
	if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, grant.ID, nil, &disabled, grant.Revision); err != nil {
		t.Fatal(err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "bot-removal-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		cancelled := false
		for _, event := range events {
			if event.Kind == model.SessionEventKindAgentSuccess {
				t.Fatal("removed bot produced a reply")
			}
			if event.Kind == model.SessionEventKindCancelSuccess {
				cancelled = true
			}
		}
		if cancelled {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("removed bot's queued request did not reach cancellation")
}
