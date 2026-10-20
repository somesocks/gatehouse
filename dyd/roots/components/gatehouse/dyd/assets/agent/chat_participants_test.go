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
	testBotRemovalCancelsQueuedReply(t, false)
}

func TestGroupDerivedBotRemovalCancelsQueuedReplyBeforeExecution(t *testing.T) {
	testBotRemovalCancelsQueuedReply(t, true)
}

func TestWorkspaceAdmissionRemovalCancelsQueuedReplyBeforeExecution(t *testing.T) {
	testBotRemovalCancelsQueuedReply(t, true, true)
}

func testBotRemovalCancelsQueuedReply(t *testing.T, inherited bool, removeAuthor ...bool) {
	t.Helper()
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
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	if inherited {
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
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	var group string
	if inherited {
		if err := store.QueryRow(`SELECT id FROM gatehouse_groups WHERE workspace_id = ? AND alias = 'team'`, workspace.Id).Scan(&group); err != nil {
			t.Fatal(err)
		}
		if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, nil, &group, "contributor"); err != nil {
			t.Fatal(err)
		}
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "Hello", "agents": []string{bot}}}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, message.Ref, model.SessionEventKindAgentRequest)
	if err != nil || request == nil {
		t.Fatalf("request = %#v %v", request, err)
	}
	err, roster, _ := store.SessionParticipantsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "agent", Limit: 50}, false)
	if err != nil || len(roster) != 1 {
		t.Fatalf("roster = %#v %v", roster, err)
	}
	disabled := false
	if len(removeAuthor) > 0 && removeAuthor[0] {
		if err, removed := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, alice.Id); err != nil || !removed {
			t.Fatalf("revoke author admission: %t %v", removed, err)
		}
		if err, active := store.SessionAgentParticipantGet(ctx, session, model.WorkspaceAgentRef{Workspace: workspace, Id: bot}); err != nil || !active {
			t.Fatalf("author revocation changed bot participation: %t %v", active, err)
		}
	} else if inherited {
		if len(roster[0].Grants) != 0 || len(roster[0].InheritedFrom) != 1 || roster[0].InheritedFrom[0].Group != group {
			t.Fatalf("group-derived bot was copied to a direct grant: %#v", roster[0])
		}
		if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, false, 1); err != nil {
			t.Fatal(err)
		}
	} else {
		grant := roster[0].Grants[0]
		if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, grant.ID, nil, &disabled, grant.Revision); err != nil {
			t.Fatal(err)
		}
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

func TestGroupDerivedBotExecutesForChatOutsider(t *testing.T) {
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
	bobAlias := "bob"
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}, {Alias: bobAlias, Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Group reply"}`, Compaction: defaultAgentModelCompaction, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Enabled: true}},
		Groups: []config.Group{
			{WorkspaceID: "engineering", Alias: "team", Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}}, Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 1}}},
			{WorkspaceID: "engineering", Alias: "contacts", Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}, {PrincipalID: bobAlias, Enabled: true}}},
		},
		WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "engineering", PrincipalID: &bobAlias, Role: "member", Revision: 1, Enabled: true}},
	}
	err, registry := migrations.Build(configuration, admittedAgentTestState(state), ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice, bob := principalRef(t, ctx, store, "alice"), principalRef(t, ctx, store, bobAlias)
	chat := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: chat, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	var group, bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_groups WHERE workspace_id = ? AND alias = 'team'`, workspace.Id).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = ?`, workspace.Id).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, chat, alice, nil, &group, "contributor"); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, chat, alice, &bob.Id, nil, "contributor"); err != nil {
		t.Fatal(err)
	}
	if err, directory := store.WorkspaceAgentsGet(ctx, workspace, bob); err != nil || len(directory) != 0 {
		t.Fatalf("outsider discovered a group-only bot: %#v %v", directory, err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: chat, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: bob, Enabled: true}, Payload: map[string]interface{}{"text": "Hello", "agents": []string{bot}}}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "group-derived-bot-test", SQLiteSystemDB: store.DB})
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
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		err, events := store.SessionEventsGet(ctx, chat)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind == model.SessionEventKindCancelRequest {
				t.Fatal("group-derived bot request was incorrectly detached")
			}
			if event.Kind == model.SessionEventKindAgentSuccess {
				if event.AuthorAgent == nil || event.AuthorAgent.Id != bot {
					t.Fatalf("wrong responding bot: %#v", event)
				}
				var count int
				if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_grants WHERE workspace = ? AND session = ? AND agent IS NOT NULL`, workspace.Id, chat.Id).Scan(&count); err != nil || count != 0 {
					t.Fatalf("group execution materialized direct bot grants: %d %v", count, err)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("group-derived bot did not reply to a chat outsider")
}
