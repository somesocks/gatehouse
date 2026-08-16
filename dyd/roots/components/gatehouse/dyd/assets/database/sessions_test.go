package database_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestSessionsGetHonorsPrincipalAndGroupGrants(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{Alias: "alice", Enabled: true},
			{Alias: "bob", Enabled: true},
			{Alias: "carol", Enabled: true},
		},
		Groups: []config.Group{{
			WorkspaceID: "engineering",
			Alias:       "developers",
			Enabled:     true,
			Members: []config.GroupMember{
				{PrincipalID: "alice", Enabled: true},
				{PrincipalID: "bob", Enabled: true},
			},
		}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	developersID := groupID(t, ctx, store, "engineering", "developers")
	alice := principalRef(t, ctx, store, "alice")
	bob := principalRef(t, ctx, store, "bob")
	carol := principalRef(t, ctx, store, "carol")
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES
			(?, 'private', ?, TRUE, '2026-01-01 00:00:02'),
			(?, 'shared', ?, TRUE, '2026-01-01 00:00:03'),
			(?, 'direct', ?, TRUE, '2026-01-01 00:00:01'),
			(?, 'disabled-grant', ?, TRUE, '2026-01-01 00:00:04'),
			(?, 'disabled-session', ?, FALSE, '2026-01-01 00:00:05')
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled) VALUES
			(?, 'private', ?, TRUE),
			(?, 'direct', ?, TRUE),
			(?, 'disabled-grant', ?, FALSE),
			(?, 'disabled-session', ?, TRUE)
	`, workspace.Id, alice.Id, workspace.Id, carol.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
			VALUES (?, 'shared', ?, TRUE)
	`, workspace.Id, developersID); err != nil {
		t.Fatal(err)
	}

	sessionsFor := func(principal model.PrincipalRef) []model.Session {
		err, sessions := store.SessionsGet(ctx, workspace, principal)
		if err != nil {
			t.Fatal(err)
		}
		return sessions
	}
	wantAlice := []model.Session{
		{Ref: model.SessionRef{Workspace: workspace, Id: "shared"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:03"},
		{Ref: model.SessionRef{Workspace: workspace, Id: "private"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:02"},
	}
	if got := sessionsFor(alice); !reflect.DeepEqual(got, wantAlice) {
		t.Fatalf("SessionsGet() for alice = %#v, want %#v", got, wantAlice)
	}
	wantBob := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "shared"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:03",
	}}
	if got := sessionsFor(bob); !reflect.DeepEqual(got, wantBob) {
		t.Fatalf("SessionsGet() for bob = %#v, want %#v", got, wantBob)
	}
	wantCarol := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "direct"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:01",
	}}
	if got := sessionsFor(carol); !reflect.DeepEqual(got, wantCarol) {
		t.Fatalf("SessionsGet() for carol = %#v, want %#v", got, wantCarol)
	}
	if _, err := store.ExecContext(ctx, `
		UPDATE gatehouse_group_members
		SET enabled = FALSE
		WHERE workspace_id = ? AND group_id = ? AND principal_id = ?
	`, workspace.Id, developersID, bob.Id); err != nil {
		t.Fatal(err)
	}
	if got := sessionsFor(bob); len(got) != 0 {
		t.Fatalf("SessionsGet() for revoked bob = %#v, want no sessions", got)
	}
}

func TestSessionsCreateAndEvents(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	aliceName := "Alice"
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{Alias: "alice", Name: &aliceName, Enabled: true},
			{Alias: "bob", Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	session := model.SessionRef{Workspace: workspace, Id: "session-one"}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	err, stored := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CreatedAt == "" {
		t.Fatalf("SessionsCreate() = %#v", stored)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", stored.CreatedAt); err != nil {
		t.Fatalf("SessionsCreate() timestamp = %q: %v", stored.CreatedAt, err)
	}
	err, storedSession := store.SessionGet(ctx, session, alice)
	if err != nil || storedSession == nil || storedSession.AuthorPrincipal == nil || *storedSession.AuthorPrincipal != alice || storedSession.AuthorAgent != nil || storedSession.AuthorGateway != nil || !storedSession.Enabled || storedSession.CreatedAt == "" {
		t.Fatalf("SessionGet() for creator = (%#v, %v)", storedSession, err)
	}
	err, storedSession = store.SessionGet(ctx, session, principalRef(t, ctx, store, "bob"))
	if err != nil || storedSession != nil {
		t.Fatalf("SessionGet() for ungranted principal = (%#v, %v)", storedSession, err)
	}

	event := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "b8607728-3072-4f6c-9bd4-d9f584b7c892"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello"},
	}
	err, storedEvent := store.SessionEventsCreate(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	if storedEvent.CreatedAt == "" {
		t.Fatalf("SessionEventsCreate() = %#v", storedEvent)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", storedEvent.CreatedAt); err != nil {
		t.Fatalf("SessionEventsCreate() timestamp = %q: %v", storedEvent.CreatedAt, err)
	}
	preciselyTimedEvent := event
	preciselyTimedEvent.Ref.Id = "c8607728-3072-4f6c-9bd4-d9f584b7c892"
	preciselyTimedEvent.CreatedAt = "2026-01-01T00:00:00.001Z"
	err, preciselyTimedEvent = store.SessionEventsCreate(ctx, preciselyTimedEvent)
	if err != nil || preciselyTimedEvent.CreatedAt != "2026-01-01T00:00:00.001Z" {
		t.Fatalf("SessionEventsCreate() explicit timestamp = (%#v, %v)", preciselyTimedEvent, err)
	}
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ref != preciselyTimedEvent.Ref || events[0].CreatedAt != preciselyTimedEvent.CreatedAt || events[1].Ref != event.Ref || events[1].Kind != event.Kind || events[1].AuthorPrincipal == nil || events[1].AuthorPrincipal.Ref != alice || events[1].AuthorPrincipal.Name == nil || *events[1].AuthorPrincipal.Name != aliceName || events[1].AuthorAgent != nil || events[1].AuthorGateway != nil || !reflect.DeepEqual(events[1].Payload, event.Payload) || events[1].CreatedAt == "" {
		t.Fatalf("SessionEventsGet() = %#v", events)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES (?, 'session-one', 'b8607728-3072-4f6c-9bd4-d9f584b7c892', '2026-01-01T00:00:00.000Z')
	`, workspace.Id); err != nil {
		t.Fatalf("create session event reply task: %v", err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES (?, 'session-one', 'b8607728-3072-4f6c-9bd4-d9f584b7c892', '2026-01-01T00:00:00.000Z')
	`, workspace.Id); err == nil {
		t.Fatal("session event reply tasks accepted a duplicate event")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES (?, 'session-one', 'missing-event', '2026-01-01T00:00:00.000Z')
	`, workspace.Id); err == nil {
		t.Fatal("session event reply tasks accepted a missing event")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event)
		VALUES (?, 'session-one', 'b8607728-3072-4f6c-9bd4-d9f584b7c892')
	`, workspace.Id); err == nil {
		t.Fatal("session event reply tasks accepted a missing timestamp")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('events', 1, 'kdf', 'key', TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
		VALUES ('provider', 1, 'openai-chat-completions', 'https://example.test/v1', 'events', 1, 'key', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled)
		VALUES ('assistant', 1, 'provider', 'example', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, priority, enabled)
		VALUES (?, 'assistant', 1, TRUE)
	`, workspace.Id); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "assistant"}}
	agentSession := model.Session{Ref: model.SessionRef{Workspace: session.Workspace, Id: "agent-session"}, AuthorAgent: &agent, Enabled: true}
	err, _ = store.SessionsCreate(ctx, agentSession, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, storedSession = store.SessionGet(ctx, agentSession.Ref, alice)
	if err != nil || storedSession == nil || storedSession.AuthorAgent == nil || *storedSession.AuthorAgent != agent || storedSession.AuthorPrincipal != nil || storedSession.AuthorGateway != nil {
		t.Fatalf("agent-authored session = (%#v, %v)", storedSession, err)
	}
	gateway := model.GatewayRef{Id: "8d485e76-6138-47e1-a9fa-1f2fb22062bd"}
	gatewaySession := model.Session{Ref: model.SessionRef{Workspace: session.Workspace, Id: "gateway-session"}, AuthorGateway: &gateway, Enabled: true}
	err, _ = store.SessionsCreate(ctx, gatewaySession, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, storedSession = store.SessionGet(ctx, gatewaySession.Ref, alice)
	if err != nil || storedSession == nil || storedSession.AuthorGateway == nil || *storedSession.AuthorGateway != gateway || storedSession.AuthorPrincipal != nil || storedSession.AuthorAgent != nil {
		t.Fatalf("gateway-authored session = (%#v, %v)", storedSession, err)
	}
	agentEvent := model.SessionEvent{
		Ref:  model.SessionEventRef{Session: session, Id: "c2af111c-fd60-439d-8f1e-07f6255b9092"},
		Kind: "message.text",
		AuthorAgent: &model.WorkspaceAgentRef{
			Workspace: session.Workspace,
			Model:     model.AgentModelRef{Id: "assistant"},
		},
		Payload: map[string]interface{}{"text": "hello from the agent"},
	}
	err, _ = store.SessionEventsCreate(ctx, agentEvent)
	if err != nil {
		t.Fatal(err)
	}
	gatewayEvent := model.SessionEvent{
		Ref:           model.SessionEventRef{Session: session, Id: "d4de1eaa-26d0-48ed-8d16-511a5c5b3fc0"},
		Kind:          "gateway.notice",
		AuthorGateway: &model.GatewayRef{Id: "8d485e76-6138-47e1-a9fa-1f2fb22062bd"},
		Payload:       map[string]interface{}{"text": "gateway notice"},
	}
	err, _ = store.SessionEventsCreate(ctx, gatewayEvent)
	if err != nil {
		t.Fatal(err)
	}
	err, events = store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]model.SessionEvent, len(events))
	for _, value := range events {
		byID[value.Ref.Id] = value
	}
	if agent := byID[agentEvent.Ref.Id]; agent.AuthorAgent == nil || *agent.AuthorAgent != *agentEvent.AuthorAgent || agent.AuthorPrincipal != nil || agent.AuthorGateway != nil {
		t.Fatalf("agent session event = %#v", agent)
	}
	if gateway := byID[gatewayEvent.Ref.Id]; gateway.AuthorGateway == nil || *gateway.AuthorGateway != *gatewayEvent.AuthorGateway || gateway.AuthorPrincipal != nil || gateway.AuthorAgent != nil {
		t.Fatalf("gateway session event = %#v", gateway)
	}

	err, _ = store.SessionEventsCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "c2af111c-fd60-439d-8f1e-07f6255b9092"}, Kind: "message.text", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("SessionEventsCreate() accepted an event without an author")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at)
		VALUES (?, 'session-one', 'unknown-author', 'message.text', ?, '{}', '2026-01-01T00:00:00.000Z')
	`, workspace.Id, newTypedID(t, "prn")); err == nil {
		t.Fatal("session events accepted an unknown principal author")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled)
		VALUES (?, 'missing-timestamp', ?, TRUE)
	`, workspace.Id, alice.Id); err == nil {
		t.Fatal("sessions accepted a missing timestamp")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload)
		VALUES (?, 'session-one', 'missing-timestamp', 'message.text', ?, '{}')
	`, workspace.Id, alice.Id); err == nil {
		t.Fatal("session events accepted a missing timestamp")
	}
}

func TestActivityTopicCheckpointsGetHonorsAuthorizationAndAdvancesIndependently(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{Alias: "alice", Enabled: true},
			{Alias: "bob", Enabled: true},
			{Alias: "carol", Enabled: true},
		},
		Groups: []config.Group{{
			WorkspaceID: "engineering",
			Alias:       "developers",
			Enabled:     true,
			Members:     []config.GroupMember{{PrincipalID: "bob", Enabled: true}},
		}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	developersID := groupID(t, ctx, store, "engineering", "developers")
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	bob := principalRef(t, ctx, store, "bob")
	carol := principalRef(t, ctx, store, "carol")
	shared := model.SessionRef{Workspace: workspace, Id: "shared"}
	private := model.SessionRef{Workspace: workspace, Id: "private"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: shared, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
		VALUES (?, 'shared', ?, TRUE)
	`, workspace.Id, developersID); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: private, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	sharedEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: shared, Id: "00000000-0000-4000-8000-000000000001"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, sharedEvent); err != nil {
		t.Fatal(err)
	}
	privateEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: private, Id: "00000000-0000-4000-8000-000000000002"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, privateEvent); err != nil {
		t.Fatal(err)
	}

	check := func(principal model.PrincipalRef, checkpoints []database.ActivityTopicCheckpoint) []database.ActivityTopicCheckpoint {
		err, advanced := store.ActivityTopicCheckpointsGet(ctx, workspace, principal, checkpoints)
		if err != nil {
			t.Fatal(err)
		}
		return advanced
	}
	initial := check(bob, []database.ActivityTopicCheckpoint{
		{Topic: database.ActivityTopicSessions},
		{Topic: database.ActivityTopicSession(shared)},
	})
	if len(initial) != 2 || initial[0].CreatedAt == "" || initial[0].ID == "" || initial[1].CreatedAt == "" || initial[1].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() = %#v, want advanced checkpoints", initial)
	}
	if initial[0] == initial[1] {
		t.Fatalf("ActivityTopicCheckpointsGet() = %#v, topics should advance independently", initial)
	}
	if repeated := check(bob, initial); !reflect.DeepEqual(repeated, initial) {
		t.Fatalf("ActivityTopicCheckpointsGet() with current checkpoints = %#v, want %#v", repeated, initial)
	}
	if checkpoints := check(bob, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(private)}}); checkpoints[0].CreatedAt != "" || checkpoints[0].ID != "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for private session = %#v, want no cursor", checkpoints)
	}
	if checkpoints := check(carol, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSessions}, {Topic: database.ActivityTopicSession(shared)}}); checkpoints[0].CreatedAt != "" || checkpoints[1].CreatedAt != "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for ungranted principal = %#v, want no cursors", checkpoints)
	}
	newSharedEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: shared, Id: "00000000-0000-4000-8000-000000000003"}, Kind: "tool.success", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, newSharedEvent); err != nil {
		t.Fatal(err)
	}
	advanced := check(bob, initial)
	if advanced[0] != initial[0] || advanced[1].CreatedAt == initial[1].CreatedAt && advanced[1].ID == initial[1].ID {
		t.Fatalf("ActivityTopicCheckpointsGet() after session event = %#v, want only session topic to advance from %#v", advanced, initial)
	}

	var activityID, createdAt string
	if err := store.QueryRowContext(ctx, `
		SELECT id, created_at FROM gatehouse_activity_events
		WHERE workspace = ? AND resource_kind = 'session'
		LIMIT 1
	`, workspace.Id).Scan(&activityID, &createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
		VALUES (?, ?, 'invalid-timestamp', '2000-01-01T00:00:00.000Z')
	`, workspace.Id, activityID); err == nil {
		t.Fatal("activity topic accepted a timestamp that does not match its parent event")
	}
	if createdAt == "" {
		t.Fatal("activity event did not retain its creation timestamp")
	}
}

func TestSessionMessagesCreateAddsReplyTaskAndEventsPageUsesKeyset(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	session := model.SessionRef{Workspace: workspace, Id: "session-one"}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "a8607728-3072-4f6c-9bd4-d9f584b7c892"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello"},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 1 || tasks[0].Event != message.Ref {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}

	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at) VALUES
			(?, 'session-one', '00000000-0000-4000-8000-000000000001', 'message.text', ?, '{"text":"first"}', '2026-01-01T00:00:00.000Z'),
			(?, 'session-one', '00000000-0000-4000-8000-000000000002', 'message.text', ?, '{"text":"second"}', '2026-01-01T00:00:00.000Z'),
			(?, 'session-one', '00000000-0000-4000-8000-000000000003', 'message.text', ?, '{"text":"third"}', '2026-01-01T00:00:01.000Z')
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	err, events := store.SessionEventsPageGet(ctx, session, "2026-01-01T00:00:00.000Z", "00000000-0000-4000-8000-000000000001", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ref.Id != "00000000-0000-4000-8000-000000000002" || events[1].Ref.Id != "00000000-0000-4000-8000-000000000003" {
		t.Fatalf("SessionEventsPageGet() = %#v", events)
	}
}

func TestWorkspaceAgentModelSelectUsesOnlyHighestPriorityTier(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	firstPrompt := "First prompt."
	emptyPrompt := ""
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{ID: "first", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"First"}`, Enabled: true},
			{ID: "second", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Second"}`, Enabled: true},
			{ID: "lower", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Lower"}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Model: "first", Priority: 2, MaxTurns: 3, SystemPrompt: &firstPrompt, Enabled: true},
			{WorkspaceID: "engineering", Model: "second", Priority: 2, MaxTurns: 3, SystemPrompt: &emptyPrompt, Enabled: true},
			{WorkspaceID: "engineering", Model: "lower", Priority: 1, MaxTurns: 2, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	for range 5 {
		err, selected := store.WorkspaceAgentModelSelect(ctx, workspace, "")
		if err != nil || selected == nil {
			t.Fatalf("WorkspaceAgentModelSelect() = (%#v, %v)", selected, err)
		}
		if selected.Ref.Model.Id != "first" && selected.Ref.Model.Id != "second" {
			t.Fatalf("WorkspaceAgentModelSelect() selected %#v outside the highest priority tier", selected)
		}
		if selected.MaxTurns != 3 {
			t.Fatalf("WorkspaceAgentModelSelect() max turns = %d, want 3", selected.MaxTurns)
		}
		if selected.Ref.Model.Id == "first" && (selected.SystemPrompt == nil || *selected.SystemPrompt != "First prompt.") {
			t.Fatalf("WorkspaceAgentModelSelect() system prompt = %#v, want first prompt", selected.SystemPrompt)
		}
		if selected.Ref.Model.Id == "second" && (selected.SystemPrompt == nil || *selected.SystemPrompt != "") {
			t.Fatalf("WorkspaceAgentModelSelect() system prompt = %#v, want empty prompt", selected.SystemPrompt)
		}
	}
}

func TestWorkspaceAgentModelSelectPrefersEligibleRequestedAgent(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{ID: "automatic", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Automatic"}`, Enabled: true},
			{ID: "requested", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Requested"}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Model: "automatic", Priority: 2, MaxTurns: 1, Enabled: true},
			{WorkspaceID: "engineering", Model: "requested", Priority: 1, MaxTurns: 1, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	err, selected := store.WorkspaceAgentModelSelect(ctx, workspace, "requested")
	if err != nil || selected == nil || selected.Ref.Model.Id != "requested" {
		t.Fatalf("WorkspaceAgentModelSelect() = (%#v, %v), want requested agent", selected, err)
	}
	err, selected = store.WorkspaceAgentModelSelect(ctx, workspace, "missing")
	if err != nil || selected == nil || selected.Ref.Model.Id != "automatic" {
		t.Fatalf("WorkspaceAgentModelSelect() = (%#v, %v), want automatic agent", selected, err)
	}
}

func TestSessionEventsPersistParentsAndLoadTreePages(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	workspace := workspaceRef(t, ctx, store, "engineering")
	session := model.SessionRef{Workspace: workspace, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	create := func(id, kind string, parent *model.SessionEventRef) model.SessionEvent {
		err, event := store.SessionEventsCreate(ctx, model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Parent:          parent,
			Kind:            kind,
			AuthorPrincipal: &alicePrincipal,
			Payload:         map[string]interface{}{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	root := create("00000000-0000-4000-8000-000000000001", "message.text", nil)
	reasoning := create("00000000-0000-4000-8000-000000000002", "message.reasoning", &root.Ref)
	call := create("00000000-0000-4000-8000-000000000003", "tool.request", &root.Ref)
	result := create("00000000-0000-4000-8000-000000000004", "tool.success", &call.Ref)
	text := create("00000000-0000-4000-8000-000000000005", "message.text", &root.Ref)

	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.SessionEvent{}
	for _, event := range events {
		byID[event.Ref.Id] = event
	}
	for _, expected := range []struct {
		event  model.SessionEvent
		parent model.SessionEventRef
	}{
		{reasoning, root.Ref},
		{call, root.Ref},
		{result, call.Ref},
		{text, root.Ref},
	} {
		stored := byID[expected.event.Ref.Id]
		if stored.Parent == nil || *stored.Parent != expected.parent {
			t.Fatalf("event %#v parent = %#v, want %#v", stored.Ref, stored.Parent, expected.parent)
		}
	}

	err, entries := store.SessionEventsTreePageGet(ctx, session, "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("SessionEventsTreePageGet() = %#v", entries)
	}
	for index, expected := range []struct {
		id    string
		depth int
	}{
		{root.Ref.Id, 0},
		{reasoning.Ref.Id, 1},
		{call.Ref.Id, 1},
		{result.Ref.Id, 2},
		{text.Ref.Id, 1},
	} {
		if entries[index].Event.Ref.Id != expected.id || entries[index].Depth != expected.depth {
			t.Fatalf("tree entry %d = %#v, want ID %q at depth %d", index, entries[index], expected.id, expected.depth)
		}
	}

	otherSession := model.SessionRef{Workspace: session.Workspace, Id: "session-two"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: otherSession, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	err, _ = store.SessionEventsCreate(ctx, model.SessionEvent{
		Ref:             model.SessionEventRef{Session: otherSession, Id: "00000000-0000-4000-8000-000000000006"},
		Parent:          &root.Ref,
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("SessionEventsCreate() accepted a parent from another session")
	}
	for _, index := range []string{
		"gatehouse_session_events_by_session_order",
		"gatehouse_session_events_roots",
		"gatehouse_session_events_children",
	} {
		var count int
		if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("session event index %q count = %d", index, count)
		}
	}
}

func TestSessionEventsCreateBatchRequiresExistingOrEarlierParents(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	workspace := workspaceRef(t, ctx, store, "engineering")
	session := model.SessionRef{Workspace: workspace, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	root := model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000001"}
	call := model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000002"}
	events := []model.SessionEvent{
		{Ref: root, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "hello"}},
		{Ref: call, Parent: &root, Kind: "tool.request", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
		{Ref: model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000003"}, Parent: &call, Kind: "tool.success", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
	}
	err, stored := store.SessionEventsCreateBatch(ctx, events)
	if err != nil || len(stored) != len(events) {
		t.Fatalf("SessionEventsCreateBatch() = (%#v, %v)", stored, err)
	}
	for index := range events {
		if stored[index].Ref != events[index].Ref || stored[index].CreatedAt == "" {
			t.Fatalf("stored batch event %d = %#v", index, stored[index])
		}
	}

	forward := model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000004"}
	err, _ = store.SessionEventsCreateBatch(ctx, []model.SessionEvent{
		{Ref: model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000005"}, Parent: &forward, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
		{Ref: forward, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
	})
	if err == nil {
		t.Fatal("SessionEventsCreateBatch() accepted a forward parent")
	}
	err, all := store.SessionEventsGet(ctx, session)
	if err != nil || len(all) != len(events) {
		t.Fatalf("events after rejected batch = (%#v, %v)", all, err)
	}

	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, parent, kind, author_principal, payload, created_at)
		VALUES (?, 'session-one', '10000000-0000-4000-8000-000000000006', '10000000-0000-4000-8000-000000000006', 'message.text', ?, '{}', '2026-01-01T00:00:00.000Z')
	`, workspace.Id, alice.Id); err == nil {
		t.Fatal("session events accepted a self parent")
	}
}
