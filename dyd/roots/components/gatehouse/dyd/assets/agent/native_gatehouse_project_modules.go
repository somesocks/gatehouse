package agent

import (
	"fmt"
	"io"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/model"
	"gatehouse/typed_id"
)

const (
	gatehouseProjectInfoModuleID    = "native:gatehouse/project/info/v1"
	gatehouseProjectFilesModuleID   = "native:gatehouse/project/files/v1"
	gatehouseProjectNotesModuleID   = "native:gatehouse/project/notes/v1"
	gatehouseProjectTasksModuleID   = "native:gatehouse/project/tasks/v1"
	gatehouseProjectSecretsModuleID = "native:gatehouse/project/secrets/v1"
	gatehouseProjectRecordsModuleID = "native:gatehouse/project/records/v1"
)

// These modules deliberately resolve the linked project for every operation.
// A session can be relinked while a tool evaluation is in progress.
func (runtime *SessionEventReplyRuntime) projectInfoModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) lisp.HostModule {
	get := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/info/get requires no arguments"), nil
		}
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return nil, lisp.Null()
		}
		err, value := runtime.store.ProjectGet(ctx, *project, principal)
		if err != nil || value == nil {
			if err != nil {
				return lisp.Errorf("project/info/get failed"), nil
			}
			return nil, lisp.Null()
		}
		return projectInfoValue(ProjectInfo{Name: value.Name, Description: value.Description, CreatedAt: value.CreatedAt}, "project/info/get")
	}
	return lisp.HostModule{ID: gatehouseProjectInfoModuleID, Exports: []lisp.HostExport{
		{Name: "get", Value: document(lisp.Function(get), projectInfoGetDocumentation)},
		{Name: "set", Value: document(lisp.Function(projectInfoSetFunction(runtime.projectInfoSet(ctx, session, principal))), projectInfoSetDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) projectFilesModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/files/list requires no arguments"), nil
		}
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return nil, lisp.List()
		}
		err, summaries := runtime.store.ProjectFilesGet(ctx, *project, principal)
		if err != nil {
			return lisp.Errorf("project/files/list failed"), nil
		}
		files := make([]File, 0, len(summaries))
		for _, summary := range summaries {
			files = append(files, File{ID: summary.ID, Name: summary.Name, MediaType: summary.MediaType, Size: summary.Size, Fingerprint: summary.Fingerprint})
		}
		return fileListFunction(files, "project/files/list")(arguments)
	}
	info := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("project/files/info requires an id"), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		if id == "" {
			return lisp.Errorf("project/files/info requires a non-empty id"), nil
		}
		if !typed_id.Valid(typed_id.ProjectFile, id) {
			return lisp.Errorf("not a project file"), nil
		}
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return nil, lisp.Null()
		}
		err, file, object := runtime.store.ProjectFileGet(ctx, model.ProjectFileRef{Project: *project, Id: id}, principal)
		if err != nil {
			return lisp.Errorf("project/files/info failed"), nil
		}
		if file == nil || object == nil {
			return nil, lisp.Null()
		}
		return fileValue(File{ID: file.Ref.Id, Name: file.Name, MediaType: file.MediaType, Size: object.Size, Fingerprint: fmt.Sprintf("sha256:%x", object.SHA256)}, "project/files/info")
	}
	read := func(id string, offset, length int64) (error, []byte) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return fmt.Errorf("read project file: %w", err), nil
		}
		err, file, object := runtime.store.ProjectFileGet(ctx, model.ProjectFileRef{Project: *project, Id: id}, principal)
		if err != nil || file == nil || object == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read project file: unavailable"), nil
		}
		return runtime.storage.Read(ctx, object.ID, offset, length)
	}
	search := func(id, query, cursor string) (error, SessionFileSearchResult) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return fmt.Errorf("search project file: %w", err), SessionFileSearchResult{}
		}
		err, file, object := runtime.store.ProjectFileGet(ctx, model.ProjectFileRef{Project: *project, Id: id}, principal)
		if err != nil || file == nil || object == nil {
			if err != nil {
				return err, SessionFileSearchResult{}
			}
			return fmt.Errorf("search project file: unavailable"), SessionFileSearchResult{}
		}
		return sessionFileSearch(func(offset int64) (error, io.ReadCloser) {
			return runtime.storage.Get(ctx, object.ID, offset)
		}, id, fmt.Sprintf("sha256:%x", object.SHA256), query, cursor)
	}
	return lisp.HostModule{ID: gatehouseProjectFilesModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.Function(list), projectFileListDocumentation)},
		{Name: "info", Value: document(lisp.FunctionNonLeaky(info), projectFileInfoDocumentation)},
		{Name: "read", Value: document(lisp.Function(fileReadFunction(read, "project/files/read", typed_id.ProjectFile, "project")), projectFileReadDocumentation)},
		{Name: "create", Value: document(lisp.FunctionWithContext(sessionFileCreateFunction(runtime.projectFileCreate(ctx, session, principal), "project/files/create")), projectFileCreateDocumentation)},
		{Name: "update", Value: document(lisp.FunctionNonLeaky(fileUpdateFunction(runtime.projectFileUpdate(ctx, session, principal), "project/files/update", typed_id.ProjectFile, "project")), projectFileUpdateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(fileRemoveFunction(runtime.projectFileRemove(ctx, session, principal), "project/files/remove", typed_id.ProjectFile, "project")), projectFileRemoveDocumentation)},
		{Name: "search-file", Value: document(lisp.FunctionNonLeaky(sessionFileSearchFunction(search, "project/files/search-file", typed_id.ProjectFile, "project")), projectFileSearchDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) projectNotesModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/notes/list requires no arguments"), nil
		}
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return nil, lisp.List()
		}
		err, summaries := runtime.store.ProjectNotesGet(ctx, *project, principal)
		if err != nil {
			return lisp.Errorf("project/notes/list failed"), nil
		}
		notes := make([]ProjectNote, 0, len(summaries))
		for _, note := range summaries {
			authorID, authorName := noteAuthorMetadata(note.AuthorPrincipal, note.AuthorName, note.AuthorAgent, note.AuthorAgentLabel, note.AuthorGateway)
			notes = append(notes, ProjectNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: authorID, AuthorName: authorName, CreatedAt: note.CreatedAt, Revision: note.Revision})
		}
		return noteListFunction(notes, "project/notes/list")(arguments)
	}
	read := func(id string, offset, length int64) (error, []byte, bool) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return fmt.Errorf("read project note: %w", err), nil, false
		}
		err, detail := runtime.store.ProjectNoteGet(ctx, model.ProjectNoteRef{Project: *project, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read project note: unavailable"), nil, false
		}
		return projectNoteBodyRange(detail.Note.Body, detail.Note.Sensitive, offset, length, "read project note")
	}
	revisions := func(id string) (error, []NoteRevision) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, summaries := runtime.store.ProjectNoteRevisionsGet(ctx, model.ProjectNoteRef{Project: *project, Id: id}, principal)
		if err != nil {
			return err, nil
		}
		result := make([]NoteRevision, 0, len(summaries))
		for _, revision := range summaries {
			authorID, authorName := noteAuthorMetadata(revision.AuthorPrincipal, revision.AuthorName, revision.AuthorAgent, revision.AuthorAgentLabel, revision.AuthorGateway)
			result = append(result, NoteRevision{Revision: revision.Ref.Revision, Title: revision.Title, Description: revision.Description, Sensitive: revision.Sensitive, AuthorID: authorID, AuthorName: authorName, CreatedAt: revision.CreatedAt})
		}
		return nil, result
	}
	revisionRead := func(id string, revision int, offset, length int64) (error, []byte, bool) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return fmt.Errorf("read project note revision: %w", err), nil, false
		}
		err, detail := runtime.store.ProjectNoteRevisionGet(ctx, model.ProjectNoteRevisionRef{Note: model.ProjectNoteRef{Project: *project, Id: id}, Revision: revision}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read project note revision: unavailable"), nil, false
		}
		return projectNoteBodyRange(detail.Revision.Body, detail.Revision.Sensitive, offset, length, "read project note revision")
	}
	return lisp.HostModule{ID: gatehouseProjectNotesModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.Function(list), projectNoteListDocumentation)},
		{Name: "read", Value: document(lisp.Function(noteReadFunction(read, "project/notes/read")), projectNoteReadDocumentation)},
		{Name: "revisions/list", Value: document(lisp.Function(noteRevisionListFunction(revisions, "project/notes/revisions/list")), projectNoteRevisionsListDocumentation)},
		{Name: "revisions/read", Value: document(lisp.Function(noteRevisionReadFunction(revisionRead, "project/notes/revisions/read")), projectNoteRevisionReadDocumentation)},
		{Name: "create", Value: document(lisp.FunctionNonLeaky(projectNoteCreateFunction(runtime.projectNoteCreate(ctx, session, principal, agent), "project/notes/create")), projectNoteCreateDocumentation)},
		{Name: "update", Value: document(lisp.FunctionNonLeaky(projectNoteUpdateFunction(runtime.projectNoteUpdate(ctx, session, principal, agent), "project/notes/update")), projectNoteUpdateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(noteRemoveFunction(runtime.projectNoteRemove(ctx, session, principal), "project/notes/remove")), projectNoteRemoveDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) projectTasksModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/tasks/list requires no arguments"), nil
		}
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return lisp.Errorf("project/tasks/list is unavailable"), nil
		}
		err, summaries := runtime.store.ProjectTasksGet(ctx, *project, principal)
		if err != nil {
			return lisp.Errorf("project/tasks/list failed"), nil
		}
		tasks := make([]Task, 0, len(summaries))
		for _, task := range summaries {
			tasks = append(tasks, projectTaskValue(task))
		}
		return taskListFunction(tasks, "project/tasks/list")(arguments)
	}
	return lisp.HostModule{ID: gatehouseProjectTasksModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.Function(list), projectTaskListDocumentation)},
		{Name: "read", Value: document(lisp.Function(taskReadFunction(runtime.projectTaskRead(ctx, session, principal), "project/tasks/read")), projectTaskReadDocumentation)},
		{Name: "create", Value: document(lisp.FunctionNonLeaky(taskCreateFunction(runtime.projectTaskCreate(ctx, session, principal, agent), "project/tasks/create")), projectTaskCreateDocumentation)},
		{Name: "update", Value: document(lisp.FunctionNonLeaky(taskUpdateFunction(runtime.projectTaskUpdate(ctx, session, principal, agent), "project/tasks/update")), projectTaskUpdateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(taskRemoveFunction(runtime.projectTaskRemove(ctx, session, principal), "project/tasks/remove")), projectTaskRemoveDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) projectSecretsModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) lisp.HostModule {
	list := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/secrets/list requires no arguments"), nil
		}
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return nil, lisp.List()
		}
		err, summaries := runtime.store.ProjectSecretsGet(ctx, *project, principal)
		if err != nil {
			return lisp.Errorf("project/secrets/list failed"), nil
		}
		secrets := make([]ProjectSecret, 0, len(summaries))
		for _, secret := range summaries {
			secrets = append(secrets, ProjectSecret{ID: secret.Ref.Id, Description: secret.Description, AuthorID: secret.AuthorPrincipal.Id, AuthorName: secret.AuthorName, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt})
		}
		return projectSecretListFunction(secrets, "project/secrets/list")(arguments)
	}
	read := func(id string) (error, []byte) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return fmt.Errorf("read project secret: %w", err), nil
		}
		err, detail := runtime.store.ProjectSecretGet(ctx, model.ProjectSecretRef{Project: *project, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read project secret: unavailable"), nil
		}
		err, encrypted := keychain.ParseResource(detail.Secret.Ciphertext)
		if err != nil {
			return fmt.Errorf("parse project secret: %w", err), nil
		}
		err, keys := runtime.keyring.Get(ctx, []model.KeychainRef{*encrypted.Key})
		if err != nil {
			return fmt.Errorf("get keychain for project secret: %w", err), nil
		}
		key := keys[*encrypted.Key]
		defer clear(key)
		clear(keys)
		err, value := keychain.Open(key, database.ProjectSecretAssociatedData(detail.Secret.Ref), encrypted)
		if err != nil {
			return fmt.Errorf("decrypt project secret: %w", err), nil
		}
		return nil, value
	}
	return lisp.HostModule{ID: gatehouseProjectSecretsModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.FunctionNonLeaky(list), projectSecretListDocumentation)},
		{Name: "read", Value: document(lisp.Function(projectSecretReadFunction(read, "project/secrets/read")), projectSecretReadDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) projectRecordsModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) lisp.HostModule {
	schemas := func() (error, []ProjectRecordSchema) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, values := runtime.store.ProjectRecordSchemasGet(ctx, *project, principal)
		if err != nil {
			return err, nil
		}
		result := make([]ProjectRecordSchema, 0, len(values))
		for _, value := range values {
			result = append(result, projectRecordSchemaFromModel(value))
		}
		return nil, result
	}
	schema := func(id string) (error, *ProjectRecordSchema) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, value := runtime.store.ProjectRecordSchemaGet(ctx, model.ProjectRecordSchemaRef{Project: *project, Id: id}, principal)
		if err != nil || value == nil {
			return err, nil
		}
		result := projectRecordSchemaFromModel(*value)
		return nil, &result
	}
	attributes := func(schemaID string) (error, []ProjectRecordAttribute) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, values := runtime.store.ProjectRecordAttributesGet(ctx, model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, principal)
		if err != nil {
			return err, nil
		}
		result := make([]ProjectRecordAttribute, 0, len(values))
		for _, value := range values {
			result = append(result, projectRecordAttributeFromModel(value))
		}
		return nil, result
	}
	attribute := func(schemaID, id string) (error, *ProjectRecordAttribute) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, value := runtime.store.ProjectRecordAttributeGet(ctx, model.ProjectRecordAttributeRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: id}, principal)
		if err != nil || value == nil {
			return err, nil
		}
		result := projectRecordAttributeFromModel(*value)
		return nil, &result
	}
	records := func(schemaID, cursor string) (error, []ProjectRecord) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, values := runtime.store.ProjectRecordsGet(ctx, model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, principal, 100, cursor)
		if err != nil {
			return err, nil
		}
		result := make([]ProjectRecord, 0, len(values))
		for _, value := range values {
			result = append(result, projectRecordFromModel(value))
		}
		return nil, result
	}
	record := func(schemaID, id string) (error, *ProjectRecord) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, value := runtime.store.ProjectRecordGet(ctx, model.ProjectRecordRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: id}, principal)
		if err != nil || value == nil {
			return err, nil
		}
		result := projectRecordFromModel(*value)
		return nil, &result
	}
	values := func(schemaID, recordID, cursor string) (error, []ProjectRecordValue) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return err, nil
		}
		err, stored := runtime.store.ProjectRecordValuesGet(ctx, model.ProjectRecordRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: recordID}, principal, 100, cursor)
		if err != nil {
			return err, nil
		}
		result := make([]ProjectRecordValue, 0, len(stored))
		for _, value := range stored {
			result = append(result, projectRecordValueFromModel(value))
		}
		return nil, result
	}
	return lisp.HostModule{ID: gatehouseProjectRecordsModuleID, Exports: []lisp.HostExport{
		{Name: "schemas/list", Value: document(lisp.Function(projectRecordSchemasListFunction(schemas)), projectRecordSchemasListDocumentation)},
		{Name: "schemas/get", Value: document(lisp.FunctionNonLeaky(projectRecordSchemaGetFunction(schema)), projectRecordSchemaGetDocumentation)},
		{Name: "schemas/create", Value: document(lisp.FunctionNonLeaky(projectRecordSchemaCreateFunction(runtime.projectRecordSchemaCreate(ctx, session, principal, agent))), projectRecordSchemaCreateDocumentation)},
		{Name: "schemas/update", Value: document(lisp.FunctionNonLeaky(projectRecordSchemaUpdateFunction(runtime.projectRecordSchemaUpdate(ctx, session, principal, agent))), projectRecordSchemaUpdateDocumentation)},
		{Name: "schemas/remove", Value: document(lisp.Function(projectRecordSchemaRemoveFunction(runtime.projectRecordSchemaRemove(ctx, session, principal))), projectRecordSchemaRemoveDocumentation)},
		{Name: "attributes/list", Value: document(lisp.Function(projectRecordAttributesListFunction(attributes)), projectRecordAttributesListDocumentation)},
		{Name: "attributes/get", Value: document(lisp.FunctionNonLeaky(projectRecordAttributeGetFunction(attribute)), projectRecordAttributeGetDocumentation)},
		{Name: "attributes/create", Value: document(lisp.FunctionNonLeaky(projectRecordAttributeCreateFunction(runtime.projectRecordAttributeCreate(ctx, session, principal, agent))), projectRecordAttributeCreateDocumentation)},
		{Name: "attributes/update", Value: document(lisp.FunctionNonLeaky(projectRecordAttributeUpdateFunction(runtime.projectRecordAttributeUpdate(ctx, session, principal, agent))), projectRecordAttributeUpdateDocumentation)},
		{Name: "attributes/remove", Value: document(lisp.Function(projectRecordAttributeRemoveFunction(runtime.projectRecordAttributeRemove(ctx, session, principal))), projectRecordAttributeRemoveDocumentation)},
		{Name: "list", Value: document(lisp.Function(projectRecordsListFunction(records)), projectRecordsListDocumentation)},
		{Name: "get", Value: document(lisp.FunctionNonLeaky(projectRecordGetFunction(record)), projectRecordGetDocumentation)},
		{Name: "create", Value: document(lisp.FunctionNonLeaky(projectRecordCreateFunction(runtime.projectRecordCreate(ctx, session, principal, agent))), projectRecordCreateDocumentation)},
		{Name: "remove", Value: document(lisp.Function(projectRecordRemoveRecordFunction(runtime.projectRecordRemove(ctx, session, principal))), projectRecordRemoveDocumentation)},
		{Name: "values/list", Value: document(lisp.FunctionNonLeaky(projectRecordValuesListFunction(values)), projectRecordValuesListDocumentation)},
		{Name: "values/mutate", Value: document(lisp.FunctionNonLeaky(projectRecordValuesMutateFunction(runtime.projectRecordValuesMutate(ctx, session, principal, agent))), projectRecordValuesMutateDocumentation)},
	}}
}

func projectNoteBodyRange(body string, sensitive bool, offset, length int64, operation string) (error, []byte, bool) {
	contents := []byte(body)
	if offset > int64(len(contents)) {
		return fmt.Errorf("%s: offset is unavailable", operation), nil, false
	}
	end := offset + length
	if end > int64(len(contents)) {
		end = int64(len(contents))
	}
	return nil, append([]byte(nil), contents[offset:end]...), sensitive
}
