package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/model"
	"gatehouse/sessionsearch"
	"gatehouse/typed_id"
)

const (
	gatehouseSessionFilesModuleID   = "native:gatehouse/session/files/v1"
	gatehouseSessionNotesModuleID   = "native:gatehouse/session/notes/v1"
	gatehouseSessionTasksModuleID   = "native:gatehouse/session/tasks/v1"
	gatehouseSessionSecretsModuleID = "native:gatehouse/session/secrets/v1"
	gatehouseSessionEventsModuleID  = "native:gatehouse/session/events/v1"
)

func (runtime *SessionEventReplyRuntime) sessionActionAllowed(ctx context.Context, session model.SessionRef, principal model.PrincipalRef, action authz.SessionAction) error {
	err, roles := runtime.store.SessionRolesGet(ctx, session, principal)
	if err != nil {
		return err
	}
	if !authz.SessionAllows(roles, action) {
		return fmt.Errorf("session action is unavailable")
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) sessionFilesModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("session/files/list requires no arguments"), nil
		}
		err, available := runtime.store.SessionGet(ctx, session, principal)
		if err != nil {
			return lisp.Errorf("session/files/list failed"), nil
		}
		if available == nil {
			return nil, lisp.List()
		}
		err, summaries := runtime.store.SessionFilesGet(ctx, session)
		if err != nil {
			return lisp.Errorf("session/files/list failed"), nil
		}
		values := make([]lisp.Expr, 0, len(summaries))
		for _, summary := range summaries {
			err, value := fileValue(File{ID: summary.ID, Name: summary.Name, MediaType: summary.MediaType, Size: summary.Size, Fingerprint: summary.Fingerprint}, "session/files/list")
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
	info := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("session/files/info requires an id"), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		if id == "" {
			return lisp.Errorf("session/files/info requires a non-empty id"), nil
		}
		if !typed_id.Valid(typed_id.SessionFile, id) {
			return lisp.Errorf("not a session file"), nil
		}
		err, file, object := runtime.store.SessionFileGet(ctx, model.SessionFileRef{Session: session, Id: id}, principal)
		if err != nil {
			return lisp.Errorf("session/files/info failed"), nil
		}
		if file == nil || object == nil {
			return nil, lisp.Null()
		}
		return fileValue(File{ID: file.Ref.Id, Name: file.Name, MediaType: file.MediaType, Size: object.Size, Fingerprint: fmt.Sprintf("sha256:%x", object.SHA256)}, "session/files/info")
	}
	read := fileReadFunction(func(id string, offset, length int64) (error, []byte) {
		err, file, _ := runtime.store.SessionFileGet(ctx, model.SessionFileRef{Session: session, Id: id}, principal)
		if err != nil || file == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read session file: unavailable"), nil
		}
		return runtime.storage.Read(ctx, file.StorageObject.Id, offset, length)
	}, "session/files/read", typed_id.SessionFile, "session")
	create := sessionFileCreateFunction(func(name, mediaType string, source io.Reader) (error, string) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (string, error) {
			if err := runtime.sessionActionAllowed(step, session, principal, authz.SessionFileCreate); err != nil {
				return "", fmt.Errorf("create session file: %w", err)
			}
			fileID, err := typed_id.New(typed_id.SessionFile)
			if err != nil {
				return "", fmt.Errorf("generate session file ID: %w", err)
			}
			objectID, err := typed_id.New(typed_id.StorageObject)
			if err != nil {
				return "", fmt.Errorf("generate storage object ID: %w", err)
			}
			mediaTypeValue := mediaType
			err, file, storageObjectID := runtime.store.SessionFileCreate(step, model.SessionFile{Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: name, MediaType: &mediaTypeValue, Enabled: true}, objectID, principal)
			if err != nil {
				return "", err
			}
			if err := runtime.storage.Put(step, storageObjectID, source, -1); err != nil {
				return "", err
			}
			if err := runtime.storage.Finish(step, storageObjectID); err != nil {
				return "", err
			}
			return file.Ref.Id, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-file-create"))
		return err, result
	}, "session/files/create")
	search := sessionFileSearchFunction(func(id, query, cursor string) (error, SessionFileSearchResult) {
		err, file, object := runtime.store.SessionFileGet(ctx, model.SessionFileRef{Session: session, Id: id}, principal)
		if err != nil || file == nil || object == nil {
			if err != nil {
				return err, SessionFileSearchResult{}
			}
			return fmt.Errorf("search session file: unavailable"), SessionFileSearchResult{}
		}
		return sessionFileSearch(func(offset int64) (error, io.ReadCloser) {
			return runtime.storage.Get(ctx, file.StorageObject.Id, offset)
		}, id, fmt.Sprintf("sha256:%x", object.SHA256), query, cursor)
	}, "session/files/search-file", typed_id.SessionFile, "session")
	update := fileUpdateFunction(func(id, name string) (error, File) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (File, error) {
			err, roles := runtime.store.SessionRolesGet(step, session, principal)
			if err != nil {
				return File{}, err
			}
			if !authz.SessionAllows(roles, authz.SessionFileUpdate) {
				return File{}, fmt.Errorf("update session file: permission denied")
			}
			err, file, object := runtime.store.SessionFileUpdate(step, model.SessionFileRef{Session: session, Id: id}, database.FileUpdate{Name: name}, principal)
			if err != nil || file == nil || object == nil {
				if err == nil {
					err = fmt.Errorf("update session file: unavailable")
				}
				return File{}, err
			}
			return File{ID: file.Ref.Id, Name: file.Name, MediaType: file.MediaType, Size: object.Size, Fingerprint: fmt.Sprintf("sha256:%x", object.SHA256)}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-file-update"))
		return err, result
	}, "session/files/update", typed_id.SessionFile, "session")
	remove := fileRemoveFunction(func(id string) (error, bool) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			err, roles := runtime.store.SessionRolesGet(step, session, principal)
			if err != nil {
				return false, err
			}
			if !authz.SessionAllows(roles, authz.SessionFileRemove) {
				return false, fmt.Errorf("remove session file: permission denied")
			}
			err, removed := runtime.store.SessionFileRemove(step, model.SessionFileRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-file-remove"))
		return err, result
	}, "session/files/remove", typed_id.SessionFile, "session")
	return lisp.HostModule{ID: gatehouseSessionFilesModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.Function(list), fileListDocumentation)},
		{Name: "info", Value: document(lisp.FunctionNonLeaky(info), fileInfoDocumentation)},
		{Name: "read", Value: document(lisp.Function(read), fileReadDocumentation)},
		{Name: "create", Value: document(lisp.FunctionWithContext(create), fileCreateDocumentation)},
		{Name: "search-file", Value: document(lisp.FunctionNonLeaky(search), fileSearchDocumentation)},
		{Name: "update", Value: document(lisp.FunctionNonLeaky(update), fileUpdateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(remove), fileRemoveDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) sessionNotesModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("session/notes/list requires no arguments"), nil
		}
		err, summaries := runtime.store.SessionNotesGet(ctx, session, principal)
		if err != nil {
			return lisp.Errorf("session/notes/list failed"), nil
		}
		values := make([]lisp.Expr, 0, len(summaries))
		for _, summary := range summaries {
			authorID, authorName := noteAuthorMetadata(summary.AuthorPrincipal, summary.AuthorName, summary.AuthorAgent, summary.AuthorAgentLabel, summary.AuthorGateway)
			err, value := sessionNoteValue(SessionNote{ID: summary.Ref.Id, Title: summary.Title, Description: summary.Description, Sensitive: summary.Sensitive, AuthorID: authorID, AuthorName: authorName, CreatedAt: summary.CreatedAt, Revision: summary.Revision}, "session/notes/list")
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
	read := noteReadFunction(func(id string, offset, length int64) (error, []byte, bool) {
		err, detail := runtime.store.SessionNoteGet(ctx, model.SessionNoteRef{Session: session, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read session note: unavailable"), nil, false
		}
		return sessionTextRange(detail.Note.Body, offset, length, detail.Note.Sensitive, "read session note")
	}, "session/notes/read")
	revisions := noteRevisionListFunction(func(id string) (error, []NoteRevision) {
		err, summaries := runtime.store.SessionNoteRevisionsGet(ctx, model.SessionNoteRef{Session: session, Id: id}, principal)
		if err != nil {
			return err, nil
		}
		values := make([]NoteRevision, 0, len(summaries))
		for _, summary := range summaries {
			authorID, authorName := noteAuthorMetadata(summary.AuthorPrincipal, summary.AuthorName, summary.AuthorAgent, summary.AuthorAgentLabel, summary.AuthorGateway)
			values = append(values, NoteRevision{Revision: summary.Ref.Revision, Title: summary.Title, Description: summary.Description, Sensitive: summary.Sensitive, AuthorID: authorID, AuthorName: authorName, CreatedAt: summary.CreatedAt})
		}
		return nil, values
	}, "session/notes/revisions/list")
	revisionRead := noteRevisionReadFunction(func(id string, revision int, offset, length int64) (error, []byte, bool) {
		err, detail := runtime.store.SessionNoteRevisionGet(ctx, model.SessionNoteRevisionRef{Note: model.SessionNoteRef{Session: session, Id: id}, Revision: revision}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read session note revision: unavailable"), nil, false
		}
		return sessionTextRange(detail.Revision.Body, offset, length, detail.Revision.Sensitive, "read session note revision")
	}, "session/notes/revisions/read")
	create := sessionNoteCreateFunction(func(title, description, body string, sensitive bool) (error, SessionNote) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (SessionNote, error) {
			if err := runtime.sessionActionAllowed(step, session, principal, authz.SessionNoteCreate); err != nil {
				return SessionNote{}, fmt.Errorf("create session note: %w", err)
			}
			id, err := typed_id.New(typed_id.SessionNote)
			if err != nil {
				return SessionNote{}, fmt.Errorf("generate session note ID: %w", err)
			}
			err, note := runtime.store.SessionNoteCreate(step, model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: id}, AuthorAgent: &agent, Title: title, Description: description, Body: body, Sensitive: sensitive}, principal)
			if err != nil {
				return SessionNote{}, err
			}
			return SessionNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorAgent.Id, CreatedAt: note.CreatedAt, Revision: note.Revision}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-create"))
		return err, result
	}, "session/notes/create")
	update := sessionNoteUpdateFunction(func(id, title, description, body string, sensitive bool) (error, SessionNote) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (SessionNote, error) {
			if err := runtime.sessionActionAllowed(step, session, principal, authz.SessionNoteEdit); err != nil {
				return SessionNote{}, fmt.Errorf("update session note: %w", err)
			}
			err, detail := runtime.store.SessionNoteDetailsSetAs(step, model.SessionNoteRef{Session: session, Id: id}, principal, database.NoteAuthor{Agent: &agent}, sensitive, &title, &description, &body)
			if err != nil || detail == nil {
				if err == nil {
					err = fmt.Errorf("update session note: unavailable")
				}
				return SessionNote{}, err
			}
			return SessionNote{ID: detail.Note.Ref.Id, Title: detail.Note.Title, Description: detail.Note.Description, Sensitive: detail.Note.Sensitive, AuthorID: noteAuthorID(detail.Note.AuthorPrincipal, detail.Note.AuthorAgent, detail.Note.AuthorGateway), CreatedAt: detail.Note.CreatedAt, Revision: detail.Note.Revision}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-update"))
		return err, result
	}, "session/notes/update")
	remove := noteRemoveFunction(func(id string) (error, bool) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			if err := runtime.sessionActionAllowed(step, session, principal, authz.SessionNoteRemove); err != nil {
				return false, fmt.Errorf("remove session note: %w", err)
			}
			err, removed := runtime.store.SessionNoteRemove(step, model.SessionNoteRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-remove"))
		return err, result
	}, "session/notes/remove")
	return lisp.HostModule{ID: gatehouseSessionNotesModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.Function(list), sessionNoteListDocumentation)},
		{Name: "read", Value: document(lisp.Function(read), sessionNoteReadDocumentation)},
		{Name: "revisions/list", Value: document(lisp.Function(revisions), sessionNoteRevisionsListDocumentation)},
		{Name: "revisions/read", Value: document(lisp.Function(revisionRead), sessionNoteRevisionReadDocumentation)},
		{Name: "create", Value: document(lisp.FunctionNonLeaky(create), sessionNoteCreateDocumentation)},
		{Name: "update", Value: document(lisp.FunctionNonLeaky(update), sessionNoteUpdateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(remove), sessionNoteRemoveDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) sessionTasksModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("session/tasks/list requires no arguments"), nil
		}
		err, tasks := runtime.store.SessionTasksGet(ctx, session, principal)
		if err != nil {
			return lisp.Errorf("session/tasks/list failed"), nil
		}
		values := make([]lisp.Expr, 0, len(tasks))
		for _, task := range tasks {
			err, value := taskValue(sessionTaskValue(task), "session/tasks/list")
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
	read := taskReadFunction(func(id string, offset, length int64) (error, []byte, bool) {
		err, task := runtime.store.SessionTaskGet(ctx, model.SessionTaskRef{Session: session, Id: id}, principal)
		if err != nil || task == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read session task: unavailable"), nil, false
		}
		description := ""
		if task.Description != nil {
			description = *task.Description
		}
		return sessionTextRange(description, offset, length, task.Sensitive, "read session task")
	}, "session/tasks/read")
	create := taskCreateFunction(func(title, description, status string, sensitive bool) (error, Task) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (Task, error) {
			rolesErr, roles := runtime.store.SessionRolesGet(step, session, principal)
			if rolesErr != nil {
				return Task{}, rolesErr
			}
			if !authz.SessionAllows(roles, authz.SessionTaskCreate) {
				return Task{}, fmt.Errorf("create session task: permission denied")
			}
			id, err := typed_id.New(typed_id.SessionTask)
			if err != nil {
				return Task{}, fmt.Errorf("generate session task ID: %w", err)
			}
			err, task := runtime.store.SessionTaskCreate(step, model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: id}, Title: title, Description: &description, Sensitive: sensitive, Status: status, CreatorAgent: &agent, UpdaterAgent: &agent}, principal)
			if err != nil {
				return Task{}, err
			}
			return sessionTaskValue(task), nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-task-create"))
		return err, result
	}, "session/tasks/create")
	update := taskUpdateFunction(func(id, title, description, status string, sensitive bool) (error, Task) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (Task, error) {
			rolesErr, roles := runtime.store.SessionRolesGet(step, session, principal)
			if rolesErr != nil {
				return Task{}, rolesErr
			}
			if !authz.SessionAllows(roles, authz.SessionTaskEdit) {
				return Task{}, fmt.Errorf("update session task: permission denied")
			}
			err, task := runtime.store.SessionTaskDetailsSetAs(step, model.SessionTaskRef{Session: session, Id: id}, principal, database.TaskAuthor{Agent: &agent}, sensitive, status, title, &description)
			if err != nil || task == nil {
				if err == nil {
					err = fmt.Errorf("update session task: unavailable")
				}
				return Task{}, err
			}
			return sessionTaskValue(*task), nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-task-update"))
		return err, result
	}, "session/tasks/update")
	remove := taskRemoveFunction(func(id string) (error, bool) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			rolesErr, roles := runtime.store.SessionRolesGet(step, session, principal)
			if rolesErr != nil {
				return false, rolesErr
			}
			if !authz.SessionAllows(roles, authz.SessionTaskRemove) {
				return false, fmt.Errorf("remove session task: permission denied")
			}
			err, removed := runtime.store.SessionTaskRemove(step, model.SessionTaskRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-task-remove"))
		return err, result
	}, "session/tasks/remove")
	return lisp.HostModule{ID: gatehouseSessionTasksModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.Function(list), sessionTaskListDocumentation)},
		{Name: "read", Value: document(lisp.Function(read), sessionTaskReadDocumentation)},
		{Name: "create", Value: document(lisp.FunctionNonLeaky(create), sessionTaskCreateDocumentation)},
		{Name: "update", Value: document(lisp.FunctionNonLeaky(update), sessionTaskUpdateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(remove), sessionTaskRemoveDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) sessionSecretsModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("session/secrets/list requires no arguments"), nil
		}
		err, summaries := runtime.store.SessionSecretsGet(ctx, session, principal)
		if err != nil {
			return lisp.Errorf("session/secrets/list failed"), nil
		}
		values := make([]lisp.Expr, 0, len(summaries))
		for _, summary := range summaries {
			err, value := sessionSecretValue(SessionSecret{ID: summary.Ref.Id, Description: summary.Description, AuthorID: summary.AuthorPrincipal.Id, AuthorName: summary.AuthorName, CreatedAt: summary.CreatedAt, UpdatedAt: summary.UpdatedAt}, "session/secrets/list")
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
	read := sessionSecretReadFunction(func(id string) (error, []byte) {
		err, detail := runtime.store.SessionSecretGet(ctx, model.SessionSecretRef{Session: session, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read session secret: unavailable"), nil
		}
		err, encrypted := keychain.ParseResource(detail.Secret.Ciphertext)
		if err != nil {
			return fmt.Errorf("parse session secret: %w", err), nil
		}
		err, keys := runtime.keyring.Get(ctx, []model.KeychainRef{*encrypted.Key})
		if err != nil {
			return fmt.Errorf("get keychain for session secret: %w", err), nil
		}
		key := keys[*encrypted.Key]
		defer clear(key)
		clear(keys)
		err, value := keychain.Open(key, database.SessionSecretAssociatedData(detail.Secret.Ref), encrypted)
		if err != nil {
			return fmt.Errorf("decrypt session secret: %w", err), nil
		}
		return nil, value
	}, "session/secrets/read")
	return lisp.HostModule{ID: gatehouseSessionSecretsModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.FunctionNonLeaky(list), sessionSecretListDocumentation)},
		{Name: "read", Value: document(lisp.Function(read), sessionSecretReadDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) sessionEventsModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) lisp.HostModule {
	read := sessionEventReadFunction(func(id string, offset, length int64) (error, []byte) {
		err, available := runtime.store.SessionGet(ctx, session, principal)
		if err != nil || available == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read session event: unavailable"), nil
		}
		err, event := runtime.store.SessionEventGet(ctx, model.SessionEventRef{Session: session, Id: id})
		if err != nil || event == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read session event: unavailable"), nil
		}
		return sessionEventReadRange(*event, offset, length)
	}, "session/events/read")
	search := sessionEventSearchFunction(func(source, cursor string) (error, SessionEventSearchResult) {
		err, available := runtime.store.SessionGet(ctx, session, principal)
		if err != nil {
			return err, SessionEventSearchResult{}
		}
		if available == nil {
			return fmt.Errorf("search session events: unavailable"), SessionEventSearchResult{}
		}
		err, expression := sessionsearch.Parse(source)
		if err != nil {
			return err, SessionEventSearchResult{}
		}
		err, beforeID := sessionsearch.Cursor(source, cursor)
		if err != nil {
			return err, SessionEventSearchResult{}
		}
		err, events, more := runtime.store.SessionEventsSearch(ctx, session, expression, beforeID, 8)
		if err != nil {
			return err, SessionEventSearchResult{}
		}
		result := SessionEventSearchResult{Events: make([]sessionsearch.EventResult, 0, len(events))}
		for _, event := range events {
			entry, matches := sessionsearch.Result(event, expression)
			if matches {
				result.Events = append(result.Events, entry)
			}
		}
		if more && len(events) > 0 {
			result.NextCursor = sessionsearch.NextCursor(source, events[len(events)-1].Ref.Id)
		}
		return nil, result
	}, "session/events/search")
	return lisp.HostModule{ID: gatehouseSessionEventsModuleID, Exports: []lisp.HostExport{
		{Name: "read", Value: document(lisp.Function(read), sessionEventReadDocumentation)},
		{Name: "search", Value: document(lisp.FunctionNonLeaky(search), sessionEventSearchDocumentation)},
	}}
}

func sessionTextRange(value string, offset, length int64, sensitive bool, operation string) (error, []byte, bool) {
	contents := []byte(value)
	if offset > int64(len(contents)) {
		return fmt.Errorf("%s: offset is unavailable", operation), nil, false
	}
	end := offset + length
	if end > int64(len(contents)) {
		end = int64(len(contents))
	}
	return nil, append([]byte(nil), contents[offset:end]...), sensitive
}
