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
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Ref != event.Ref || events[0].Kind != event.Kind || events[0].AuthorPrincipal == nil || *events[0].AuthorPrincipal != alice || events[0].AuthorAgent != nil || events[0].AuthorGateway != nil || !reflect.DeepEqual(events[0].Payload, event.Payload) || events[0].CreatedAt == "" {
		t.Fatalf("SessionEventsGet() = %#v", events)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('events', 1, 'kdf', 'key', TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
		VALUES ('provider', 1, 'openai-compatible', 'https://example.test/v1', 'events', 1, 'key', TRUE);
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
