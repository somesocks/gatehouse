package database_test

import (
	"context"
	"crypto/sha256"
	"strings"
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

func TestProjectNotesUseProjectAuthorizationAndActivity(t *testing.T) {
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
	project := model.ProjectRef{Workspace: workspace, Id: projectID}
	if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, alice, nil); err != nil {
		t.Fatal(err)
	}
	create := func(at time.Time, title string) model.ProjectNote {
		id, err := typed_id.NewAt(typed_id.ProjectNote, at)
		if err != nil {
			t.Fatal(err)
		}
		err, stored := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: id}, Title: "  "+title+"  ", Description: "  Description for "+title+".  ", Body: "# "+title}, alice)
		if err != nil || stored.AuthorPrincipal != alice || stored.Title != title || stored.Description != "Description for "+title+"." || stored.Sensitive || stored.CreatedAt != at.Format("2006-01-02T15:04:05.000Z") {
			t.Fatalf("ProjectNoteCreate() = (%#v, %v)", stored, err)
		}
		return stored
	}
	older := create(time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC), "Guide")
	newer := create(time.Date(2026, 1, 2, 3, 4, 6, 678_000_000, time.UTC), "Architecture")
	sensitiveID, err := typed_id.NewAt(typed_id.ProjectNote, time.Date(2026, 1, 2, 3, 4, 6, 679_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, sensitive := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: sensitiveID}, Title: "Credentials", Body: "secret", Sensitive: true}, alice)
	if err != nil || !sensitive.Sensitive {
		t.Fatalf("ProjectNoteCreate() sensitive = (%#v, %v)", sensitive, err)
	}
	err, notes := store.ProjectNotesGet(ctx, project, alice)
	if err != nil || len(notes) != 3 || notes[0].Ref.Id != sensitive.Ref.Id || !notes[0].Sensitive || notes[1].Ref.Id != newer.Ref.Id || notes[1].Title != "Architecture" || notes[2].Ref.Id != older.Ref.Id {
		t.Fatalf("ProjectNotesGet() = (%#v, %v)", notes, err)
	}
	err, sensitiveDetail := store.ProjectNoteGet(ctx, sensitive.Ref, alice)
	if err != nil || sensitiveDetail == nil || !sensitiveDetail.Note.Sensitive {
		t.Fatalf("ProjectNoteGet() sensitive = (%#v, %v)", sensitiveDetail, err)
	}
	err, denied := store.ProjectNotesGet(ctx, project, bob)
	if err != nil || len(denied) != 0 {
		t.Fatalf("ProjectNotesGet() for ungranted principal = (%#v, %v)", denied, err)
	}
	err, hidden := store.ProjectNoteGet(ctx, newer.Ref, bob)
	if err != nil || hidden != nil {
		t.Fatalf("ProjectNoteGet() for ungranted principal = (%#v, %v)", hidden, err)
	}
	if createErr, _ := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: newer.Ref.Id}, Title: "Denied", Description: "Denied note", Body: "Denied"}, bob); createErr == nil {
		t.Fatal("ProjectNoteCreate() accepted an ungranted principal")
	}
	title, description, body := "Updated guide", "Updated description", "# Updated guide"
	err, updated := store.ProjectNoteDetailsSet(ctx, older.Ref, alice, &title, &description, &body)
	if err != nil || updated == nil || updated.Note.AuthorPrincipal != alice || updated.Note.Title != title || updated.Note.Description != description || updated.Note.Body != body {
		t.Fatalf("ProjectNoteDetailsSet() = (%#v, %v)", updated, err)
	}
	err, removed := store.ProjectNoteRemove(ctx, newer.Ref, bob)
	if err != nil || removed {
		t.Fatalf("ProjectNoteRemove() for ungranted principal = (%t, %v)", removed, err)
	}
	err, removed = store.ProjectNoteRemove(ctx, newer.Ref, alice)
	if err != nil || !removed {
		t.Fatalf("ProjectNoteRemove() = (%t, %v)", removed, err)
	}
	err, hidden = store.ProjectNoteGet(ctx, newer.Ref, alice)
	if err != nil || hidden != nil {
		t.Fatalf("ProjectNoteGet() after removal = (%#v, %v)", hidden, err)
	}
	err, notes = store.ProjectNotesGet(ctx, project, alice)
	if err != nil || len(notes) != 2 || notes[0].Ref.Id != sensitive.Ref.Id || !notes[0].Sensitive || notes[1].Ref.Id != older.Ref.Id {
		t.Fatalf("ProjectNotesGet() after removal = (%#v, %v)", notes, err)
	}
	oversizedID, err := typed_id.NewAt(typed_id.ProjectNote, time.Date(2026, 1, 2, 3, 4, 7, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if createErr, _ := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: oversizedID}, Title: "Oversized", Description: "Oversized body", Body: strings.Repeat("x", 1024*1024+1)}, alice); createErr == nil {
		t.Fatal("ProjectNoteCreate() accepted a body exceeding 1 MiB")
	}
	emptyID, err := typed_id.NewAt(typed_id.ProjectNote, time.Date(2026, 1, 2, 3, 4, 8, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, empty := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: emptyID}, Title: "Empty"}, alice)
	if err != nil || empty.Description != "" || empty.Body != "" {
		t.Fatalf("ProjectNoteCreate() with empty optional fields = (%#v, %v)", empty, err)
	}
	for _, event := range []string{"project_note.create", "project_note.update", "project_note.remove"} {
		var count int
		if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE workspace = ? AND project = ? AND event = ?`, workspace.Id, project.Id, event).Scan(&count); err != nil || count == 0 {
			t.Fatalf("project note activity %q = (%d, %v)", event, count, err)
		}
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
		Groups:     []config.Group{{WorkspaceID: "engineering", Alias: "reviewers", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	reviewers := model.GroupRef{Workspace: workspace, Id: groupID(t, ctx, store, "engineering", "reviewers")}
	alice := principalRef(t, ctx, store, "alice")
	bob := principalRef(t, ctx, store, "bob")
	projectID, err := typed_id.New(typed_id.Project)
	if err != nil {
		t.Fatal(err)
	}
	projectRef := model.ProjectRef{Workspace: workspace, Id: projectID}
	err, project := store.ProjectsCreate(ctx, model.Project{Ref: projectRef, Enabled: true}, alice, []model.GroupRef{reviewers})
	if err != nil {
		t.Fatal(err)
	}
	if project.CreatedAt == "" {
		t.Fatal("ProjectsCreate() did not derive a creation timestamp")
	}
	if project.Name != nil {
		t.Fatalf("ProjectsCreate() name = %q, want nil", *project.Name)
	}
	var managerBindings int
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_project_grants
		WHERE workspace = ? AND project = ? AND role = 'manager'
			AND (principal = ? OR "group" = ?)
	`, workspace.Id, projectRef.Id, alice.Id, reviewers.Id).Scan(&managerBindings); err != nil {
		t.Fatal(err)
	}
	if managerBindings != 2 {
		t.Fatalf("project manager grants = %d, want 2", managerBindings)
	}
	err, projects := store.ProjectsGet(ctx, workspace, alice)
	if err != nil || len(projects) != 1 || projects[0].Ref != projectRef {
		t.Fatalf("ProjectsGet() for creator = (%#v, %v)", projects, err)
	}
	err, projects = store.ProjectsGet(ctx, workspace, bob)
	if err != nil || len(projects) != 0 {
		t.Fatalf("ProjectsGet() for ungranted principal = (%#v, %v)", projects, err)
	}
	name := "  Design system  "
	description := "  Shared components and guidelines.  "
	err, updated := store.ProjectDetailsSet(ctx, projectRef, alice, &name, &description)
	if err != nil || updated == nil || updated.Name == nil || *updated.Name != "Design system" || updated.Description == nil || *updated.Description != "Shared components and guidelines." {
		t.Fatalf("ProjectDetailsSet() = (%#v, %v)", updated, err)
	}
	blank := "  "
	err, updated = store.ProjectDetailsSet(ctx, projectRef, alice, &blank, &blank)
	if err != nil || updated == nil || updated.Name != nil || updated.Description != nil {
		t.Fatalf("ProjectDetailsSet() blank values = (%#v, %v)", updated, err)
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
		INSERT INTO gatehouse_project_grants (workspace, project, role, principal, "group", enabled) VALUES
			(?, 'prj_00000000000000000000000000', 'member', ?, NULL, TRUE),
			(?, 'prj_00000000000000000000000001', 'member', ?, NULL, TRUE),
			(?, 'prj_00000000000000000000000002', 'member', ?, NULL, TRUE)
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
