package database_test

import (
	"context"
	"reflect"
	"testing"

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
		INSERT INTO gatehouse_sessions (workspace, id, created_by, enabled, created_at) VALUES
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
		{Ref: model.SessionRef{Workspace: workspace, Id: "shared"}, CreatedBy: model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:03"},
		{Ref: model.SessionRef{Workspace: workspace, Id: "private"}, CreatedBy: model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:02"},
	}
	if got := sessionsFor("alice"); !reflect.DeepEqual(got, wantAlice) {
		t.Fatalf("SessionsGet() for alice = %#v, want %#v", got, wantAlice)
	}
	wantBob := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "shared"}, CreatedBy: model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:03",
	}}
	if got := sessionsFor("bob"); !reflect.DeepEqual(got, wantBob) {
		t.Fatalf("SessionsGet() for bob = %#v, want %#v", got, wantBob)
	}
	wantCarol := []model.Session{{
		Ref: model.SessionRef{Workspace: workspace, Id: "direct"}, CreatedBy: model.PrincipalRef{Id: "alice"}, Enabled: true, CreatedAt: "2026-01-01 00:00:01",
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
