package database_test

import (
	"context"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestProjectsAuthorizeSessionsAndPublishActivity(t *testing.T) {
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
	projectID, err := typed_id.New(typed_id.Project)
	if err != nil {
		t.Fatal(err)
	}
	projectRef := model.ProjectRef{Workspace: workspace, Id: projectID}
	err, project := store.ProjectsCreate(ctx, model.Project{Ref: projectRef, Enabled: true}, alice, nil)
	if err != nil {
		t.Fatal(err)
	}
	if project.CreatedAt == "" {
		t.Fatal("ProjectsCreate() did not derive a creation timestamp")
	}
	if project.Name != nil {
		t.Fatalf("ProjectsCreate() name = %q, want nil", *project.Name)
	}
	err, projects := store.ProjectsGet(ctx, workspace, alice)
	if err != nil || len(projects) != 1 || projects[0].Ref != projectRef {
		t.Fatalf("ProjectsGet() for creator = (%#v, %v)", projects, err)
	}
	err, projects = store.ProjectsGet(ctx, workspace, bob)
	if err != nil || len(projects) != 0 {
		t.Fatalf("ProjectsGet() for ungranted principal = (%#v, %v)", projects, err)
	}

	sessionID, err := typed_id.New(typed_id.Session)
	if err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: sessionID}
	err, _ = store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, linked := store.SessionProjectSet(ctx, session, &projectRef, alice)
	if err != nil || linked == nil || linked.Project == nil || *linked.Project != projectRef {
		t.Fatalf("SessionProjectSet() link = (%#v, %v)", linked, err)
	}
	checkpoints := []database.ActivityTopicCheckpoint{
		{Topic: "project/*"},
		{Topic: database.ActivityTopicSession(session)},
	}
	err, advanced := store.ActivityTopicCheckpointsGet(ctx, workspace, alice, checkpoints)
	if err != nil || advanced[0].ID == "" || advanced[1].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for creator = (%#v, %v)", advanced, err)
	}
	err, hidden := store.ActivityTopicCheckpointsGet(ctx, workspace, bob, checkpoints)
	if err != nil || hidden[0].ID != "" || hidden[1].ID != "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for ungranted principal = (%#v, %v)", hidden, err)
	}
	err, linked = store.SessionProjectSet(ctx, session, nil, alice)
	if err != nil || linked == nil || linked.Project != nil {
		t.Fatalf("SessionProjectSet() unlink = (%#v, %v)", linked, err)
	}
}

func TestProjectsSearchMatchesNamesAndPaginatesByID(t *testing.T) {
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
		INSERT INTO gatehouse_projects (workspace, id, name, enabled, created_at) VALUES
			(?, 'prj_00000000000000000000000000', 'Website refresh', TRUE, '2026-01-01 00:00:00'),
			(?, 'prj_00000000000000000000000001', 'Mobile refresh', TRUE, '2026-01-01 00:00:01'),
			(?, 'prj_00000000000000000000000002', 'Operations', TRUE, '2026-01-01 00:00:02')
	`, workspace.Id, workspace.Id, workspace.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_project_principal_grants (workspace, project, principal, enabled) VALUES
			(?, 'prj_00000000000000000000000000', ?, TRUE),
			(?, 'prj_00000000000000000000000001', ?, TRUE),
			(?, 'prj_00000000000000000000000002', ?, TRUE)
	`, workspace.Id, alice.Id, workspace.Id, alice.Id, workspace.Id, alice.Id); err != nil {
		t.Fatal(err)
	}
	err, first, cursor := store.ProjectsSearch(ctx, workspace, alice, database.ProjectSearch{Name: "REFRESH", Limit: 1})
	if err != nil || len(first) != 1 || first[0].Ref.Id != "prj_00000000000000000000000001" || cursor != first[0].Ref.Id {
		t.Fatalf("ProjectsSearch() first page = (%#v, %q, %v)", first, cursor, err)
	}
	err, second, cursor := store.ProjectsSearch(ctx, workspace, alice, database.ProjectSearch{Name: "refresh", Cursor: cursor, Limit: 1})
	if err != nil || len(second) != 1 || second[0].Ref.Id != "prj_00000000000000000000000000" || cursor != "" {
		t.Fatalf("ProjectsSearch() second page = (%#v, %q, %v)", second, cursor, err)
	}
}
