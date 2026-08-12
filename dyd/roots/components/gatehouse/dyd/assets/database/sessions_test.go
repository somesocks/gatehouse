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
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{ID: "alice", Enabled: true},
			{ID: "bob", Enabled: true},
			{ID: "carol", Enabled: true},
		},
		Groups: []config.Group{{
			WorkspaceID: "engineering",
			ID:          "developers",
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
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES
			('engineering', 'private', 'alice', TRUE, '2026-01-01 00:00:02'),
			('engineering', 'shared', 'alice', TRUE, '2026-01-01 00:00:03'),
			('engineering', 'direct', 'alice', TRUE, '2026-01-01 00:00:01'),
			('engineering', 'disabled-grant', 'alice', TRUE, '2026-01-01 00:00:04'),
			('engineering', 'disabled-session', 'alice', FALSE, '2026-01-01 00:00:05');
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled) VALUES
			('engineering', 'private', 'alice', TRUE),
			('engineering', 'direct', 'carol', TRUE),
			('engineering', 'disabled-grant', 'alice', FALSE),
			('engineering', 'disabled-session', 'alice', TRUE);
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
			VALUES ('engineering', 'shared', 'developers', TRUE);
	`); err != nil {
		t.Fatal(err)
	}

	workspace := model.WorkspaceRef{Id: "engineering"}
	sessionsFor := func(principal string) []model.Session {
		err, sessions := store.SessionsGet(ctx, workspace, model.PrincipalRef{Id: principal})
		if err != nil {
			t.Fatal(err)
		}
		return sessions
	}
	wantAlice := []model.Session{
		{Ref: model.SessionRef{Workspace: workspace, Id: "shared"}, AuthorPrincipal: &model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:03"},
		{Ref: model.SessionRef{Workspace: workspace, Id: "private"}, AuthorPrincipal: &model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:02"},
	}
	if got := sessionsFor("alice"); !reflect.DeepEqual(got, wantAlice) {
		t.Fatalf("SessionsGet() for alice = %#v, want %#v", got, wantAlice)
	}
	wantBob := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "shared"}, AuthorPrincipal: &model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:03",
	}}
	if got := sessionsFor("bob"); !reflect.DeepEqual(got, wantBob) {
		t.Fatalf("SessionsGet() for bob = %#v, want %#v", got, wantBob)
	}
	wantCarol := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "direct"}, AuthorPrincipal: &model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:01",
	}}
	if got := sessionsFor("carol"); !reflect.DeepEqual(got, wantCarol) {
		t.Fatalf("SessionsGet() for carol = %#v, want %#v", got, wantCarol)
	}
	if _, err := store.ExecContext(ctx, `
		UPDATE gatehouse_group_members
		SET enabled = FALSE
		WHERE workspace_id = 'engineering' AND group_id = 'developers' AND principal_id = 'bob'
	`); err != nil {
		t.Fatal(err)
	}
	if got := sessionsFor("bob"); len(got) != 0 {
		t.Fatalf("SessionsGet() for revoked bob = %#v, want no sessions", got)
	}
}

func TestSessionsCreateAndEvents(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{ID: "alice", Enabled: true},
			{ID: "bob", Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "session-one"}
	alice := model.PrincipalRef{Id: "alice"}
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
	err, storedSession = store.SessionGet(ctx, session, model.PrincipalRef{Id: "bob"})
	if err != nil || storedSession != nil {
		t.Fatalf("SessionGet() for ungranted principal = (%#v, %v)", storedSession, err)
	}

	event := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "b8607728-3072-4f6c-9bd4-d9f584b7c892"},
		Kind:            "message.text",
		AuthorPrincipal: &alice,
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
	if len(events) != 2 || events[0].Ref != preciselyTimedEvent.Ref || events[0].CreatedAt != preciselyTimedEvent.CreatedAt || events[1].Ref != event.Ref || events[1].Kind != event.Kind || events[1].AuthorPrincipal == nil || *events[1].AuthorPrincipal != alice || events[1].AuthorAgent != nil || events[1].AuthorGateway != nil || !reflect.DeepEqual(events[1].Payload, event.Payload) || events[1].CreatedAt == "" {
		t.Fatalf("SessionEventsGet() = %#v", events)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES ('engineering', 'session-one', 'b8607728-3072-4f6c-9bd4-d9f584b7c892', '2026-01-01T00:00:00.000Z')
	`); err != nil {
		t.Fatalf("create session event reply task: %v", err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES ('engineering', 'session-one', 'b8607728-3072-4f6c-9bd4-d9f584b7c892', '2026-01-01T00:00:00.000Z')
	`); err == nil {
		t.Fatal("session event reply tasks accepted a duplicate event")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES ('engineering', 'session-one', 'missing-event', '2026-01-01T00:00:00.000Z')
	`); err == nil {
		t.Fatal("session event reply tasks accepted a missing event")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event)
		VALUES ('engineering', 'session-one', 'b8607728-3072-4f6c-9bd4-d9f584b7c892')
	`); err == nil {
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
		VALUES ('engineering', 'assistant', 1, TRUE)
	`); err != nil {
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
		VALUES ('engineering', 'session-one', 'unknown-author', 'message.text', 'unknown', '{}', '2026-01-01T00:00:00.000Z')
	`); err == nil {
		t.Fatal("session events accepted an unknown principal author")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled)
		VALUES ('engineering', 'missing-timestamp', 'alice', TRUE)
	`); err == nil {
		t.Fatal("sessions accepted a missing timestamp")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload)
		VALUES ('engineering', 'session-one', 'missing-timestamp', 'message.text', 'alice', '{}')
	`); err == nil {
		t.Fatal("session events accepted a missing timestamp")
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
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{{ID: "alice", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "session-one"}
	alice := model.PrincipalRef{Id: "alice"}
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
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 1 || tasks[0].Event != message.Ref {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}

	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at) VALUES
			('engineering', 'session-one', '00000000-0000-4000-8000-000000000001', 'message.text', 'alice', '{"text":"first"}', '2026-01-01T00:00:00.000Z'),
			('engineering', 'session-one', '00000000-0000-4000-8000-000000000002', 'message.text', 'alice', '{"text":"second"}', '2026-01-01T00:00:00.000Z'),
			('engineering', 'session-one', '00000000-0000-4000-8000-000000000003', 'message.text', 'alice', '{"text":"third"}', '2026-01-01T00:00:01.000Z')
	`); err != nil {
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
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{ID: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{ID: "first", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"First"}`, Enabled: true},
			{ID: "second", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Second"}`, Enabled: true},
			{ID: "lower", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Lower"}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Model: "first", Priority: 2, MaxTurns: 3, Enabled: true},
			{WorkspaceID: "engineering", Model: "second", Priority: 2, MaxTurns: 3, Enabled: true},
			{WorkspaceID: "engineering", Model: "lower", Priority: 1, MaxTurns: 2, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		err, selected := store.WorkspaceAgentModelSelect(ctx, model.WorkspaceRef{Id: "engineering"})
		if err != nil || selected == nil {
			t.Fatalf("WorkspaceAgentModelSelect() = (%#v, %v)", selected, err)
		}
		if selected.Ref.Model.Id != "first" && selected.Ref.Model.Id != "second" {
			t.Fatalf("WorkspaceAgentModelSelect() selected %#v outside the highest priority tier", selected)
		}
		if selected.MaxTurns != 3 {
			t.Fatalf("WorkspaceAgentModelSelect() max turns = %d, want 3", selected.MaxTurns)
		}
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
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{{ID: "alice", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	alice := model.PrincipalRef{Id: "alice"}
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	create := func(id, kind string, parent *model.SessionEventRef) model.SessionEvent {
		err, event := store.SessionEventsCreate(ctx, model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Parent:          parent,
			Kind:            kind,
			AuthorPrincipal: &alice,
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
		AuthorPrincipal: &alice,
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
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{{ID: "alice", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	alice := model.PrincipalRef{Id: "alice"}
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "session-one"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	root := model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000001"}
	call := model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000002"}
	events := []model.SessionEvent{
		{Ref: root, Kind: "message.text", AuthorPrincipal: &alice, Payload: map[string]interface{}{"text": "hello"}},
		{Ref: call, Parent: &root, Kind: "tool.request", AuthorPrincipal: &alice, Payload: map[string]interface{}{}},
		{Ref: model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000003"}, Parent: &call, Kind: "tool.success", AuthorPrincipal: &alice, Payload: map[string]interface{}{}},
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
		{Ref: model.SessionEventRef{Session: session, Id: "10000000-0000-4000-8000-000000000005"}, Parent: &forward, Kind: "message.text", AuthorPrincipal: &alice, Payload: map[string]interface{}{}},
		{Ref: forward, Kind: "message.text", AuthorPrincipal: &alice, Payload: map[string]interface{}{}},
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
		VALUES ('engineering', 'session-one', '10000000-0000-4000-8000-000000000006', '10000000-0000-4000-8000-000000000006', 'message.text', 'alice', '{}', '2026-01-01T00:00:00.000Z')
	`); err == nil {
		t.Fatal("session events accepted a self parent")
	}
}
