package database_test

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestProjectFilesUseProjectAuthorizationAndManagedStorage(t *testing.T) {
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
		StorageProviders: []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{{WorkspaceID: "engineering", ProviderAlias: "embedded", Priority: 1, Enabled: true}},
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
	project := model.ProjectRef{Workspace: workspace, Id: projectID}
	if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, alice, nil); err != nil {
		t.Fatal(err)
	}
	activity := func(event string, want int) {
		var got int
		if err := store.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics
				ON topics.workspace = events.workspace AND topics.activity = events.id
			WHERE events.workspace = ? AND events.event = ? AND topics.topic = ?
		`, workspace.Id, event, database.ActivityTopicProject(project)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s activity count = %d, want %d", event, got, want)
		}
	}
	pendingFileID, err := typed_id.NewAt(typed_id.ProjectFile, time.Date(2026, 1, 2, 3, 4, 2, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	pendingObjectID, err := typed_id.NewAt(typed_id.StorageObject, time.Date(2026, 1, 2, 3, 4, 2, 679_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	pending := model.ProjectFile{Ref: model.ProjectFileRef{Project: project, Id: pendingFileID}, Name: "pending.txt", Enabled: true}
	if err, _, _ := store.ProjectFileCreate(ctx, pending, pendingObjectID, alice); err != nil {
		t.Fatal(err)
	}
	activity("project_file.create", 1)
	digest := sha256.Sum256([]byte("pending"))
	if err := store.StorageObjectStoreIntegrity(ctx, pendingObjectID, digest[:], int64(len("pending"))); err != nil {
		t.Fatal(err)
	}
	if err := store.StorageObjectMarkSuccess(ctx, pendingObjectID); err != nil {
		t.Fatal(err)
	}
	if err, _, _ := store.ProjectFileFinish(ctx, pending.Ref, alice); err != nil {
		t.Fatal(err)
	}
	activity("project_file.update", 1)
	revokedFileID, err := typed_id.NewAt(typed_id.ProjectFile, time.Date(2026, 1, 2, 3, 4, 3, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	revokedObjectID, err := typed_id.NewAt(typed_id.StorageObject, time.Date(2026, 1, 2, 3, 4, 3, 679_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	revoked := model.ProjectFile{Ref: model.ProjectFileRef{Project: project, Id: revokedFileID}, Name: "revoked.txt", Enabled: true}
	if err, _, _ := store.ProjectFileCreate(ctx, revoked, revokedObjectID, alice); err != nil {
		t.Fatal(err)
	}
	if err, removed := store.ProjectFileRemove(ctx, revoked.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectFileRemove() pending = (%t, %v)", removed, err)
	}
	if err, provider := store.StorageObjectPendingGet(ctx, revokedObjectID); err != nil || provider != nil {
		t.Fatalf("StorageObjectPendingGet() after project file removal = (%#v, %v)", provider, err)
	}
	activity("project_file.remove", 1)
	deniedFileID, err := typed_id.NewAt(typed_id.ProjectFile, time.Date(2026, 1, 2, 3, 4, 4, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	deniedObjectID, err := typed_id.NewAt(typed_id.StorageObject, time.Date(2026, 1, 2, 3, 4, 4, 679_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if createErr, _, _ := store.ProjectFileCreate(ctx, model.ProjectFile{Ref: model.ProjectFileRef{Project: project, Id: deniedFileID}, Name: "private.txt", Enabled: true}, deniedObjectID, bob); createErr == nil {
		t.Fatal("ProjectFileCreate() accepted an ungranted principal")
	}

	create := func(at time.Time, name string) (model.ProjectFileRef, string) {
		fileID, err := typed_id.NewAt(typed_id.ProjectFile, at)
		if err != nil {
			t.Fatal(err)
		}
		objectID, err := typed_id.NewAt(typed_id.StorageObject, at.Add(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		file := model.ProjectFile{Ref: model.ProjectFileRef{Project: project, Id: fileID}, Name: name, MediaType: stringPointer("application/octet-stream"), Enabled: true}
		err, stored, objectID := store.ProjectFileCreate(ctx, file, objectID, alice)
		if err != nil || stored.CreatedAt != at.Format("2006-01-02T15:04:05.000Z") || stored.StorageObject.Id != objectID {
			t.Fatalf("ProjectFileCreate() = (%#v, %q, %v)", stored, objectID, err)
		}
		err, pending, object := store.ProjectFileGet(ctx, file.Ref, alice)
		if err != nil || pending == nil || object == nil || object.State != "pending" {
			t.Fatalf("ProjectFileGet() pending = (%#v, %#v, %v)", pending, object, err)
		}
		digest := sha256.Sum256([]byte(name))
		if err := store.StorageObjectStoreIntegrity(ctx, objectID, digest[:], int64(len(name))); err != nil {
			t.Fatal(err)
		}
		if err := store.StorageObjectMarkSuccess(ctx, objectID); err != nil {
			t.Fatal(err)
		}
		return file.Ref, objectID
	}

	older, _ := create(time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC), "requirements.txt")
	newer, _ := create(time.Date(2026, 1, 2, 3, 4, 6, 678_000_000, time.UTC), "design.pdf")
	err, files := store.ProjectFilesGet(ctx, project, alice)
	if err != nil || len(files) != 3 || files[0].ID != newer.Id || files[0].Name != "design.pdf" || files[0].MediaType == nil || *files[0].MediaType != "application/octet-stream" || files[0].Size != int64(len("design.pdf")) || files[0].Fingerprint != "sha256:2699a8a0c49f286591802474c202cf177fb2b9de754315de6051485c5d30f10f" || files[1].ID != older.Id || files[2].ID != pending.Ref.Id {
		t.Fatalf("ProjectFilesGet() = (%#v, %v)", files, err)
	}
	err, denied := store.ProjectFilesGet(ctx, project, bob)
	if err != nil || len(denied) != 0 {
		t.Fatalf("ProjectFilesGet() for ungranted principal = (%#v, %v)", denied, err)
	}
	err, hidden, object := store.ProjectFileGet(ctx, newer, bob)
	if err != nil || hidden != nil || object != nil {
		t.Fatalf("ProjectFileGet() for ungranted principal = (%#v, %#v, %v)", hidden, object, err)
	}
	err, removed := store.ProjectFileRemove(ctx, newer, bob)
	if err != nil || removed {
		t.Fatalf("ProjectFileRemove() for ungranted principal = (%t, %v)", removed, err)
	}
	err, removed = store.ProjectFileRemove(ctx, newer, alice)
	if err != nil || !removed {
		t.Fatalf("ProjectFileRemove() = (%t, %v)", removed, err)
	}
	err, hidden, object = store.ProjectFileGet(ctx, newer, alice)
	if err != nil || hidden != nil || object != nil {
		t.Fatalf("ProjectFileGet() after removal = (%#v, %#v, %v)", hidden, object, err)
	}
	err, files = store.ProjectFilesGet(ctx, project, alice)
	if err != nil || len(files) != 2 || files[0].ID != older.Id || files[1].ID != pending.Ref.Id {
		t.Fatalf("ProjectFilesGet() after removal = (%#v, %v)", files, err)
	}
}

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
