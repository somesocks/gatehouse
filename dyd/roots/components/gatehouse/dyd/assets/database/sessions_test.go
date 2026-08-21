package database_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"

	"github.com/oklog/ulid/v2"
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
			(?, 'ses_00000000000000000000000000', ?, TRUE, '2026-01-01 00:00:02'),
			(?, 'ses_00000000000000000000000001', ?, TRUE, '2026-01-01 00:00:03'),
			(?, 'ses_00000000000000000000000002', ?, TRUE, '2026-01-01 00:00:01'),
			(?, 'ses_00000000000000000000000003', ?, TRUE, '2026-01-01 00:00:04'),
			(?, 'ses_00000000000000000000000004', ?, FALSE, '2026-01-01 00:00:05')
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled) VALUES
			(?, 'ses_00000000000000000000000000', ?, TRUE),
			(?, 'ses_00000000000000000000000002', ?, TRUE),
			(?, 'ses_00000000000000000000000003', ?, FALSE),
			(?, 'ses_00000000000000000000000004', ?, TRUE)
	`, workspace.Id, alice.Id, workspace.Id, carol.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
			VALUES (?, 'ses_00000000000000000000000001', ?, TRUE)
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
		{Ref: model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000001"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:03"},
		{Ref: model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:02"},
	}
	if got := sessionsFor(alice); !reflect.DeepEqual(got, wantAlice) {
		t.Fatalf("SessionsGet() for alice = %#v, want %#v", got, wantAlice)
	}
	wantBob := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000001"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:03",
	}}
	if got := sessionsFor(bob); !reflect.DeepEqual(got, wantBob) {
		t.Fatalf("SessionsGet() for bob = %#v, want %#v", got, wantBob)
	}
	wantCarol := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000002"}, AuthorPrincipal: &alice, Enabled: true, CreatedAt: "2026-01-01 00:00:01",
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

func TestSessionsSearchMatchesNamesAndPaginatesByID(t *testing.T) {
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
	alice := principalRef(t, ctx, store, "alice")
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, name, author_principal, enabled, created_at) VALUES
			(?, 'ses_00000000000000000000000000', 'Weekly report', ?, TRUE, '2026-01-01 00:00:00'),
			(?, 'ses_00000000000000000000000001', 'Incident report', ?, TRUE, '2026-01-01 00:00:01'),
			(?, 'ses_00000000000000000000000002', 'Planning', ?, TRUE, '2026-01-01 00:00:02')
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled) VALUES
			(?, 'ses_00000000000000000000000000', ?, TRUE),
			(?, 'ses_00000000000000000000000001', ?, TRUE),
			(?, 'ses_00000000000000000000000002', ?, TRUE)
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	err, first, cursor := store.SessionsSearch(ctx, workspace, alice, database.SessionSearch{Name: "REPORT", Limit: 1})
	if err != nil || len(first) != 1 || first[0].Ref.Id != "ses_00000000000000000000000001" || cursor != first[0].Ref.Id {
		t.Fatalf("SessionsSearch() first page = (%#v, %q, %v)", first, cursor, err)
	}
	err, second, cursor := store.SessionsSearch(ctx, workspace, alice, database.SessionSearch{Name: "report", Cursor: cursor, Limit: 1})
	if err != nil || len(second) != 1 || second[0].Ref.Id != "ses_00000000000000000000000000" || cursor != "" {
		t.Fatalf("SessionsSearch() second page = (%#v, %q, %v)", second, cursor, err)
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
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	err, stored := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CreatedAt != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("SessionsCreate() timestamp = %q, want ID timestamp", stored.CreatedAt)
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
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello"},
	}
	err, storedEvent := store.SessionEventsCreate(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	if storedEvent.CreatedAt != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("SessionEventsCreate() timestamp = %q, want ID timestamp", storedEvent.CreatedAt)
	}
	preciselyTimedEvent := event
	preciselyTimedEvent.Ref.Id = "sev_00000000000000000000000002"
	preciselyTimedEvent.CreatedAt = "2026-01-01T00:00:00.001Z"
	err, preciselyTimedEvent = store.SessionEventsCreate(ctx, preciselyTimedEvent)
	if err != nil || preciselyTimedEvent.CreatedAt != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("SessionEventsCreate() timestamp = (%#v, %v), want ID timestamp", preciselyTimedEvent, err)
	}
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ref != event.Ref || events[0].Kind != event.Kind || events[0].AuthorPrincipal == nil || events[0].AuthorPrincipal.Ref != alice || events[0].AuthorPrincipal.Name == nil || *events[0].AuthorPrincipal.Name != aliceName || events[0].AuthorAgent != nil || events[0].AuthorGateway != nil || !reflect.DeepEqual(events[0].Payload, event.Payload) || events[0].CreatedAt == "" || events[1].Ref != preciselyTimedEvent.Ref || events[1].CreatedAt != preciselyTimedEvent.CreatedAt {
		t.Fatalf("SessionEventsGet() = %#v", events)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000001', '2026-01-01T00:00:00.000Z')
	`, workspace.Id); err != nil {
		t.Fatalf("create session event reply task: %v", err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000001', '2026-01-01T00:00:00.000Z')
	`, workspace.Id); err == nil {
		t.Fatal("session event reply tasks accepted a duplicate event")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000007', '2026-01-01T00:00:00.000Z')
	`, workspace.Id); err == nil {
		t.Fatal("session event reply tasks accepted a missing event")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event)
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000001')
	`, workspace.Id); err == nil {
		t.Fatal("session event reply tasks accepted a missing timestamp")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('events', 1, 'kdf', 'key', TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
		VALUES ('apr_01arz3ndektsv4rrffq69g5fav', 1, 'openai-chat-completions', 'https://example.test/v1', 'events', 1, 'key', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled)
		VALUES ('amd_01arz3ndektsv4rrffq69g5fav', 1, 'apr_01arz3ndektsv4rrffq69g5fav', 'example', '{}', TRUE);
		INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, priority, enabled)
		VALUES (?, 'amd_01arz3ndektsv4rrffq69g5fav', 1, TRUE)
	`, workspace.Id); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "amd_01arz3ndektsv4rrffq69g5fav"}}
	agentSession := model.Session{Ref: model.SessionRef{Workspace: session.Workspace, Id: "ses_00000000000000000000000001"}, AuthorAgent: &agent, Enabled: true}
	err, _ = store.SessionsCreate(ctx, agentSession, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, storedSession = store.SessionGet(ctx, agentSession.Ref, alice)
	if err != nil || storedSession == nil || storedSession.AuthorAgent == nil || *storedSession.AuthorAgent != agent || storedSession.AuthorPrincipal != nil || storedSession.AuthorGateway != nil {
		t.Fatalf("agent-authored session = (%#v, %v)", storedSession, err)
	}
	gateway := model.GatewayRef{Id: newTypedID(t, "gwy")}
	gatewaySession := model.Session{Ref: model.SessionRef{Workspace: session.Workspace, Id: "ses_00000000000000000000000002"}, AuthorGateway: &gateway, Enabled: true}
	err, _ = store.SessionsCreate(ctx, gatewaySession, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, storedSession = store.SessionGet(ctx, gatewaySession.Ref, alice)
	if err != nil || storedSession == nil || storedSession.AuthorGateway == nil || *storedSession.AuthorGateway != gateway || storedSession.AuthorPrincipal != nil || storedSession.AuthorAgent != nil {
		t.Fatalf("gateway-authored session = (%#v, %v)", storedSession, err)
	}
	agentEvent := model.SessionEvent{
		Ref:  model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"},
		Kind: "message.text",
		AuthorAgent: &model.WorkspaceAgentRef{
			Workspace: session.Workspace,
			Model:     model.AgentModelRef{Id: "amd_01arz3ndektsv4rrffq69g5fav"},
		},
		Payload: map[string]interface{}{"text": "hello from the agent"},
	}
	err, _ = store.SessionEventsCreate(ctx, agentEvent)
	if err != nil {
		t.Fatal(err)
	}
	gatewayEvent := model.SessionEvent{
		Ref:           model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000004"},
		Kind:          "gateway.notice",
		AuthorGateway: &gateway,
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

	err, _ = store.SessionEventsCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"}, Kind: "message.text", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("SessionEventsCreate() accepted an event without an author")
	}
	invalidGateway := model.GatewayRef{Id: "invalid"}
	err, _ = store.SessionsCreate(ctx, model.Session{Ref: model.SessionRef{Workspace: session.Workspace, Id: newTypedID(t, "ses")}, AuthorGateway: &invalidGateway, Enabled: true}, alice)
	if err == nil {
		t.Fatal("SessionsCreate() accepted an invalid gateway author")
	}
	err, _ = store.SessionEventsCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: newTypedID(t, "sev")}, Kind: "gateway.notice", AuthorGateway: &invalidGateway, Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("SessionEventsCreate() accepted an invalid gateway author")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at)
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000005', 'message.text', ?, '{}', '2026-01-01T00:00:00.000Z')
	`, workspace.Id, newTypedID(t, "prn")); err == nil {
		t.Fatal("session events accepted an unknown principal author")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled)
		VALUES (?, 'ses_00000000000000000000000005', ?, TRUE)
	`, workspace.Id, alice.Id); err == nil {
		t.Fatal("sessions accepted a missing timestamp")
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload)
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000006', 'message.text', ?, '{}')
	`, workspace.Id, alice.Id); err == nil {
		t.Fatal("session events accepted a missing timestamp")
	}
}

func TestSessionNotesUseSessionAuthorizationAndActivity(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	bob := principalRef(t, ctx, store, "bob")
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	create := func(at time.Time, title string) model.SessionNote {
		id, err := typed_id.NewAt(typed_id.SessionNote, at)
		if err != nil {
			t.Fatal(err)
		}
		err, stored := store.SessionNoteCreate(ctx, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: id}, Title: "  " + title + "  ", Description: "  Description for " + title + ".  ", Body: "# " + title}, alice)
		if err != nil || stored.AuthorPrincipal != alice || stored.Title != title || stored.Description != "Description for "+title+"." || stored.Sensitive || stored.CreatedAt != at.Format("2006-01-02T15:04:05.000Z") {
			t.Fatalf("SessionNoteCreate() = (%#v, %v)", stored, err)
		}
		return stored
	}
	older := create(time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC), "Guide")
	newer := create(time.Date(2026, 1, 2, 3, 4, 6, 678_000_000, time.UTC), "Architecture")
	sensitiveID, err := typed_id.NewAt(typed_id.SessionNote, time.Date(2026, 1, 2, 3, 4, 6, 679_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, sensitive := store.SessionNoteCreate(ctx, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: sensitiveID}, Title: "Credentials", Body: "secret", Sensitive: true}, alice)
	if err != nil || !sensitive.Sensitive {
		t.Fatalf("SessionNoteCreate() sensitive = (%#v, %v)", sensitive, err)
	}
	err, notes := store.SessionNotesGet(ctx, session, alice)
	if err != nil || len(notes) != 3 || notes[0].Ref.Id != sensitive.Ref.Id || !notes[0].Sensitive || notes[1].Ref.Id != newer.Ref.Id || notes[1].Title != "Architecture" || notes[2].Ref.Id != older.Ref.Id {
		t.Fatalf("SessionNotesGet() = (%#v, %v)", notes, err)
	}
	err, sensitiveDetail := store.SessionNoteGet(ctx, sensitive.Ref, alice)
	if err != nil || sensitiveDetail == nil || !sensitiveDetail.Note.Sensitive {
		t.Fatalf("SessionNoteGet() sensitive = (%#v, %v)", sensitiveDetail, err)
	}
	err, denied := store.SessionNotesGet(ctx, session, bob)
	if err != nil || len(denied) != 0 {
		t.Fatalf("SessionNotesGet() for ungranted principal = (%#v, %v)", denied, err)
	}
	err, hidden := store.SessionNoteGet(ctx, newer.Ref, bob)
	if err != nil || hidden != nil {
		t.Fatalf("SessionNoteGet() for ungranted principal = (%#v, %v)", hidden, err)
	}
	if createErr, _ := store.SessionNoteCreate(ctx, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: newer.Ref.Id}, Title: "Denied", Description: "Denied note", Body: "Denied"}, bob); createErr == nil {
		t.Fatal("SessionNoteCreate() accepted an ungranted principal")
	}
	title, description, body := "Updated guide", "Updated description", "# Updated guide"
	err, updated := store.SessionNoteDetailsSet(ctx, older.Ref, alice, &title, &description, &body)
	if err != nil || updated == nil || updated.Note.AuthorPrincipal != alice || updated.Note.Title != title || updated.Note.Description != description || updated.Note.Body != body {
		t.Fatalf("SessionNoteDetailsSet() = (%#v, %v)", updated, err)
	}
	err, removed := store.SessionNoteRemove(ctx, newer.Ref, bob)
	if err != nil || removed {
		t.Fatalf("SessionNoteRemove() for ungranted principal = (%t, %v)", removed, err)
	}
	err, removed = store.SessionNoteRemove(ctx, newer.Ref, alice)
	if err != nil || !removed {
		t.Fatalf("SessionNoteRemove() = (%t, %v)", removed, err)
	}
	err, hidden = store.SessionNoteGet(ctx, newer.Ref, alice)
	if err != nil || hidden != nil {
		t.Fatalf("SessionNoteGet() after removal = (%#v, %v)", hidden, err)
	}
	err, notes = store.SessionNotesGet(ctx, session, alice)
	if err != nil || len(notes) != 2 || notes[0].Ref.Id != sensitive.Ref.Id || !notes[0].Sensitive || notes[1].Ref.Id != older.Ref.Id {
		t.Fatalf("SessionNotesGet() after removal = (%#v, %v)", notes, err)
	}
	oversizedID, err := typed_id.NewAt(typed_id.SessionNote, time.Date(2026, 1, 2, 3, 4, 7, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if createErr, _ := store.SessionNoteCreate(ctx, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: oversizedID}, Title: "Oversized", Description: "Oversized body", Body: strings.Repeat("x", 1024*1024+1)}, alice); createErr == nil {
		t.Fatal("SessionNoteCreate() accepted a body exceeding 1 MiB")
	}
	emptyID, err := typed_id.NewAt(typed_id.SessionNote, time.Date(2026, 1, 2, 3, 4, 8, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, empty := store.SessionNoteCreate(ctx, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: emptyID}, Title: "Empty"}, alice)
	if err != nil || empty.Description != "" || empty.Body != "" {
		t.Fatalf("SessionNoteCreate() with empty optional fields = (%#v, %v)", empty, err)
	}
	for _, event := range []string{"session_note.create", "session_note.update", "session_note.remove"} {
		var count int
		if err := store.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics
				ON topics.workspace = events.workspace AND topics.activity = events.id
			WHERE events.workspace = ? AND events.session = ? AND events.event = ? AND topics.topic = ?
		`, workspace.Id, session.Id, event, database.ActivityTopicSession(session)).Scan(&count); err != nil || count == 0 {
			t.Fatalf("session note activity %q = (%d, %v)", event, count, err)
		}
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
	shared := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	private := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000001"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: shared, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
		VALUES (?, 'ses_00000000000000000000000000', ?, TRUE)
	`, workspace.Id, developersID); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: private, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	sharedEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: shared, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, sharedEvent); err != nil {
		t.Fatal(err)
	}
	privateEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: private, Id: "sev_00000000000000000000000001"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
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
	initial := check(bob, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(shared)}})
	if len(initial) != 1 || initial[0].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() = %#v, want an advanced checkpoint", initial)
	}
	if repeated := check(bob, initial); !reflect.DeepEqual(repeated, initial) {
		t.Fatalf("ActivityTopicCheckpointsGet() with current checkpoints = %#v, want %#v", repeated, initial)
	}
	if checkpoints := check(bob, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(private)}}); checkpoints[0].ID != "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for private session = %#v, want no cursor", checkpoints)
	}
	if checkpoints := check(carol, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(shared)}}); checkpoints[0].ID != "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for ungranted principal = %#v, want no cursors", checkpoints)
	}
	newSharedEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: shared, Id: "sev_00000000000000000000000002"}, Kind: "tool.success", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, newSharedEvent); err != nil {
		t.Fatal(err)
	}
	advanced := check(bob, initial)
	if advanced[0].ID == initial[0].ID {
		t.Fatalf("ActivityTopicCheckpointsGet() after session event = %#v, want an advanced checkpoint from %#v", advanced, initial)
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
	if !typed_id.Valid(typed_id.ActivityEvent, activityID) {
		t.Fatalf("activity event ID %q is invalid", activityID)
	}
	id, err := ulid.ParseStrict(strings.ToUpper(strings.TrimPrefix(activityID, typed_id.ActivityEvent+"_")))
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse("2006-01-02T15:04:05.000Z", createdAt)
	if err != nil || !id.Timestamp().Equal(at) {
		t.Fatalf("activity event ID timestamp = (%v, %v), want %v", id.Timestamp(), err, at)
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
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"},
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
			(?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000001', 'message.text', ?, '{"text":"first"}', '2026-01-01T00:00:00.000Z'),
			(?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000002', 'message.text', ?, '{"text":"second"}', '2026-01-01T00:00:00.000Z'),
			(?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000003', 'message.text', ?, '{"text":"third"}', '2026-01-01T00:00:01.000Z')
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	err, events := store.SessionEventsPageGet(ctx, session, "sev_00000000000000000000000001", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ref.Id != "sev_00000000000000000000000002" || events[1].Ref.Id != "sev_00000000000000000000000003" {
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
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{Alias: "first", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"First"}`, Enabled: true},
			{Alias: "second", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Second"}`, Enabled: true},
			{Alias: "lower", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Lower"}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", ModelAlias: "first", Priority: 2, MaxTurns: 3, SystemPrompt: &firstPrompt, Enabled: true},
			{WorkspaceID: "engineering", ModelAlias: "second", Priority: 2, MaxTurns: 3, SystemPrompt: &emptyPrompt, Enabled: true},
			{WorkspaceID: "engineering", ModelAlias: "lower", Priority: 1, MaxTurns: 2, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var firstID, secondID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'first'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'second'`).Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		err, selected := store.WorkspaceAgentModelSelect(ctx, workspace, "")
		if err != nil || selected == nil {
			t.Fatalf("WorkspaceAgentModelSelect() = (%#v, %v)", selected, err)
		}
		if selected.Ref.Model.Id != firstID && selected.Ref.Model.Id != secondID {
			t.Fatalf("WorkspaceAgentModelSelect() selected %#v outside the highest priority tier", selected)
		}
		if selected.MaxTurns != 3 {
			t.Fatalf("WorkspaceAgentModelSelect() max turns = %d, want 3", selected.MaxTurns)
		}
		if selected.Ref.Model.Id == firstID && (selected.SystemPrompt == nil || *selected.SystemPrompt != "First prompt.") {
			t.Fatalf("WorkspaceAgentModelSelect() system prompt = %#v, want first prompt", selected.SystemPrompt)
		}
		if selected.Ref.Model.Id == secondID && (selected.SystemPrompt == nil || *selected.SystemPrompt != "") {
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
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{Alias: "automatic", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Automatic"}`, Enabled: true},
			{Alias: "requested", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Requested"}`, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", ModelAlias: "automatic", Priority: 2, MaxTurns: 1, Enabled: true},
			{WorkspaceID: "engineering", ModelAlias: "requested", Priority: 1, MaxTurns: 1, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var requestedID, automaticID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'requested'`).Scan(&requestedID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'automatic'`).Scan(&automaticID); err != nil {
		t.Fatal(err)
	}
	err, selected := store.WorkspaceAgentModelSelect(ctx, workspace, requestedID)
	if err != nil || selected == nil || selected.Ref.Model.Id != requestedID {
		t.Fatalf("WorkspaceAgentModelSelect() = (%#v, %v), want requested agent", selected, err)
	}
	err, selected = store.WorkspaceAgentModelSelect(ctx, workspace, "missing")
	if err != nil || selected == nil || selected.Ref.Model.Id != automaticID {
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
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
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
	root := create("sev_00000000000000000000000000", "message.text", nil)
	reasoning := create("sev_00000000000000000000000001", "message.reasoning", &root.Ref)
	call := create("sev_00000000000000000000000002", "tool.request", &root.Ref)
	result := create("sev_00000000000000000000000003", "tool.success", &call.Ref)
	text := create("sev_00000000000000000000000004", "message.text", &root.Ref)

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

	err, entries := store.SessionEventsTreePageGet(ctx, session, "", 10)
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

	otherSession := model.SessionRef{Workspace: session.Workspace, Id: "ses_00000000000000000000000001"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: otherSession, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	err, _ = store.SessionEventsCreate(ctx, model.SessionEvent{
		Ref:             model.SessionEventRef{Session: otherSession, Id: "sev_00000000000000000000000005"},
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
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	root := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}
	call := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}
	events := []model.SessionEvent{
		{Ref: root, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "hello"}},
		{Ref: call, Parent: &root, Kind: "tool.request", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &call, Kind: "tool.success", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
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

	forward := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"}
	err, _ = store.SessionEventsCreateBatch(ctx, []model.SessionEvent{
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000004"}, Parent: &forward, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}},
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
		VALUES (?, 'ses_00000000000000000000000000', 'sev_00000000000000000000000005', 'sev_00000000000000000000000005', 'message.text', ?, '{}', '2026-01-01T00:00:00.000Z')
	`, workspace.Id, alice.Id); err == nil {
		t.Fatal("session events accepted a self parent")
	}
}

func TestSessionApprovalResponseCreatesOneDecisionTask(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:     []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:    []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "test", Parameters: `{}`, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "assistant", Priority: 1, MaxTurns: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	workspace := workspaceRef(t, ctx, store, "engineering")
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: workspace, Model: model.AgentModelRef{Id: agentID}}
	root := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "hello"}}
	if err, _ := store.SessionEventsCreate(ctx, root); err != nil {
		t.Fatal(err)
	}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &root.Ref, Kind: "tool.request", AuthorAgent: &agent, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, tool); err != nil {
		t.Fatal(err)
	}
	request := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &tool.Ref, Kind: "approval.request", AuthorAgent: &agent, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, request); err != nil {
		t.Fatal(err)
	}
	decision := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"}, Parent: &request.Ref, Kind: "approval.approved", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	err, stored := store.SessionApprovalResponseCreate(ctx, decision)
	if err != nil || stored.Ref != decision.Ref {
		t.Fatalf("SessionApprovalResponseCreate() = (%#v, %v)", stored, err)
	}
	err, tasks := store.SessionApprovalDecisionTasksGet(ctx, 10)
	if err != nil || len(tasks) != 1 || tasks[0].Approval != request.Ref || tasks[0].Response != decision.Ref {
		t.Fatalf("SessionApprovalDecisionTasksGet() = (%#v, %v)", tasks, err)
	}
	rejected := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000004"}, Parent: &request.Ref, Kind: "approval.rejected", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	err, _ = store.SessionApprovalResponseCreate(ctx, rejected)
	if !errors.Is(err, database.ErrSessionApprovalResolved) {
		t.Fatalf("second SessionApprovalResponseCreate() error = %v", err)
	}
	if err := store.SessionApprovalDecisionTaskDelivered(ctx, request.Ref); err != nil {
		t.Fatal(err)
	}
	err, tasks = store.SessionApprovalDecisionTasksGet(ctx, 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("delivered SessionApprovalDecisionTasksGet() = (%#v, %v)", tasks, err)
	}
}
