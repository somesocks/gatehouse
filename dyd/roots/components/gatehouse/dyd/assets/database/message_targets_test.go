package database_test

import (
	"context"
	"errors"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestMessageTargetsSnapshotEffectiveParticipantsAndDispatchOnlyBots(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}, {Alias: "outsider", Enabled: true}, {Alias: "disabled", Enabled: false}},
		Groups:          []config.Group{{WorkspaceID: "engineering", Alias: "team", Enabled: true, Members: []config.GroupMember{{PrincipalID: "bob", Enabled: true}, {PrincipalID: "disabled", Enabled: true}}}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Enabled: true}, {WorkspaceID: "engineering", Alias: "second", ModelAlias: "assistant", Revision: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice, bob := principalRef(t, ctx, store, "alice"), principalRef(t, ctx, store, "bob")
	group := groupID(t, ctx, store, "engineering", "team")
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, nil, &group, authz.Member); err != nil {
		t.Fatal(err)
	}
	var bot, second string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'second'`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		err, page, next := store.SessionMessageTargetsSearch(ctx, session, database.SessionParticipantSearch{Kind: "principal", Limit: 1, Cursor: cursor})
		if err != nil || len(page) != 1 || seen[page[0].Ref.Id] {
			t.Fatalf("recipient page = %#v, %v", page, err)
		}
		seen[page[0].Ref.Id] = true
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 2 || !seen[bob.Id] || !seen[alice.Id] {
		t.Fatalf("effective people = %#v", seen)
	}
	forged := "Forged name"
	targets := []model.MessageTarget{
		{Ref: model.MessageTargetRef{Kind: "principal", Id: bob.Id}, Alias: &forged},
		{Ref: model.MessageTargetRef{Kind: "group", Id: group}},
		{Ref: model.MessageTargetRef{Kind: "agent", Id: bot}},
		{Ref: model.MessageTargetRef{Kind: "agent", Id: second}},
		{Ref: model.MessageTargetRef{Kind: "agent", Id: bot}},
	}
	messageID, _ := typed_id.New(typed_id.SessionEvent)
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: messageID}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "Review", "targets": targets}}
	err, stored := store.SessionMessagesCreate(ctx, message)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := database.SessionMessageTargets(stored)
	if err != nil || len(saved) != 4 || saved[0].Alias == nil || *saved[0].Alias != "bob" {
		t.Fatalf("snapshots = %#v, %v", saved, err)
	}
	if _, legacy := stored.Payload["agents"]; legacy {
		t.Fatal("new message retained duplicate agents field")
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("reply tasks = %#v, %v", tasks, err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_principals SET alias = 'renamed' WHERE id = ?`, bob.Id); err != nil {
		t.Fatal(err)
	}
	err, historical := store.SessionEventGet(ctx, stored.Ref)
	if err != nil {
		t.Fatal(err)
	}
	saved, err = database.SessionMessageTargets(*historical)
	if err != nil || *saved[0].Alias != "bob" {
		t.Fatalf("historical labels changed: %#v, %v", saved, err)
	}
	for _, ref := range []model.MessageTargetRef{
		{Kind: "principal", Id: principalRef(t, ctx, store, "outsider").Id},
		{Kind: "principal", Id: principalRef(t, ctx, store, "disabled").Id},
		{Kind: "group", Id: bob.Id},
		{Kind: "unknown", Id: bot},
	} {
		message.Ref.Id, _ = typed_id.New(typed_id.SessionEvent)
		message.Payload = map[string]interface{}{"text": "Invalid", "targets": []model.MessageTarget{{Ref: ref}}}
		if err, _ := store.SessionMessagesCreate(ctx, message); !errors.Is(err, database.ErrMessageTarget) {
			t.Fatalf("invalid target %#v = %v", ref, err)
		}
	}
	// Human/group-only messages create no additional bot tasks.
	message.Ref.Id, _ = typed_id.New(typed_id.SessionEvent)
	message.Payload = map[string]interface{}{"text": "For the team", "targets": targets[:2]}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, tasks = store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("human targets dispatched a bot: %#v, %v", tasks, err)
	}
}
