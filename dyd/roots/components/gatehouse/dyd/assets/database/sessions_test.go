package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/sessionsearch"
	"gatehouse/typed_id"

	"github.com/oklog/ulid/v2"
)

func TestSessionFileUpdateChangesOnlyName(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:                []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:                []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}},
		StorageProviders:          []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	bob := principalRef(t, ctx, store, "bob")
	sessionID, err := typed_id.New(typed_id.Session)
	if err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: sessionID}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	memberGrantID, err := typed_id.New(typed_id.SessionGrant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled)
		VALUES (?, ?, ?, ?, ?, NULL, TRUE)
	`, memberGrantID, workspace.Id, session.Id, authz.Member, bob.Id); err != nil {
		t.Fatal(err)
	}
	fileID, err := typed_id.New(typed_id.SessionFile)
	if err != nil {
		t.Fatal(err)
	}
	objectID, err := typed_id.New(typed_id.StorageObject)
	if err != nil {
		t.Fatal(err)
	}
	file := model.SessionFile{Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: "draft.txt", Enabled: true}
	if err, _, _ := store.SessionFileCreate(ctx, file, objectID, alice); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("immutable bytes"))
	if err := store.StorageObjectStoreIntegrity(ctx, objectID, digest[:], int64(len("immutable bytes"))); err != nil {
		t.Fatal(err)
	}
	if err := store.StorageObjectMarkSuccess(ctx, objectID); err != nil {
		t.Fatal(err)
	}
	if err, _, _ := store.SessionFileFinish(ctx, file.Ref, alice); err != nil {
		t.Fatal(err)
	}
	err, updated, object := store.SessionFileUpdate(ctx, file.Ref, database.FileUpdate{Name: "  final.txt  "}, alice)
	if err != nil || updated == nil || object == nil || updated.Ref != file.Ref || updated.Name != "final.txt" || updated.StorageObject.Id != objectID || object.ID != objectID || object.Size != int64(len("immutable bytes")) || object.State != "success" {
		t.Fatalf("SessionFileUpdate() = (%#v, %#v, %v)", updated, object, err)
	}
	err, files := store.SessionFilesGet(ctx, session)
	if err != nil || len(files) != 1 || files[0].ID != fileID || files[0].Name != "final.txt" || files[0].Fingerprint != "sha256:59d8792018a51a408d2738f31eedebd6fe9926cc4260fa168a38710bc51d7e30" {
		t.Fatalf("SessionFilesGet() after update = (%#v, %v)", files, err)
	}
	var updates int
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM gatehouse_activity_events
		WHERE event = 'session_file.update' AND resource_session_file = ?
	`, fileID).Scan(&updates); err != nil || updates != 2 {
		t.Fatalf("session file update activity = (%d, %v), want 2", updates, err)
	}
	if err, visible, object := store.SessionFileGet(ctx, file.Ref, bob); err != nil || visible == nil || object == nil {
		t.Fatalf("SessionFileGet() for member = (%#v, %#v, %v)", visible, object, err)
	}
	if err, updated, object := store.SessionFileUpdate(ctx, file.Ref, database.FileUpdate{Name: "private.txt"}, bob); err == nil || updated != nil || object != nil {
		t.Fatalf("SessionFileUpdate() for member = (%#v, %#v, %v)", updated, object, err)
	}
	if err, _, _ := store.SessionFileUpdate(ctx, file.Ref, database.FileUpdate{Name: "  "}, alice); err == nil {
		t.Fatal("SessionFileUpdate() accepted a blank name")
	}
	if err, updated, object := store.SessionFileUpdate(ctx, file.Ref, database.FileUpdate{Name: "final.txt"}, alice); err != nil || updated == nil || object == nil {
		t.Fatalf("SessionFileUpdate() no-op = (%#v, %#v, %v)", updated, object, err)
	}
	if err, removed := store.SessionFileRemove(ctx, file.Ref, alice); err != nil || !removed {
		t.Fatalf("SessionFileRemove() = (%t, %v)", removed, err)
	}
	if err, updated, object := store.SessionFileUpdate(ctx, file.Ref, database.FileUpdate{Name: "restored.txt"}, alice); err != nil || updated != nil || object != nil {
		t.Fatalf("SessionFileUpdate() after removal = (%#v, %#v, %v)", updated, object, err)
	}
}

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
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled) VALUES
			('sgr_00000000000000000000000000', ?, 'ses_00000000000000000000000000', 'member', ?, NULL, TRUE),
			('sgr_00000000000000000000000001', ?, 'ses_00000000000000000000000002', 'contributor', ?, NULL, TRUE),
			('sgr_00000000000000000000000002', ?, 'ses_00000000000000000000000003', 'manager', ?, NULL, FALSE),
			('sgr_00000000000000000000000003', ?, 'ses_00000000000000000000000004', 'member', ?, NULL, TRUE)
	`, workspace.Id, alice.Id, workspace.Id, carol.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled)
			VALUES ('sgr_00000000000000000000000004', ?, 'ses_00000000000000000000000001', 'manager', NULL, ?, TRUE)
	`, workspace.Id, developersID); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		principal model.PrincipalRef
		roles     []authz.Role
	}{
		{principal: alice, roles: []authz.Role{authz.Manager}},
		{principal: bob, roles: []authz.Role{authz.Manager}},
		{principal: carol, roles: []authz.Role{}},
	} {
		err, roles := store.SessionRolesGet(ctx, model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000001"}, test.principal)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(roles, test.roles) {
			t.Fatalf("SessionRolesGet() = %#v, want %#v", roles, test.roles)
		}
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
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:     []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:    []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Enabled: true},
			{WorkspaceID: "engineering", Alias: "assistant-two", ModelAlias: "assistant", Revision: 1, Enabled: true},
		},
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
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled) VALUES
			('sgr_00000000000000000000000005', ?, 'ses_00000000000000000000000000', 'member', ?, NULL, TRUE),
			('sgr_00000000000000000000000006', ?, 'ses_00000000000000000000000001', 'member', ?, NULL, TRUE),
			('sgr_00000000000000000000000007', ?, 'ses_00000000000000000000000002', 'member', ?, NULL, TRUE)
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
	requestMS, inputTokens, cachedInputTokens, outputTokens, reasoningTokens, totalTokens := 1840, 4218, 3072, 611, 384, 4829
	err, stored := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CreatedAt != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("SessionsCreate() timestamp = %q, want ID timestamp", stored.CreatedAt)
	}
	var managerBindings int
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_session_grants
		WHERE workspace = ? AND session = ? AND role = 'manager' AND principal = ?
	`, workspace.Id, session.Id, alice.Id).Scan(&managerBindings); err != nil {
		t.Fatal(err)
	}
	if managerBindings != 1 {
		t.Fatalf("session manager grants = %d, want 1", managerBindings)
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
		Metrics:         &model.SessionEventMetrics{RequestMs: &requestMS, InputTokens: &inputTokens, CachedInputTokens: &cachedInputTokens, OutputTokens: &outputTokens, ReasoningTokens: &reasoningTokens, TotalTokens: &totalTokens},
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
	if len(events) != 2 || events[0].Ref != event.Ref || events[0].Kind != event.Kind || events[0].AuthorPrincipal == nil || events[0].AuthorPrincipal.Ref != alice || events[0].AuthorPrincipal.Name == nil || *events[0].AuthorPrincipal.Name != aliceName || events[0].AuthorAgent != nil || events[0].AuthorGateway != nil || !reflect.DeepEqual(events[0].Payload, event.Payload) || !reflect.DeepEqual(events[0].Metrics, event.Metrics) || events[0].CreatedAt == "" || events[1].Ref != preciselyTimedEvent.Ref || !reflect.DeepEqual(events[1].Metrics, preciselyTimedEvent.Metrics) || events[1].CreatedAt != preciselyTimedEvent.CreatedAt {
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
		INSERT INTO gatehouse_workspace_agents (id, workspace_id, alias, model_id, revision, "default", enabled)
		VALUES ('wag_01arz3ndektsv4rrffq69g5fav', ?, 'assistant', 'amd_01arz3ndektsv4rrffq69g5fav', 1, TRUE, TRUE)
	`, workspace.Id); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_01arz3ndektsv4rrffq69g5fav"}
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
			Id:        "wag_01arz3ndektsv4rrffq69g5fav",
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
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Label: stringPointer("Assistant"), Default: true, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	bob := principalRef(t, ctx, store, "bob")
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = ? AND alias = 'assistant'`, workspace.Id).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
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
		if err != nil || stored.AuthorPrincipal == nil || *stored.AuthorPrincipal != alice || stored.Revision != 1 || stored.Title != title || stored.Description != "Description for "+title+"." || stored.Sensitive || stored.CreatedAt != at.Format("2006-01-02T15:04:05.000Z") {
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
	gateway := model.GatewayRef{Id: "gwy_00000000000000000000000000"}
	title, description, body := "Published credentials guide", "Public replacement", "# Public"
	err, sensitiveDetail = store.SessionNoteDetailsSetAs(ctx, sensitive.Ref, alice, database.NoteAuthor{Gateway: &gateway}, false, &title, &description, &body)
	if err != nil || sensitiveDetail == nil || sensitiveDetail.Note.Sensitive || sensitiveDetail.Note.AuthorGateway == nil || *sensitiveDetail.Note.AuthorGateway != gateway || sensitiveDetail.Note.Revision != 2 {
		t.Fatalf("SessionNoteDetailsSetAs() changes current sensitivity and author = (%#v, %v)", sensitiveDetail, err)
	}
	err, sensitiveRevisions := store.SessionNoteRevisionsGet(ctx, sensitive.Ref, alice)
	if err != nil || len(sensitiveRevisions) != 2 || sensitiveRevisions[0].Ref.Revision != 2 || sensitiveRevisions[0].Sensitive || sensitiveRevisions[0].AuthorGateway == nil || *sensitiveRevisions[0].AuthorGateway != gateway || sensitiveRevisions[1].Ref.Revision != 1 || !sensitiveRevisions[1].Sensitive {
		t.Fatalf("SessionNoteRevisionsGet() preserves revision sensitivity and authors = (%#v, %v)", sensitiveRevisions, err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_session_note_revisions (workspace, session, note, revision, author_agent, title, description, body, sensitive, created_at) VALUES (?, ?, ?, 3, ?, 'Agent revision', 'Agent revision', 'Agent revision', FALSE, '2026-01-01T00:00:00.000Z')`, workspace.Id, session.Id, sensitive.Ref.Id, agent.Id); err != nil {
		t.Fatal(err)
	}
	err, agentRevisions := store.SessionNoteRevisionsGet(ctx, sensitive.Ref, alice)
	if err != nil || len(agentRevisions) != 3 || agentRevisions[0].AuthorAgent == nil || *agentRevisions[0].AuthorAgent != agent || agentRevisions[0].AuthorAgentLabel == nil || *agentRevisions[0].AuthorAgentLabel != "Assistant" {
		t.Fatalf("SessionNoteRevisionsGet() resolves agent labels = (%#v, %v)", agentRevisions, err)
	}
	err, agentRevision := store.SessionNoteRevisionGet(ctx, model.SessionNoteRevisionRef{Note: sensitive.Ref, Revision: 3}, alice)
	if err != nil || agentRevision == nil || agentRevision.Revision.AuthorAgent == nil || *agentRevision.Revision.AuthorAgent != agent || agentRevision.AuthorAgentLabel == nil || *agentRevision.AuthorAgentLabel != "Assistant" {
		t.Fatalf("SessionNoteRevisionGet() resolves agent labels = (%#v, %v)", agentRevision, err)
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
	title, description, body = "Updated guide", "Updated description", "# Updated guide"
	err, updated := store.SessionNoteDetailsSetAs(ctx, older.Ref, alice, database.NoteAuthor{Principal: &alice}, false, &title, &description, &body)
	if err != nil || updated == nil || updated.Note.AuthorPrincipal == nil || *updated.Note.AuthorPrincipal != alice || updated.Note.Revision != 2 || updated.Note.Title != title || updated.Note.Description != description || updated.Note.Body != body {
		t.Fatalf("SessionNoteDetailsSetAs() = (%#v, %v)", updated, err)
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
	if err != nil || len(notes) != 2 || notes[0].Ref.Id != sensitive.Ref.Id || notes[0].Sensitive || notes[1].Ref.Id != older.Ref.Id {
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
	for event, topic := range map[string]string{
		"session_note.create": database.ActivityTopicSessionNote(older.Ref),
		"session_note.update": database.ActivityTopicSessionNote(sensitive.Ref),
		"session_note.remove": database.ActivityTopicSessionNote(newer.Ref),
	} {
		var count int
		if err := store.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics
				ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic = ?
		`, event, topic).Scan(&count); err != nil || count == 0 {
			t.Fatalf("session note activity %q = (%d, %v)", event, count, err)
		}
	}
}

func TestSessionTasksUseSessionAuthorizationAndActivity(t *testing.T) {
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
	sessionID, err := typed_id.New(typed_id.Session)
	if err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: sessionID}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	olderID, err := typed_id.NewAt(typed_id.SessionTask, time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, older := store.SessionTaskCreate(ctx, model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: olderID}, Title: "  Implement tasks  ", Description: stringPointer("  Backend work  "), Sensitive: true}, alice)
	if err != nil || older.Status != "draft" || older.CreatorPrincipal == nil || *older.CreatorPrincipal != alice || older.UpdaterPrincipal == nil || *older.UpdaterPrincipal != alice || older.Description == nil || *older.Description != "Backend work" {
		t.Fatalf("SessionTaskCreate() = (%#v, %v)", older, err)
	}
	newerID, err := typed_id.NewAt(typed_id.SessionTask, time.Date(2026, 1, 2, 3, 4, 6, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, newer := store.SessionTaskCreate(ctx, model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: newerID}, Title: "Review", Status: "ready"}, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, tasks := store.SessionTasksGet(ctx, session, alice)
	if err != nil || len(tasks) != 2 || tasks[0].Ref != newer.Ref || tasks[0].Description != nil || tasks[1].Description != nil {
		t.Fatalf("SessionTasksGet() = (%#v, %v)", tasks, err)
	}
	err, detail := store.SessionTaskGet(ctx, older.Ref, alice)
	if err != nil || detail == nil || detail.Description == nil || *detail.Description != "Backend work" {
		t.Fatalf("SessionTaskGet() = (%#v, %v)", detail, err)
	}
	err, hidden := store.SessionTaskGet(ctx, older.Ref, bob)
	if err != nil || hidden != nil {
		t.Fatalf("SessionTaskGet() for ungranted principal = (%#v, %v)", hidden, err)
	}
	if createErr, _ := store.SessionTaskCreate(ctx, model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: newerID}, Title: "Denied"}, bob); createErr == nil {
		t.Fatal("SessionTaskCreate() accepted an ungranted principal")
	}
	err, updated := store.SessionTaskDetailsSetAs(ctx, older.Ref, alice, database.TaskAuthor{Gateway: &model.GatewayRef{Id: "gwy_00000000000000000000000000"}}, false, "done", "Implemented", nil)
	if err != nil || updated == nil || updated.Status != "done" || updated.Description != nil || updated.UpdaterGateway == nil {
		t.Fatalf("SessionTaskDetailsSetAs() = (%#v, %v)", updated, err)
	}
	err, tasks = store.SessionTasksGet(ctx, session, alice)
	if err != nil || len(tasks) != 2 || tasks[0].Ref != newer.Ref {
		t.Fatalf("SessionTasksGet() after update = (%#v, %v)", tasks, err)
	}
	err, removed := store.SessionTaskRemove(ctx, newer.Ref, bob)
	if err != nil || removed {
		t.Fatalf("SessionTaskRemove() for ungranted principal = (%t, %v)", removed, err)
	}
	err, removed = store.SessionTaskRemove(ctx, newer.Ref, alice)
	if err != nil || !removed {
		t.Fatalf("SessionTaskRemove() = (%t, %v)", removed, err)
	}
	err, hidden = store.SessionTaskGet(ctx, newer.Ref, alice)
	if err != nil || hidden != nil {
		t.Fatalf("SessionTaskGet() after removal = (%#v, %v)", hidden, err)
	}
	for event, task := range map[string]model.SessionTaskRef{"session_task.create": older.Ref, "session_task.update": older.Ref, "session_task.remove": newer.Ref} {
		var count int
		if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events AS events JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id WHERE events.event = ? AND events.resource_session_task = ? AND topics.topic = ?`, event, task.Id, database.ActivityTopicSessionTask(task)).Scan(&count); err != nil || count != 1 {
			t.Fatalf("session task activity %q = (%d, %v)", event, count, err)
		}
	}
}

func TestActivityTopicCheckpointsGetAuthorizesContextRootsAndAdvancesIndependently(t *testing.T) {
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
		WorkspaceGrants: []config.WorkspaceGrant{{
			WorkspaceID: "engineering", Role: "member", GroupID: stringPointer("developers"), Revision: 1, Enabled: true,
		}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	developersID := groupID(t, ctx, store, "engineering", "developers")
	var workspaceGrantID string
	if err := store.QueryRowContext(ctx, `
		SELECT id FROM gatehouse_workspace_grants
		WHERE workspace = ? AND "group" = ?
	`, workspace.Id, developersID).Scan(&workspaceGrantID); err != nil {
		t.Fatal(err)
	}
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
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled)
		VALUES ('sgr_00000000000000000000000000', ?, 'ses_00000000000000000000000000', 'contributor', NULL, ?, TRUE)
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
		err, advanced := store.ActivityTopicCheckpointsGet(ctx, principal, checkpoints)
		if err != nil {
			t.Fatal(err)
		}
		return advanced
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, activity := range []struct {
		kind, topic string
	}{
		{database.ActivityResourceKindWorkspace, database.ActivityTopicWorkspace(workspace)},
		{database.ActivityResourceKindWorkspaceGrant, database.ActivityTopicGroupGrant(workspace, model.GroupRef{Id: developersID}, workspaceGrantID)},
		{database.ActivityResourceKindWorkspace, bob.Id},
	} {
		event := model.ActivityEvent{
			Ref:          model.ActivityEventRef{},
			Event:        activity.kind + ".update",
			ResourceKind: activity.kind,
		}
		switch activity.kind {
		case database.ActivityResourceKindWorkspace:
			event.ResourceWorkspace = &workspace.Id
		case database.ActivityResourceKindWorkspaceGrant:
			event.ResourceWorkspaceGrant = &workspaceGrantID
		}
		if err, _ := store.ActivityEventAppend(ctx, transaction, event, []string{activity.topic}); err != nil {
			t.Fatal(err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{
		database.ActivityTopicWorkspace(workspace),
		database.ActivityTopicWorkspace(workspace),
		database.ActivityTopicWorkspace(workspace),
	} {
		if checkpoints := check(bob, []database.ActivityTopicCheckpoint{{Topic: topic, Events: []string{"workspace.*"}}}); checkpoints[0].ID == "" {
			t.Fatalf("ActivityTopicCheckpointsGet() for workspace topic %q = %#v, want a cursor", topic, checkpoints)
		}
		if checkpoints := check(carol, []database.ActivityTopicCheckpoint{{Topic: topic, Events: []string{"workspace.*"}}}); checkpoints[0].ID != "" {
			t.Fatalf("ActivityTopicCheckpointsGet() for unavailable workspace topic %q = %#v, want no cursor", topic, checkpoints)
		}
	}
	principalInitial := check(bob, []database.ActivityTopicCheckpoint{{Topic: bob.Id, Events: []string{"workspace.*"}}})
	if len(principalInitial) != 1 || principalInitial[0].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for principal topic = %#v, want an advanced checkpoint", principalInitial)
	}
	if denied := check(carol, principalInitial); !reflect.DeepEqual(denied, principalInitial) {
		t.Fatalf("ActivityTopicCheckpointsGet() for another principal topic = %#v, want unchanged %#v", denied, principalInitial)
	}
	initial := check(bob, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(shared), Events: []string{"session.*", "session_event.*"}}})
	if len(initial) != 1 || initial[0].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() = %#v, want an advanced checkpoint", initial)
	}
	if repeated := check(bob, initial); !reflect.DeepEqual(repeated, initial) {
		t.Fatalf("ActivityTopicCheckpointsGet() with current checkpoints = %#v, want %#v", repeated, initial)
	}
	if checkpoints := check(bob, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(private), Events: []string{"session.*"}}}); checkpoints[0].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for workspace session topic = %#v, want a cursor", checkpoints)
	}
	if checkpoints := check(carol, []database.ActivityTopicCheckpoint{{Topic: database.ActivityTopicSession(shared), Events: []string{"session.*"}}}); checkpoints[0].ID != "" {
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
		WHERE resource_kind = 'session'
		LIMIT 1
	`).Scan(&activityID, &createdAt); err != nil {
		t.Fatal(err)
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

func TestSessionMessagesCreateOnlyAddsTasksForTargetedMessagesAndEventsPageUsesKeyset(t *testing.T) {
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
		AgentModels:    []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Enabled: true},
			{WorkspaceID: "engineering", Alias: "assistant-two", ModelAlias: "assistant", Revision: 1, Enabled: true},
		},
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
	var agentID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	var secondAgentID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant-two'`).Scan(&secondAgentID); err != nil {
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
	if err != nil || len(tasks) != 0 {
		t.Fatalf("SessionEventReplyTasksGet() for human-only message = (%#v, %v)", tasks, err)
	}
	targeted := message
	targeted.Ref.Id = "sev_00000000000000000000000004"
	targeted.Payload = map[string]interface{}{"text": "ask assistants", "agents": []string{agentID, agentID, secondAgentID}}
	if err, _ := store.SessionMessagesCreate(ctx, targeted); err != nil {
		t.Fatal(err)
	}
	err, tasks = store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 2 || tasks[0].Event == targeted.Ref || tasks[1].Event == targeted.Ref {
		t.Fatalf("SessionEventReplyTasksGet() for targeted message = (%#v, %v)", tasks, err)
	}
	targets := map[string]bool{}
	for _, task := range tasks {
		err, request := store.SessionEventGet(ctx, task.Event)
		if err != nil || request == nil || request.Kind != "agent.request" || request.Parent == nil || *request.Parent != targeted.Ref || request.AuthorPrincipal == nil || request.AuthorPrincipal.Ref != alice {
			t.Fatalf("created agent request = (%#v, %v)", request, err)
		}
		target, _ := request.Payload["agent"].(string)
		targets[target] = true
	}
	if !targets[agentID] || !targets[secondAgentID] || len(targets) != 2 {
		t.Fatalf("agent request targets = %#v", targets)
	}
	err, nameTasks := store.SessionNameTasksGet(ctx, 10)
	if err != nil || len(nameTasks) != 1 || nameTasks[0].Session != session {
		t.Fatalf("SessionNameTasksGet() for targeted message = (%#v, %v)", nameTasks, err)
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

func TestSessionAgentRequestEventsGetUsesRequestSnapshot(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var agentID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	create := func(text string) model.SessionEvent {
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			t.Fatal(err)
		}
		err, message := store.SessionMessagesCreate(ctx, model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Kind:            "message.text",
			AuthorPrincipal: &alicePrincipal,
			Payload:         map[string]interface{}{"text": text, "agents": []string{agentID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	first := create("first")
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil || len(events) != 2 {
		t.Fatalf("first request events = (%#v, %v)", events, err)
	}
	firstRequest := model.SessionEvent{}
	for _, event := range events {
		if event.Kind == "agent.request" && event.Parent != nil && *event.Parent == first.Ref {
			firstRequest = event
		}
	}
	if firstRequest.Ref.Id == "" {
		t.Fatalf("first agent request = %#v", events)
	}
	time.Sleep(time.Millisecond)
	second := create("second")
	err, events = store.SessionEventsGet(ctx, session)
	if err != nil || len(events) != 4 {
		t.Fatalf("second request events = (%#v, %v)", events, err)
	}
	secondRequest := model.SessionEvent{}
	for _, event := range events {
		if event.Kind == "agent.request" && event.Parent != nil && *event.Parent == second.Ref {
			secondRequest = event
		}
	}
	if secondRequest.Ref.Id == "" {
		t.Fatalf("second agent request = %#v", events)
	}
	lateID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionEventsCreate(ctx, model.SessionEvent{
		Ref:         model.SessionEventRef{Session: session, Id: lateID},
		Parent:      &firstRequest.Ref,
		Kind:        model.SessionEventKindAgentSuccess,
		AuthorAgent: &agent,
		Payload:     map[string]interface{}{"text": "late"},
	}); err != nil {
		t.Fatal(err)
	}
	err, visible := store.SessionAgentRequestEventsGet(ctx, secondRequest.Ref, agent)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	var firstVisible *model.SessionEvent
	for _, event := range visible {
		ids[event.Ref.Id] = true
		if event.Ref == first.Ref {
			firstVisible = &event
		}
	}
	if !ids[first.Ref.Id] || !ids[firstRequest.Ref.Id] || !ids[second.Ref.Id] || !ids[secondRequest.Ref.Id] || ids[lateID] {
		t.Fatalf("snapshot IDs = %#v", ids)
	}
	if firstVisible == nil || firstVisible.AuthorPrincipal == nil || firstVisible.AuthorPrincipal.Ref != alice || firstVisible.Payload["text"] != "first" {
		t.Fatalf("hydrated snapshot event = %#v", firstVisible)
	}
}

func TestAgentContextLatestGetSelectsCompatibleCheckpoint(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: 100, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	principal := principalRef(t, ctx, store, "alice")
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principal, Enabled: true}, principal); err != nil {
		t.Fatal(err)
	}
	var agentID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
	roots := make([]model.SessionEventRef, 3)
	for index := range roots {
		roots[index] = model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_0000000000000000000000000%d", index)}
		event := model.SessionEvent{Ref: roots[index], Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: map[string]interface{}{"text": "hello"}}
		if err, _ := store.SessionMessagesCreate(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	for _, context := range []database.AgentContext{
		{Root: roots[0], Model: agent, Profile: "profile", State: json.RawMessage(`{"user_high_from":"first"}`), UpdatedAt: "2026-01-01T00:00:01.000Z"},
		{Root: roots[1], Model: agent, Profile: "profile", State: json.RawMessage(`{"user_high_from":"second"}`), UpdatedAt: "2026-01-01T00:00:02.000Z"},
		{Root: roots[2], Model: agent, Profile: "other", State: json.RawMessage(`{"user_high_from":"other"}`), UpdatedAt: "2026-01-01T00:00:03.000Z"},
	} {
		if err := store.AgentContextSet(ctx, context); err != nil {
			t.Fatal(err)
		}
	}
	err, latest := store.AgentContextLatestGet(ctx, session, agent, "profile")
	if err != nil || latest == nil || latest.Root != roots[1] || string(latest.State) != `{"user_high_from":"second"}` {
		t.Fatalf("AgentContextLatestGet() = (%#v, %v)", latest, err)
	}
	err, missing := store.AgentContextLatestGet(ctx, session, agent, "missing")
	if err != nil || missing != nil {
		t.Fatalf("AgentContextLatestGet() missing = (%#v, %v)", missing, err)
	}
}

func TestWorkspaceAgentModelGetUsesRequestedBinding(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	firstPrompt := "First prompt."
	emptyPrompt := ""
	prelude := `(let ((profile "first")))`
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{Alias: "first", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"First"}`, MaxTurns: 3, MaxOutputTokens: 2000, Enabled: true},
			{Alias: "second", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Second"}`, MaxTurns: 3, MaxOutputTokens: 2000, Enabled: true},
			{Alias: "lower", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Lower"}`, MaxTurns: 2, MaxOutputTokens: 2000, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Alias: "first", ModelAlias: "first", Revision: 1, SystemPrompt: &firstPrompt, Prelude: &prelude, Default: true, Enabled: true},
			{WorkspaceID: "engineering", Alias: "second", ModelAlias: "second", Revision: 1, SystemPrompt: &emptyPrompt, Enabled: true},
			{WorkspaceID: "engineering", Alias: "lower", ModelAlias: "lower", Revision: 1, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var firstID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'first'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	var bindingID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'first'`).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	err, selected := store.WorkspaceAgentModelGet(ctx, workspace, bindingID)
	if err != nil || selected == nil || selected.AgentModel.Id != firstID || selected.Ref.Id != bindingID || selected.Alias != "first" {
		t.Fatalf("WorkspaceAgentModelGet() = (%#v, %v), want requested binding", selected, err)
	}
	if selected.MaxTurns != 3 || selected.MaxOutputTokens != 2000 {
		t.Fatalf("WorkspaceAgentModelGet() limits = (%d, %d)", selected.MaxTurns, selected.MaxOutputTokens)
	}
	if selected.SystemPrompt == nil || *selected.SystemPrompt != "First prompt." {
		t.Fatalf("WorkspaceAgentModelGet() system prompt = %#v, want first prompt", selected.SystemPrompt)
	}
	if selected.Prelude == nil || *selected.Prelude != prelude {
		t.Fatalf("WorkspaceAgentModelGet() prelude = %#v", selected.Prelude)
	}
}

func TestWorkspaceAgentModelGetRequiresEligibleRequestedAgent(t *testing.T) {
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
			{Alias: "automatic", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Automatic"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true},
			{Alias: "requested", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Requested"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Alias: "automatic", ModelAlias: "automatic", Revision: 1, Default: true, Enabled: true},
			{WorkspaceID: "engineering", Alias: "requested", ModelAlias: "requested", Revision: 1, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var requestedID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'requested'`).Scan(&requestedID); err != nil {
		t.Fatal(err)
	}
	err, selected := store.WorkspaceAgentModelGet(ctx, workspace, requestedID)
	if err != nil || selected == nil || selected.Ref.Id != requestedID {
		t.Fatalf("WorkspaceAgentModelGet() = (%#v, %v), want requested agent", selected, err)
	}
	err, selected = store.WorkspaceAgentModelGet(ctx, workspace, "wag_00000000000000000000000000")
	if err != nil || selected != nil {
		t.Fatalf("WorkspaceAgentModelGet() with missing binding = (%#v, %v)", selected, err)
	}
	state.WorkspaceAgents[1].Enabled = false
	state.WorkspaceAgents[1].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	err, selected = store.WorkspaceAgentModelGet(ctx, workspace, requestedID)
	if err != nil || selected != nil {
		t.Fatalf("WorkspaceAgentModelGet() with disabled binding = (%#v, %v)", selected, err)
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
	notice := create("sev_00000000000000000000000001", "gateway.notice", &root.Ref)
	call := create("sev_00000000000000000000000002", "tool.request", &root.Ref)
	result := create("sev_00000000000000000000000003", "tool.success", &call.Ref)
	text := create("sev_00000000000000000000000004", "message.text", &root.Ref)
	err, newest := store.SessionEventsTailPageGet(ctx, session, "", 2)
	if err != nil || len(newest) != 2 || newest[0].Ref != result.Ref || newest[1].Ref != text.Ref {
		t.Fatalf("SessionEventsTailPageGet(newest) = (%#v, %v)", newest, err)
	}
	err, older := store.SessionEventsTailPageGet(ctx, session, newest[0].Ref.Id, 2)
	if err != nil || len(older) != 2 || older[0].Ref != notice.Ref || older[1].Ref != call.Ref {
		t.Fatalf("SessionEventsTailPageGet(older) = (%#v, %v)", older, err)
	}
	err, oldest := store.SessionEventsTailPageGet(ctx, session, older[0].Ref.Id, 2)
	if err != nil || len(oldest) != 1 || oldest[0].Ref != root.Ref {
		t.Fatalf("SessionEventsTailPageGet(oldest) = (%#v, %v)", oldest, err)
	}

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
		{notice, root.Ref},
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
		{notice.Ref.Id, 1},
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

func TestSessionEventsSearch(t *testing.T) {
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
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	for _, event := range []model.SessionEvent{
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "Conroe ordinance"}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Kind: "tool.success", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"output": "Conroe zoning"}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Kind: "gateway.notice", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "Conroe ordinance"}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"}, Kind: "approval.request", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"description": "Approve Conroe ordinance"}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000004"}, Kind: "tool.request", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"name": "lisp", "call_id": "call", "code": `say "Conroe"`, "reason": "Explain the quote."}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000005"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": float64(42)}},
		{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000006"}, Kind: model.SessionEventKindAgentSuccess, AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "Irving response"}},
	} {
		if err, _ := store.SessionEventsCreate(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	err, expression := sessionsearch.Parse(`(and "conroe" (or "ordinance" "zoning"))`)
	if err != nil {
		t.Fatal(err)
	}
	err, events, more := store.SessionEventsSearch(ctx, session, expression, "", 2)
	if err != nil || !more || len(events) != 2 || events[0].Ref.Id != "sev_00000000000000000000000003" || events[1].Ref.Id != "sev_00000000000000000000000001" {
		t.Fatalf("SessionEventsSearch() = (%#v, %t, %v)", events, more, err)
	}
	err, events, more = store.SessionEventsSearch(ctx, session, expression, events[1].Ref.Id, 2)
	if err != nil || more || len(events) != 1 || events[0].Ref.Id != "sev_00000000000000000000000000" {
		t.Fatalf("SessionEventsSearch() second page = (%#v, %t, %v)", events, more, err)
	}
	err, expression = sessionsearch.Parse(`"\"Conroe\""`)
	if err != nil {
		t.Fatal(err)
	}
	err, events, more = store.SessionEventsSearch(ctx, session, expression, "", 2)
	if err != nil || more || len(events) != 1 || events[0].Ref.Id != "sev_00000000000000000000000004" {
		t.Fatalf("SessionEventsSearch() quoted request = (%#v, %t, %v)", events, more, err)
	}
	err, expression = sessionsearch.Parse(`"42"`)
	if err != nil {
		t.Fatal(err)
	}
	err, events, more = store.SessionEventsSearch(ctx, session, expression, "", 2)
	if err != nil || more || len(events) != 0 {
		t.Fatalf("SessionEventsSearch() malformed text = (%#v, %t, %v)", events, more, err)
	}
	err, expression = sessionsearch.Parse(`"Irving"`)
	if err != nil {
		t.Fatal(err)
	}
	err, events, more = store.SessionEventsSearch(ctx, session, expression, "", 2)
	if err != nil || more || len(events) != 1 || events[0].Ref.Id != "sev_00000000000000000000000006" {
		t.Fatalf("SessionEventsSearch() agent reply = (%#v, %t, %v)", events, more, err)
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
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "test", Parameters: `{}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
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
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
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
	decision := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"}, Parent: &request.Ref, Kind: model.SessionEventKindApprovalSuccess, AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	err, stored := store.SessionApprovalResponseCreate(ctx, decision)
	if err != nil || stored.Ref != decision.Ref {
		t.Fatalf("SessionApprovalResponseCreate() = (%#v, %v)", stored, err)
	}
	err, tasks := store.SessionApprovalDecisionTasksGet(ctx, 10)
	if err != nil || len(tasks) != 1 || tasks[0].Approval != request.Ref || tasks[0].Response != decision.Ref {
		t.Fatalf("SessionApprovalDecisionTasksGet() = (%#v, %v)", tasks, err)
	}
	rejected := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000004"}, Parent: &request.Ref, Kind: model.SessionEventKindApprovalFailure, AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"code": "rejected"}}
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
