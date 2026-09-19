package database_test

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"gatehouse/authz"
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
				ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic LIKE ?
		`, event, database.ActivityTopicProject(project)+"/%").Scan(&got); err != nil {
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
	newer, newerObjectID := create(time.Date(2026, 1, 2, 3, 4, 6, 678_000_000, time.UTC), "design.pdf")
	err, files := store.ProjectFilesGet(ctx, project, alice)
	if err != nil || len(files) != 3 || files[0].ID != newer.Id || files[0].Name != "design.pdf" || files[0].MediaType == nil || *files[0].MediaType != "application/octet-stream" || files[0].Size != int64(len("design.pdf")) || files[0].Fingerprint != "sha256:2699a8a0c49f286591802474c202cf177fb2b9de754315de6051485c5d30f10f" || files[1].ID != older.Id || files[2].ID != pending.Ref.Id {
		t.Fatalf("ProjectFilesGet() = (%#v, %v)", files, err)
	}
	err, updated, object := store.ProjectFileUpdate(ctx, newer, database.FileUpdate{Name: "  design-final.pdf  "}, alice)
	if err != nil || updated == nil || object == nil || updated.Ref != newer || updated.Name != "design-final.pdf" || updated.StorageObject.Id != newerObjectID || object.ID != newerObjectID || object.Size != int64(len("design.pdf")) || object.State != "success" {
		t.Fatalf("ProjectFileUpdate() = (%#v, %#v, %v)", updated, object, err)
	}
	activity("project_file.update", 2)
	err, files = store.ProjectFilesGet(ctx, project, alice)
	if err != nil || len(files) != 3 || files[0].ID != newer.Id || files[0].Name != "design-final.pdf" || files[0].Fingerprint != "sha256:2699a8a0c49f286591802474c202cf177fb2b9de754315de6051485c5d30f10f" {
		t.Fatalf("ProjectFilesGet() after update = (%#v, %v)", files, err)
	}
	memberGrantID, err := typed_id.New(typed_id.ProjectGrant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_project_grants (id, workspace, project, role, principal, "group", enabled)
		VALUES (?, ?, ?, ?, ?, NULL, TRUE)
	`, memberGrantID, workspace.Id, project.Id, authz.Member, bob.Id); err != nil {
		t.Fatal(err)
	}
	if err, visible, object := store.ProjectFileGet(ctx, newer, bob); err != nil || visible == nil || object == nil {
		t.Fatalf("ProjectFileGet() for member = (%#v, %#v, %v)", visible, object, err)
	}
	if err, updated, object := store.ProjectFileUpdate(ctx, newer, database.FileUpdate{Name: "private.pdf"}, bob); err == nil || updated != nil || object != nil {
		t.Fatalf("ProjectFileUpdate() for member = (%#v, %#v, %v)", updated, object, err)
	}
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_project_grants SET enabled = FALSE WHERE id = ?`, memberGrantID); err != nil {
		t.Fatal(err)
	}
	if err, _, _ := store.ProjectFileUpdate(ctx, newer, database.FileUpdate{Name: "  "}, alice); err == nil {
		t.Fatal("ProjectFileUpdate() accepted a blank name")
	}
	if err, updated, object := store.ProjectFileUpdate(ctx, newer, database.FileUpdate{Name: "design-final.pdf"}, alice); err != nil || updated == nil || object == nil {
		t.Fatalf("ProjectFileUpdate() no-op = (%#v, %#v, %v)", updated, object, err)
	}
	activity("project_file.update", 2)
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
	if err, updated, object := store.ProjectFileUpdate(ctx, newer, database.FileUpdate{Name: "restored.pdf"}, alice); err != nil || updated != nil || object != nil {
		t.Fatalf("ProjectFileUpdate() after removal = (%#v, %#v, %v)", updated, object, err)
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
		err, stored := store.ProjectNoteCreate(ctx, model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: id}, Title: "  " + title + "  ", Description: "  Description for " + title + ".  ", Body: "# " + title}, alice)
		if err != nil || stored.AuthorPrincipal == nil || *stored.AuthorPrincipal != alice || stored.Revision != 1 || stored.Title != title || stored.Description != "Description for "+title+"." || stored.Sensitive || stored.CreatedAt != at.Format("2006-01-02T15:04:05.000Z") {
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
	gateway := model.GatewayRef{Id: "gwy_00000000000000000000000000"}
	title, description, body := "Published credentials guide", "Public replacement", "# Public"
	err, sensitiveDetail = store.ProjectNoteDetailsSetAs(ctx, sensitive.Ref, alice, database.NoteAuthor{Gateway: &gateway}, false, &title, &description, &body)
	if err != nil || sensitiveDetail == nil || sensitiveDetail.Note.Sensitive || sensitiveDetail.Note.AuthorGateway == nil || *sensitiveDetail.Note.AuthorGateway != gateway || sensitiveDetail.Note.Revision != 2 {
		t.Fatalf("ProjectNoteDetailsSetAs() changes current sensitivity and author = (%#v, %v)", sensitiveDetail, err)
	}
	err, sensitiveRevisions := store.ProjectNoteRevisionsGet(ctx, sensitive.Ref, alice)
	if err != nil || len(sensitiveRevisions) != 2 || sensitiveRevisions[0].Ref.Revision != 2 || sensitiveRevisions[0].Sensitive || sensitiveRevisions[0].AuthorGateway == nil || *sensitiveRevisions[0].AuthorGateway != gateway || sensitiveRevisions[1].Ref.Revision != 1 || !sensitiveRevisions[1].Sensitive {
		t.Fatalf("ProjectNoteRevisionsGet() preserves revision sensitivity and authors = (%#v, %v)", sensitiveRevisions, err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_project_note_revisions (workspace, project, note, revision, author_agent, title, description, body, sensitive, created_at) VALUES (?, ?, ?, 3, ?, 'Agent revision', 'Agent revision', 'Agent revision', FALSE, '2026-01-01T00:00:00.000Z')`, workspace.Id, project.Id, sensitive.Ref.Id, agent.Id); err != nil {
		t.Fatal(err)
	}
	err, agentRevisions := store.ProjectNoteRevisionsGet(ctx, sensitive.Ref, alice)
	if err != nil || len(agentRevisions) != 3 || agentRevisions[0].AuthorAgent == nil || *agentRevisions[0].AuthorAgent != agent || agentRevisions[0].AuthorAgentLabel == nil || *agentRevisions[0].AuthorAgentLabel != "Assistant" {
		t.Fatalf("ProjectNoteRevisionsGet() resolves agent labels = (%#v, %v)", agentRevisions, err)
	}
	err, agentRevision := store.ProjectNoteRevisionGet(ctx, model.ProjectNoteRevisionRef{Note: sensitive.Ref, Revision: 3}, alice)
	if err != nil || agentRevision == nil || agentRevision.Revision.AuthorAgent == nil || *agentRevision.Revision.AuthorAgent != agent || agentRevision.AuthorAgentLabel == nil || *agentRevision.AuthorAgentLabel != "Assistant" {
		t.Fatalf("ProjectNoteRevisionGet() resolves agent labels = (%#v, %v)", agentRevision, err)
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
	title, description, body = "Updated guide", "Updated description", "# Updated guide"
	err, updated := store.ProjectNoteDetailsSetAs(ctx, older.Ref, alice, database.NoteAuthor{Principal: &alice}, false, &title, &description, &body)
	if err != nil || updated == nil || updated.Note.AuthorPrincipal == nil || *updated.Note.AuthorPrincipal != alice || updated.Note.Revision != 2 || updated.Note.Title != title || updated.Note.Description != description || updated.Note.Body != body {
		t.Fatalf("ProjectNoteDetailsSetAs() = (%#v, %v)", updated, err)
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
	if err != nil || len(notes) != 2 || notes[0].Ref.Id != sensitive.Ref.Id || notes[0].Sensitive || notes[1].Ref.Id != older.Ref.Id {
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
	for event, topic := range map[string]string{
		"project_note.create": database.ActivityTopicProjectNote(older.Ref),
		"project_note.update": database.ActivityTopicProjectNote(sensitive.Ref),
		"project_note.remove": database.ActivityTopicProjectNote(newer.Ref),
	} {
		var count int
		if err := store.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic = ?
		`, event, topic).Scan(&count); err != nil || count == 0 {
			t.Fatalf("project note activity %q = (%d, %v)", event, count, err)
		}
	}
}

func TestProjectTasksUseProjectAuthorizationAndActivity(t *testing.T) {
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
	olderID, err := typed_id.NewAt(typed_id.ProjectTask, time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, older := store.ProjectTaskCreate(ctx, model.ProjectTask{Ref: model.ProjectTaskRef{Project: project, Id: olderID}, Title: "  Implement tasks  ", Description: stringPointer("  Backend work  "), Sensitive: true}, alice)
	if err != nil || older.Status != "draft" || older.CreatorPrincipal == nil || *older.CreatorPrincipal != alice || older.UpdaterPrincipal == nil || *older.UpdaterPrincipal != alice || older.Description == nil || *older.Description != "Backend work" {
		t.Fatalf("ProjectTaskCreate() = (%#v, %v)", older, err)
	}
	newerID, err := typed_id.NewAt(typed_id.ProjectTask, time.Date(2026, 1, 2, 3, 4, 6, 678_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, newer := store.ProjectTaskCreate(ctx, model.ProjectTask{Ref: model.ProjectTaskRef{Project: project, Id: newerID}, Title: "Review", Status: "ready"}, alice)
	if err != nil {
		t.Fatal(err)
	}
	err, tasks := store.ProjectTasksGet(ctx, project, alice)
	if err != nil || len(tasks) != 2 || tasks[0].Ref != newer.Ref || tasks[0].Description != nil || tasks[1].Description != nil {
		t.Fatalf("ProjectTasksGet() = (%#v, %v)", tasks, err)
	}
	err, detail := store.ProjectTaskGet(ctx, older.Ref, alice)
	if err != nil || detail == nil || detail.Description == nil || *detail.Description != "Backend work" {
		t.Fatalf("ProjectTaskGet() = (%#v, %v)", detail, err)
	}
	err, hidden := store.ProjectTaskGet(ctx, older.Ref, bob)
	if err != nil || hidden != nil {
		t.Fatalf("ProjectTaskGet() for ungranted principal = (%#v, %v)", hidden, err)
	}
	if createErr, _ := store.ProjectTaskCreate(ctx, model.ProjectTask{Ref: model.ProjectTaskRef{Project: project, Id: newerID}, Title: "Denied"}, bob); createErr == nil {
		t.Fatal("ProjectTaskCreate() accepted an ungranted principal")
	}
	err, updated := store.ProjectTaskDetailsSetAs(ctx, older.Ref, alice, database.TaskAuthor{Gateway: &model.GatewayRef{Id: "gwy_00000000000000000000000000"}}, false, "done", "Implemented", nil)
	if err != nil || updated == nil || updated.Status != "done" || updated.Description != nil || updated.UpdaterGateway == nil {
		t.Fatalf("ProjectTaskDetailsSetAs() = (%#v, %v)", updated, err)
	}
	err, tasks = store.ProjectTasksGet(ctx, project, alice)
	if err != nil || len(tasks) != 2 || tasks[0].Ref != newer.Ref {
		t.Fatalf("ProjectTasksGet() after update = (%#v, %v)", tasks, err)
	}
	err, removed := store.ProjectTaskRemove(ctx, newer.Ref, bob)
	if err != nil || removed {
		t.Fatalf("ProjectTaskRemove() for ungranted principal = (%t, %v)", removed, err)
	}
	err, removed = store.ProjectTaskRemove(ctx, newer.Ref, alice)
	if err != nil || !removed {
		t.Fatalf("ProjectTaskRemove() = (%t, %v)", removed, err)
	}
	err, hidden = store.ProjectTaskGet(ctx, newer.Ref, alice)
	if err != nil || hidden != nil {
		t.Fatalf("ProjectTaskGet() after removal = (%#v, %v)", hidden, err)
	}
	for event, task := range map[string]model.ProjectTaskRef{"project_task.create": older.Ref, "project_task.update": older.Ref, "project_task.remove": newer.Ref} {
		var count int
		if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events AS events JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id WHERE events.event = ? AND events.resource_project_task = ? AND topics.topic = ?`, event, task.Id, database.ActivityTopicProjectTask(task)).Scan(&count); err != nil || count != 1 {
			t.Fatalf("project task activity %q = (%d, %v)", event, count, err)
		}
	}
}

func TestProjectRecordsUseTypedValuesAndHardDeletion(t *testing.T) {
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
	projectID, err := typed_id.NewAt(typed_id.Project, time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	project := model.ProjectRef{Workspace: workspace, Id: projectID}
	if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, alice, nil); err != nil {
		t.Fatal(err)
	}
	schemaID, err := typed_id.NewAt(typed_id.ProjectRecordSchema, time.Date(2026, 1, 2, 3, 4, 1, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	schema := model.ProjectRecordSchema{Ref: model.ProjectRecordSchemaRef{Project: project, Id: schemaID}, Name: "contacts", Label: "Contacts"}
	err, schema = store.ProjectRecordSchemaCreate(ctx, schema, alice, database.ProjectRecordAuthor{})
	if err != nil || schema.CreatedAt != "2026-01-02T03:04:01.000Z" || schema.AuthorPrincipal == nil || *schema.AuthorPrincipal != alice {
		t.Fatalf("ProjectRecordSchemaCreate() = (%#v, %v)", schema, err)
	}
	emailID, err := typed_id.NewAt(typed_id.ProjectRecordAttribute, time.Date(2026, 1, 2, 3, 4, 2, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	email := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: emailID}, Name: "zulu_email", Label: "Email", Type: "text", Cardinality: "one", Uniqueness: "global", Display: "primary", DisplayOrder: 20}
	err, email = store.ProjectRecordAttributeCreate(ctx, email, alice, database.ProjectRecordAuthor{})
	if err != nil || email.DisplayOrder != 20 {
		t.Fatalf("ProjectRecordAttributeCreate() email = (%#v, %v)", email, err)
	}
	alphaNameID, err := typed_id.NewAt(typed_id.ProjectRecordAttribute, time.Date(2026, 1, 2, 3, 4, 2, 1_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	alphaName := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: alphaNameID}, Name: "alpha_name", Label: "Alpha name", Type: "text", Cardinality: "one", Uniqueness: "none", Display: "primary", DisplayOrder: 20}
	err, alphaName = store.ProjectRecordAttributeCreate(ctx, alphaName, alice, database.ProjectRecordAuthor{})
	if err != nil || alphaName.DisplayOrder != 20 {
		t.Fatalf("ProjectRecordAttributeCreate() alpha name = (%#v, %v)", alphaName, err)
	}
	middleNameID, err := typed_id.NewAt(typed_id.ProjectRecordAttribute, time.Date(2026, 1, 2, 3, 4, 2, 2_000_000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	middleName := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: middleNameID}, Name: "middle_name", Label: "Middle name", Type: "text", Cardinality: "one", Uniqueness: "none", Display: "primary", DisplayOrder: 10}
	err, middleName = store.ProjectRecordAttributeCreate(ctx, middleName, alice, database.ProjectRecordAuthor{})
	if err != nil || middleName.DisplayOrder != 10 {
		t.Fatalf("ProjectRecordAttributeCreate() middle name = (%#v, %v)", middleName, err)
	}
	invalidDisplayOrder := email
	invalidDisplayOrder.DisplayOrder = -1
	if err, _ := store.ProjectRecordAttributeCreate(ctx, invalidDisplayOrder, alice, database.ProjectRecordAuthor{}); err == nil {
		t.Fatal("ProjectRecordAttributeCreate() accepted a negative display order")
	}
	if err, _ := store.ProjectRecordAttributeSetAs(ctx, invalidDisplayOrder, alice, database.ProjectRecordAuthor{}); err == nil {
		t.Fatal("ProjectRecordAttributeSetAs() accepted a negative display order")
	}
	err, storedEmail := store.ProjectRecordAttributeGet(ctx, email.Ref, alice)
	if err != nil || storedEmail == nil || storedEmail.DisplayOrder != 20 {
		t.Fatalf("ProjectRecordAttributeGet() display order = (%#v, %v)", storedEmail, err)
	}
	err, attributes := store.ProjectRecordAttributesGet(ctx, schema.Ref, alice)
	if err != nil || len(attributes) != 3 || attributes[0].Ref != middleName.Ref || attributes[0].DisplayOrder != 10 || attributes[1].Ref != alphaName.Ref || attributes[1].DisplayOrder != 20 || attributes[2].Ref != email.Ref || attributes[2].DisplayOrder != 20 {
		t.Fatalf("ProjectRecordAttributesGet() display ordering = (%#v, %v)", attributes, err)
	}
	recordID, err := typed_id.NewAt(typed_id.ProjectRecord, time.Date(2026, 1, 2, 3, 4, 3, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	record := model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: recordID}}
	err, record, values := store.ProjectRecordCreate(ctx, record, alice, database.ProjectRecordAuthor{}, []database.ProjectRecordValueCreate{{Attribute: email.Ref, Value: "ada@example.test"}, {Attribute: alphaName.Ref, Value: "Ada"}, {Attribute: middleName.Ref, Value: "Lovelace"}})
	if err != nil || len(values) != 3 || values[0].Value != "ada@example.test" || values[0].AuthorPrincipal == nil || *values[0].AuthorPrincipal != alice {
		t.Fatalf("ProjectRecordCreate() = (%#v, %#v, %v)", record, values, err)
	}
	duplicateID, err := typed_id.NewAt(typed_id.ProjectRecord, time.Date(2026, 1, 2, 3, 4, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err, _, _ := store.ProjectRecordCreate(ctx, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: duplicateID}}, alice, database.ProjectRecordAuthor{}, []database.ProjectRecordValueCreate{{Attribute: email.Ref, Value: "ada@example.test"}}); err == nil {
		t.Fatal("ProjectRecordCreate() accepted a duplicate globally unique value")
	}
	relatedID, err := typed_id.NewAt(typed_id.ProjectRecordAttribute, time.Date(2026, 1, 2, 3, 4, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	related := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: relatedID}, Name: "related", Label: "Related", Type: "record", TargetSchema: &schema.Ref, Cardinality: "one", Uniqueness: "none", Display: "primary"}
	err, related = store.ProjectRecordAttributeCreate(ctx, related, alice, database.ProjectRecordAuthor{})
	if err != nil {
		t.Fatal(err)
	}
	referencingID, err := typed_id.NewAt(typed_id.ProjectRecord, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err, _, _ = store.ProjectRecordCreate(ctx, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: referencingID}}, alice, database.ProjectRecordAuthor{}, []database.ProjectRecordValueCreate{{Attribute: related.Ref, Value: record.Ref.Id}})
	if err != nil {
		t.Fatalf("ProjectRecordCreate() with a record value: %v", err)
	}
	err, cards := store.ProjectRecordCardsGet(ctx, schema.Ref, alice, 100, "")
	if err != nil || len(cards) != 2 || cards[0].Record.Ref.Id != referencingID || len(cards[0].Values) != 1 || cards[0].Values[0].Reference == nil || cards[0].Values[0].Reference.SchemaLabel != "Contacts" || len(cards[0].Values[0].Reference.PrimaryValues) != 3 || cards[0].Values[0].Reference.PrimaryValues[0].Value != "Lovelace" || cards[0].Values[0].Reference.PrimaryValues[1].Value != "Ada" || cards[0].Values[0].Reference.PrimaryValues[2].Value != "ada@example.test" || cards[1].Record.Ref.Id != recordID || len(cards[1].Values) != 3 || cards[1].Values[0].Attribute != middleName.Ref.Id || cards[1].Values[1].Attribute != alphaName.Ref.Id || cards[1].Values[2].Attribute != email.Ref.Id {
		t.Fatalf("ProjectRecordCardsGet() = (%#v, %v)", cards, err)
	}
	relatedManyID, err := typed_id.NewAt(typed_id.ProjectRecordAttribute, time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	relatedMany := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: relatedManyID}, Name: "related_many", Label: "Related many", Type: "record", TargetSchema: &schema.Ref, Cardinality: "many", Uniqueness: "none", Display: "secondary"}
	err, relatedMany = store.ProjectRecordAttributeCreate(ctx, relatedMany, alice, database.ProjectRecordAuthor{})
	if err != nil {
		t.Fatal(err)
	}
	err, _ = store.ProjectRecordValuesMutate(ctx, model.ProjectRecordRef{Schema: schema.Ref, Id: referencingID}, alice, database.ProjectRecordAuthor{}, database.ProjectRecordValuesMutation{Create: []database.ProjectRecordValueCreate{{Attribute: relatedMany.Ref, Value: record.Ref.Id}, {Attribute: relatedMany.Ref, Value: record.Ref.Id}}})
	if err != nil {
		t.Fatalf("ProjectRecordValuesMutate() related many = %v", err)
	}
	err, incoming := store.ProjectRecordIncomingReferencesGet(ctx, record.Ref, alice, 100, "")
	if err != nil || len(incoming) != 2 {
		t.Fatalf("ProjectRecordIncomingReferencesGet() = (%#v, %v)", incoming, err)
	}
	var many *database.ProjectRecordIncomingReferenceGroup
	var relatedGroup *database.ProjectRecordIncomingReferenceGroup
	for index := range incoming {
		if incoming[index].Attribute.Id == relatedMany.Ref.Id {
			many = &incoming[index]
		}
		if incoming[index].Attribute.Id == related.Ref.Id {
			relatedGroup = &incoming[index]
		}
	}
	if relatedGroup == nil || len(relatedGroup.References) != 1 || len(relatedGroup.References[0].PrimaryValues) != 0 {
		t.Fatalf("ProjectRecordIncomingReferencesGet() excludes grouping attribute = %#v", relatedGroup)
	}
	if many == nil || many.SchemaLabel != "Contacts" || many.AttributeLabel != "Related many" || len(many.References) != 2 || many.References[0].Record.Id != referencingID || many.References[1].Record.Id != referencingID {
		t.Fatalf("ProjectRecordIncomingReferencesGet() many group = %#v", many)
	}
	targetSchemaID, err := typed_id.NewAt(typed_id.ProjectRecordSchema, time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	targetSchema := model.ProjectRecordSchema{Ref: model.ProjectRecordSchemaRef{Project: project, Id: targetSchemaID}, Name: "organizations", Label: "Organizations"}
	err, targetSchema = store.ProjectRecordSchemaCreate(ctx, targetSchema, alice, database.ProjectRecordAuthor{})
	if err != nil {
		t.Fatalf("ProjectRecordSchemaCreate() target = %v", err)
	}
	targetRecordID, err := typed_id.NewAt(typed_id.ProjectRecord, time.Date(2026, 1, 2, 3, 4, 8, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	targetRecord := model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: targetSchema.Ref, Id: targetRecordID}}
	err, targetRecord, _ = store.ProjectRecordCreate(ctx, targetRecord, alice, database.ProjectRecordAuthor{}, nil)
	if err != nil {
		t.Fatalf("ProjectRecordCreate() target = %v", err)
	}
	targetAttributeID, err := typed_id.NewAt(typed_id.ProjectRecordAttribute, time.Date(2026, 1, 2, 3, 4, 9, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	targetAttribute := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: targetAttributeID}, Name: "organization", Label: "Organization", Type: "record", TargetSchema: &targetSchema.Ref, Cardinality: "one", Uniqueness: "none", Display: "secondary"}
	err, targetAttribute = store.ProjectRecordAttributeCreate(ctx, targetAttribute, alice, database.ProjectRecordAuthor{})
	if err != nil {
		t.Fatalf("ProjectRecordAttributeCreate() target = %v", err)
	}
	err, _ = store.ProjectRecordValuesMutate(ctx, model.ProjectRecordRef{Schema: schema.Ref, Id: referencingID}, alice, database.ProjectRecordAuthor{}, database.ProjectRecordValuesMutation{Create: []database.ProjectRecordValueCreate{{Attribute: targetAttribute.Ref, Value: targetRecord.Ref.Id}}})
	if err != nil {
		t.Fatalf("ProjectRecordValuesMutate() cross-schema reference = %v", err)
	}
	err, incomingTarget := store.ProjectRecordIncomingReferencesGet(ctx, targetRecord.Ref, alice, 100, "")
	if err != nil || len(incomingTarget) != 1 || incomingTarget[0].Attribute != targetAttribute.Ref || len(incomingTarget[0].References) != 1 || incomingTarget[0].References[0].Record.Id != referencingID {
		t.Fatalf("ProjectRecordIncomingReferencesGet() cross-schema = (%#v, %v)", incomingTarget, err)
	}
	if err, removed := store.ProjectRecordRemove(ctx, targetRecord.Ref, alice); err == nil || removed {
		t.Fatal("ProjectRecordRemove() removed a cross-schema referenced record")
	}
	updatedRelated := related
	updatedRelated.Uniqueness = "global"
	err, updated := store.ProjectRecordAttributeSetAs(ctx, updatedRelated, alice, database.ProjectRecordAuthor{})
	if err != nil || updated == nil || updated.Uniqueness != "global" {
		t.Fatalf("ProjectRecordAttributeSetAs() uniqueness = (%#v, %v)", updated, err)
	}
	related = *updated
	invalidRelatedMany := relatedMany
	invalidRelatedMany.Cardinality = "one"
	if err, _ := store.ProjectRecordAttributeSetAs(ctx, invalidRelatedMany, alice, database.ProjectRecordAuthor{}); err == nil {
		t.Fatal("ProjectRecordAttributeSetAs() accepted one cardinality with duplicate values")
	}
	err, currentRelatedMany := store.ProjectRecordAttributeGet(ctx, relatedMany.Ref, alice)
	if err != nil || currentRelatedMany == nil || currentRelatedMany.Cardinality != "many" {
		t.Fatalf("ProjectRecordAttributeGet() after rejected cardinality = (%#v, %v)", currentRelatedMany, err)
	}
	err, mutation := store.ProjectRecordValuesMutate(ctx, record.Ref, alice, database.ProjectRecordAuthor{}, database.ProjectRecordValuesMutation{Update: []database.ProjectRecordValueUpdate{{ID: values[0].Ref.Id, Value: "ada.lovelace@example.test"}}})
	if err != nil || len(mutation.Created) != 1 || mutation.Created[0].Ref.Id == values[0].Ref.Id || len(mutation.Removed) != 1 || mutation.Removed[0] != values[0].Ref.Id {
		t.Fatalf("ProjectRecordValuesMutate() = (%#v, %v)", mutation, err)
	}
	if err, removed := store.ProjectRecordAttributeRemove(ctx, email.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordAttributeRemove() with values = (%t, %v)", removed, err)
	}
	err, remainingValues := store.ProjectRecordValuesGet(ctx, record.Ref, alice, 100, "")
	if err != nil || len(remainingValues) != 2 || !((remainingValues[0].Attribute == middleName.Ref && remainingValues[1].Attribute == alphaName.Ref) || (remainingValues[0].Attribute == alphaName.Ref && remainingValues[1].Attribute == middleName.Ref)) {
		t.Fatalf("ProjectRecordValuesGet() after attribute removal = (%#v, %v)", remainingValues, err)
	}
	if err, removed := store.ProjectRecordSchemaRemove(ctx, schema.Ref, alice); err == nil || removed {
		t.Fatal("ProjectRecordSchemaRemove() removed a schema with records")
	}
	if err, removed := store.ProjectRecordRemove(ctx, record.Ref, alice); err == nil || removed {
		t.Fatal("ProjectRecordRemove() removed a referenced record")
	}
	if err, removed := store.ProjectRecordRemove(ctx, model.ProjectRecordRef{Schema: schema.Ref, Id: referencingID}, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordRemove() referring record = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordRemove(ctx, targetRecord.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordRemove() target record = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordRemove(ctx, record.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordRemove() = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordAttributeRemove(ctx, related.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordAttributeRemove() after values = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordAttributeRemove(ctx, relatedMany.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordAttributeRemove() related many = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordAttributeRemove(ctx, targetAttribute.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordAttributeRemove() target = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordAttributeRemove(ctx, alphaName.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordAttributeRemove() alpha name = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordAttributeRemove(ctx, middleName.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordAttributeRemove() middle name = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordSchemaRemove(ctx, schema.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordSchemaRemove() after records = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectRecordSchemaRemove(ctx, targetSchema.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordSchemaRemove() target = (%t, %v)", removed, err)
	}
	var updates int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'project_record.update' AND resource_project_record = ?`, record.Ref.Id).Scan(&updates); err != nil || updates != 1 {
		t.Fatalf("project record update activity = (%d, %v)", updates, err)
	}
}

func TestProjectRecordFileValuesRequirePublishedFiles(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:                []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:                []config.Principal{{Alias: "alice", Enabled: true}},
		StorageProviders:          []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	newID := func(kind string) string {
		id, err := typed_id.New(kind)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	createProject := func() model.ProjectRef {
		id, err := typed_id.New(typed_id.Project)
		if err != nil {
			t.Fatal(err)
		}
		project := model.ProjectRef{Workspace: workspace, Id: id}
		if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, alice, nil); err != nil {
			t.Fatal(err)
		}
		return project
	}
	createFile := func(project model.ProjectRef, name string, published bool) model.ProjectFileRef {
		fileID, err := typed_id.New(typed_id.ProjectFile)
		if err != nil {
			t.Fatal(err)
		}
		objectID, err := typed_id.New(typed_id.StorageObject)
		if err != nil {
			t.Fatal(err)
		}
		file := model.ProjectFile{Ref: model.ProjectFileRef{Project: project, Id: fileID}, Name: name, Enabled: true}
		if err, _, _ := store.ProjectFileCreate(ctx, file, objectID, alice); err != nil {
			t.Fatal(err)
		}
		if published {
			digest := sha256.Sum256([]byte(name))
			if err := store.StorageObjectStoreIntegrity(ctx, objectID, digest[:], int64(len(name))); err != nil {
				t.Fatal(err)
			}
			if err := store.StorageObjectMarkSuccess(ctx, objectID); err != nil {
				t.Fatal(err)
			}
		}
		return file.Ref
	}
	project := createProject()
	schemaID, err := typed_id.New(typed_id.ProjectRecordSchema)
	if err != nil {
		t.Fatal(err)
	}
	schema := model.ProjectRecordSchema{Ref: model.ProjectRecordSchemaRef{Project: project, Id: schemaID}, Name: "documents", Label: "Documents"}
	err, schema = store.ProjectRecordSchemaCreate(ctx, schema, alice, database.ProjectRecordAuthor{})
	if err != nil {
		t.Fatal(err)
	}
	attributeID, err := typed_id.New(typed_id.ProjectRecordAttribute)
	if err != nil {
		t.Fatal(err)
	}
	attachment := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema.Ref, Id: attributeID}, Name: "attachment", Label: "Attachment", Type: "file", Cardinality: "one", Uniqueness: "none", Display: "secondary"}
	err, attachment = store.ProjectRecordAttributeCreate(ctx, attachment, alice, database.ProjectRecordAuthor{})
	if err != nil || attachment.TargetSchema != nil {
		t.Fatalf("ProjectRecordAttributeCreate() file = (%#v, %v)", attachment, err)
	}
	published := createFile(project, "design.pdf", true)
	err, record, values := store.ProjectRecordCreate(ctx, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: newID(typed_id.ProjectRecord)}}, alice, database.ProjectRecordAuthor{}, []database.ProjectRecordValueCreate{{Attribute: attachment.Ref, Value: published.Id}})
	if err != nil || len(values) != 1 || values[0].Value != published.Id {
		t.Fatalf("ProjectRecordCreate() file = (%#v, %#v, %v)", record, values, err)
	}
	err, displays := store.ProjectRecordValueFileReferenceDisplaysGet(ctx, record.Ref, alice, values)
	if err != nil || displays[values[0].Ref.Id] == nil || displays[values[0].Ref.Id].Name != "design.pdf" {
		t.Fatalf("ProjectRecordValueFileReferenceDisplaysGet() = (%#v, %v)", displays, err)
	}
	if err, removed := store.ProjectFileRemove(ctx, published, alice); err == nil || removed {
		t.Fatal("ProjectFileRemove() removed a referenced file")
	}
	pending := createFile(project, "pending.pdf", false)
	if err, _, _ := store.ProjectRecordCreate(ctx, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: newID(typed_id.ProjectRecord)}}, alice, database.ProjectRecordAuthor{}, []database.ProjectRecordValueCreate{{Attribute: attachment.Ref, Value: pending.Id}}); err == nil {
		t.Fatal("ProjectRecordCreate() accepted a pending file")
	}
	otherProject := createProject()
	foreign := createFile(otherProject, "other.pdf", true)
	if err, _, _ := store.ProjectRecordCreate(ctx, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: newID(typed_id.ProjectRecord)}}, alice, database.ProjectRecordAuthor{}, []database.ProjectRecordValueCreate{{Attribute: attachment.Ref, Value: foreign.Id}}); err == nil {
		t.Fatal("ProjectRecordCreate() accepted a cross-project file")
	}
	if err, removed := store.ProjectRecordRemove(ctx, record.Ref, alice); err != nil || !removed {
		t.Fatalf("ProjectRecordRemove() file reference = (%t, %v)", removed, err)
	}
	if err, removed := store.ProjectFileRemove(ctx, published, alice); err != nil || !removed {
		t.Fatalf("ProjectFileRemove() after reference removal = (%t, %v)", removed, err)
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
		WorkspaceGrants: []config.WorkspaceGrant{{
			WorkspaceID: "engineering", Role: "member", PrincipalID: stringPointer("alice"), Revision: 1, Enabled: true,
		}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	reviewers := model.GroupRef{Id: groupID(t, ctx, store, "engineering", "reviewers")}
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
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_workspaces SET enabled = FALSE WHERE id = ?`, workspace.Id); err != nil {
		t.Fatal(err)
	}
	err, unavailable := store.ProjectGet(ctx, projectRef, alice)
	if err != nil || unavailable != nil {
		t.Fatalf("ProjectGet() for disabled workspace = (%#v, %v)", unavailable, err)
	}
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_workspaces SET enabled = TRUE WHERE id = ?`, workspace.Id); err != nil {
		t.Fatal(err)
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
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_projects SET enabled = FALSE WHERE workspace = ? AND id = ?`, workspace.Id, projectRef.Id); err != nil {
		t.Fatal(err)
	}
	err, unavailableSession := store.SessionGet(ctx, session, alice)
	if err != nil || unavailableSession != nil {
		t.Fatalf("SessionGet() for disabled project = (%#v, %v)", unavailableSession, err)
	}
	err, unavailableSessions, _ := store.SessionsSearch(ctx, workspace, alice, database.SessionSearch{Limit: 50})
	if err != nil || len(unavailableSessions) != 0 {
		t.Fatalf("SessionsSearch() for disabled project = (%#v, %v)", unavailableSessions, err)
	}
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_projects SET enabled = TRUE WHERE workspace = ? AND id = ?`, workspace.Id, projectRef.Id); err != nil {
		t.Fatal(err)
	}
	checkpoints := []database.ActivityTopicCheckpoint{
		{Topic: database.ActivityTopicProject(projectRef), Events: []string{"project.*"}},
		{Topic: database.ActivityTopicSession(session), Events: []string{"session.*"}},
	}
	err, advanced := store.ActivityTopicCheckpointsGet(ctx, alice, checkpoints)
	if err != nil || advanced[0].ID == "" || advanced[1].ID == "" {
		t.Fatalf("ActivityTopicCheckpointsGet() for creator = (%#v, %v)", advanced, err)
	}
	err, hidden := store.ActivityTopicCheckpointsGet(ctx, bob, checkpoints)
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
		INSERT INTO gatehouse_project_grants (id, workspace, project, role, principal, "group", enabled) VALUES
			('pgr_00000000000000000000000000', ?, 'prj_00000000000000000000000000', 'member', ?, NULL, TRUE),
			('pgr_00000000000000000000000001', ?, 'prj_00000000000000000000000001', 'member', ?, NULL, TRUE),
			('pgr_00000000000000000000000002', ?, 'prj_00000000000000000000000002', 'member', ?, NULL, TRUE)
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
