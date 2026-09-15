package agent

import (
	"gatehouse/lisp"
	"gatehouse/typed_id"
)

const (
	gatehouseProjectModuleID = "native:gatehouse/project/v1"
	gatehouseSessionModuleID = "native:gatehouse/session/v1"
)

// SessionNotes contains the authorized notes in the current session.
type SessionNotes struct {
	Notes        []SessionNote
	Read         SessionNoteRead
	Revisions    SessionNoteRevisionsGet
	RevisionRead SessionNoteRevisionRead
	Create       SessionNoteCreate
	Update       SessionNoteUpdate
	Remove       NoteRemove
}

// SessionTasks contains the authorized flat tasks in the current session.
type SessionTasks struct {
	Tasks  []Task
	Read   TaskRead
	Create TaskCreate
	Update TaskUpdate
	Remove TaskRemove
}

// ProjectFiles contains the authorized files in a linked project.
type ProjectFiles struct {
	Files  []File
	Read   FileRead
	Create ProjectFileCreate
	Update FileUpdate
	Remove FileRemove
	Search ProjectFileSearch
}

// ProjectFileSearch searches one authorized linked-project file.
type ProjectFileSearch = SessionFileSearch

// ProjectNotes contains the authorized notes in a linked project.
type ProjectNotes struct {
	Notes        []ProjectNote
	Read         ProjectNoteRead
	Revisions    ProjectNoteRevisionsGet
	RevisionRead ProjectNoteRevisionRead
	Create       ProjectNoteCreate
	Update       ProjectNoteUpdate
	Remove       NoteRemove
}

// ProjectTasks contains the authorized flat tasks in a linked project.
type ProjectTasks struct {
	Tasks  []Task
	Read   TaskRead
	Create TaskCreate
	Update TaskUpdate
	Remove TaskRemove
}

// ProjectSecretRead reads an authorized project secret value.
type ProjectSecretRead func(id string) (error, []byte)

// ProjectSecrets contains authorized secrets in a linked project.
type ProjectSecrets struct {
	Secrets []ProjectSecret
	Read    ProjectSecretRead
}

// SessionFileActions contains mutation callbacks for current-session files.
type SessionFileActions struct {
	Update FileUpdate
	Remove FileRemove
}

func NewSessionModule(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, nil, nil, nil, notes, nil, nil, firstSessionEventRead(eventReads), nil)
}

func NewSessionModuleWithSecrets(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, secrets *SessionSecrets, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, nil, nil, nil, notes, nil, secrets, firstSessionEventRead(eventReads), nil)
}

func NewSessionModuleWithTasks(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, tasks *SessionTasks, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, nil, nil, nil, notes, tasks, nil, firstSessionEventRead(eventReads), nil)
}

func NewSessionModuleWithSecretsAndTasks(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, tasks *SessionTasks, secrets *SessionSecrets, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, nil, nil, nil, notes, tasks, secrets, firstSessionEventRead(eventReads), nil)
}

func NewSessionModuleWithSecretsTasksAndEventSearch(files []File, read FileRead, create SessionFileCreate, fileSearch SessionFileSearch, notes *SessionNotes, tasks *SessionTasks, secrets *SessionSecrets, eventRead SessionEventRead, eventSearch SessionEventSearch, actions ...SessionFileActions) lisp.HostModule {
	var update FileUpdate
	var remove FileRemove
	if len(actions) > 0 {
		update, remove = actions[0].Update, actions[0].Remove
	}
	return newSessionModule(files, read, create, update, remove, fileSearch, notes, tasks, secrets, eventRead, eventSearch)
}

func firstSessionEventRead(reads []SessionEventRead) SessionEventRead {
	if len(reads) == 0 {
		return nil
	}
	return reads[0]
}

func newSessionModule(files []File, read FileRead, create SessionFileCreate, update FileUpdate, remove FileRemove, fileSearch SessionFileSearch, notes *SessionNotes, tasks *SessionTasks, secrets *SessionSecrets, eventRead SessionEventRead, eventSearch SessionEventSearch) lisp.HostModule {
	fileRead := unavailableRead("session/files/read")
	if read != nil {
		fileRead = fileReadFunction(read, "session/files/read", typed_id.SessionFile, "session")
	}
	fileCreate := func(_ *lisp.FunctionContext, _ []lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("session/files/create is unavailable"), nil
	}
	if create != nil {
		fileCreate = sessionFileCreateFunction(create, "session/files/create")
	}
	fileUpdate := unavailableCreate("session/files/update")
	if update != nil {
		fileUpdate = fileUpdateFunction(update, "session/files/update", typed_id.SessionFile, "session")
	}
	fileRemove := unavailableRemove("session/files/remove")
	if remove != nil {
		fileRemove = fileRemoveFunction(remove, "session/files/remove", typed_id.SessionFile, "session")
	}
	fileSearchFunction := unavailableSessionFileSearch("session/files/search-file")
	if fileSearch != nil {
		fileSearchFunction = sessionFileSearchFunction(fileSearch, "session/files/search-file", typed_id.SessionFile, "session")
	}
	eventReadFunction := unavailableRead("session/events/read")
	if eventRead != nil {
		eventReadFunction = sessionEventReadFunction(eventRead, "session/events/read")
	}
	eventSearchFunction := unavailableSessionEventSearch("session/events/search")
	if eventSearch != nil {
		eventSearchFunction = sessionEventSearchFunction(eventSearch, "session/events/search")
	}
	notesList, noteRead := sessionNoteListFunction(nil, "session/notes/list"), unavailableRead("session/notes/read")
	noteRevisionsList, noteRevisionRead := unavailableNoteRevisionList("session/notes/revisions/list"), unavailableRead("session/notes/revisions/read")
	noteCreate, noteUpdate, noteRemove := unavailableCreate("session/notes/create"), unavailableCreate("session/notes/update"), unavailableRemove("session/notes/remove")
	if notes != nil {
		notesList = sessionNoteListFunction(notes.Notes, "session/notes/list")
		if notes.Read != nil {
			noteRead = noteReadFunction(notes.Read, "session/notes/read")
		}
		if notes.Revisions != nil {
			noteRevisionsList = noteRevisionListFunction(notes.Revisions, "session/notes/revisions/list")
		}
		if notes.RevisionRead != nil {
			noteRevisionRead = noteRevisionReadFunction(notes.RevisionRead, "session/notes/revisions/read")
		}
		if notes.Create != nil {
			noteCreate = sessionNoteCreateFunction(notes.Create, "session/notes/create")
		}
		if notes.Update != nil {
			noteUpdate = sessionNoteUpdateFunction(notes.Update, "session/notes/update")
		}
		if notes.Remove != nil {
			noteRemove = noteRemoveFunction(notes.Remove, "session/notes/remove")
		}
	}
	tasksList, taskRead := unavailableTaskList("session/tasks/list"), unavailableRead("session/tasks/read")
	taskCreate, taskUpdate, taskRemove := unavailableCreate("session/tasks/create"), unavailableCreate("session/tasks/update"), unavailableRemove("session/tasks/remove")
	if tasks != nil {
		tasksList = taskListFunction(tasks.Tasks, "session/tasks/list")
		if tasks.Read != nil {
			taskRead = taskReadFunction(tasks.Read, "session/tasks/read")
		}
		if tasks.Create != nil {
			taskCreate = taskCreateFunction(tasks.Create, "session/tasks/create")
		}
		if tasks.Update != nil {
			taskUpdate = taskUpdateFunction(tasks.Update, "session/tasks/update")
		}
		if tasks.Remove != nil {
			taskRemove = taskRemoveFunction(tasks.Remove, "session/tasks/remove")
		}
	}
	secretsList, secretRead := sessionSecretListFunction(nil, "session/secrets/list"), unavailableSecretRead("session/secrets/read")
	if secrets != nil {
		secretsList = sessionSecretListFunction(secrets.Secrets, "session/secrets/list")
	}
	if secrets != nil && secrets.Read != nil {
		secretRead = sessionSecretReadFunction(secrets.Read, "session/secrets/read")
	}
	return lisp.HostModule{ID: gatehouseSessionModuleID, Exports: []lisp.HostExport{
		{Name: "files/list", Value: document(lisp.Function(fileListFunction(files, "session/files/list")), fileListDocumentation)}, {Name: "files/info", Value: document(lisp.FunctionNonLeaky(fileInfoFunction(files, "session/files/info", typed_id.SessionFile, "session")), fileInfoDocumentation)}, {Name: "files/read", Value: document(lisp.Function(fileRead), fileReadDocumentation)}, {Name: "files/create", Value: document(lisp.FunctionWithContext(fileCreate), fileCreateDocumentation)}, {Name: "files/search-file", Value: document(lisp.FunctionNonLeaky(fileSearchFunction), fileSearchDocumentation)}, {Name: "files/update", Value: document(lisp.FunctionNonLeaky(fileUpdate), fileUpdateDocumentation)}, {Name: "files/remove", Value: document(lisp.Function(fileRemove), fileRemoveDocumentation)}, {Name: "events/read", Value: document(lisp.Function(eventReadFunction), sessionEventReadDocumentation)}, {Name: "events/search", Value: document(lisp.FunctionNonLeaky(eventSearchFunction), sessionEventSearchDocumentation)}, {Name: "secrets/list", Value: document(lisp.FunctionNonLeaky(secretsList), sessionSecretListDocumentation)}, {Name: "secrets/read", Value: document(lisp.Function(secretRead), sessionSecretReadDocumentation)}, {Name: "notes/list", Value: document(lisp.Function(notesList), sessionNoteListDocumentation)}, {Name: "notes/read", Value: document(lisp.Function(noteRead), sessionNoteReadDocumentation)}, {Name: "notes/revisions/list", Value: document(lisp.Function(noteRevisionsList), sessionNoteRevisionsListDocumentation)}, {Name: "notes/revisions/read", Value: document(lisp.Function(noteRevisionRead), sessionNoteRevisionReadDocumentation)}, {Name: "notes/create", Value: document(lisp.FunctionNonLeaky(noteCreate), sessionNoteCreateDocumentation)}, {Name: "notes/update", Value: document(lisp.FunctionNonLeaky(noteUpdate), sessionNoteUpdateDocumentation)}, {Name: "notes/remove", Value: document(lisp.Function(noteRemove), sessionNoteRemoveDocumentation)}, {Name: "tasks/list", Value: document(lisp.Function(tasksList), sessionTaskListDocumentation)}, {Name: "tasks/read", Value: document(lisp.Function(taskRead), sessionTaskReadDocumentation)}, {Name: "tasks/create", Value: document(lisp.FunctionNonLeaky(taskCreate), sessionTaskCreateDocumentation)}, {Name: "tasks/update", Value: document(lisp.FunctionNonLeaky(taskUpdate), sessionTaskUpdateDocumentation)}, {Name: "tasks/remove", Value: document(lisp.Function(taskRemove), sessionTaskRemoveDocumentation)},
	}}
}

func NewProjectModule(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModule(info, files, notes, projectTasksArgument(tasks), nil)
}

func NewProjectModuleWithSecrets(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, secrets *ProjectSecrets, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModule(info, files, notes, projectTasksArgument(tasks), secrets)
}

func NewProjectModuleWithRecords(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, records *ProjectRecords, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModuleWithRecords(info, files, notes, projectTasksArgument(tasks), nil, records)
}

func NewProjectModuleWithSecretsAndRecords(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, secrets *ProjectSecrets, records *ProjectRecords, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModuleWithRecords(info, files, notes, projectTasksArgument(tasks), secrets, records)
}

func projectTasksArgument(tasks []*ProjectTasks) *ProjectTasks {
	if len(tasks) == 0 {
		return nil
	}
	return tasks[0]
}

func newProjectModule(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, tasks *ProjectTasks, secrets *ProjectSecrets) lisp.HostModule {
	return newProjectModuleWithRecords(info, files, notes, tasks, secrets, nil)
}

func newProjectModuleWithRecords(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, tasks *ProjectTasks, secrets *ProjectSecrets, records *ProjectRecords) lisp.HostModule {
	infoGet, infoSet := projectInfoGetFunction(info), unavailableCreate("project/info/set")
	if info != nil && info.Set != nil {
		infoSet = projectInfoSetFunction(info.Set)
	}
	filesList, filesInfo, fileRead := fileListFunction(nil, "project/files/list"), fileInfoFunction(nil, "project/files/info", typed_id.ProjectFile, "project"), unavailableRead("project/files/read")
	fileCreate := func(_ *lisp.FunctionContext, _ []lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("project/files/create is unavailable"), nil
	}
	fileUpdate, fileRemove, fileSearch := unavailableCreate("project/files/update"), unavailableRemove("project/files/remove"), unavailableSessionFileSearch("project/files/search-file")
	if files != nil {
		filesList, filesInfo = fileListFunction(files.Files, "project/files/list"), fileInfoFunction(files.Files, "project/files/info", typed_id.ProjectFile, "project")
		if files.Read != nil {
			fileRead = fileReadFunction(files.Read, "project/files/read", typed_id.ProjectFile, "project")
		}
		if files.Create != nil {
			fileCreate = sessionFileCreateFunction(files.Create, "project/files/create")
		}
		if files.Update != nil {
			fileUpdate = fileUpdateFunction(files.Update, "project/files/update", typed_id.ProjectFile, "project")
		}
		if files.Remove != nil {
			fileRemove = fileRemoveFunction(files.Remove, "project/files/remove", typed_id.ProjectFile, "project")
		}
		if files.Search != nil {
			fileSearch = sessionFileSearchFunction(files.Search, "project/files/search-file", typed_id.ProjectFile, "project")
		}
	}
	notesList, noteRead := noteListFunction(nil, "project/notes/list"), unavailableRead("project/notes/read")
	noteRevisionsList, noteRevisionRead := unavailableNoteRevisionList("project/notes/revisions/list"), unavailableRead("project/notes/revisions/read")
	noteCreate, noteUpdate, noteRemove := unavailableCreate("project/notes/create"), unavailableCreate("project/notes/update"), unavailableRemove("project/notes/remove")
	if notes != nil {
		notesList = noteListFunction(notes.Notes, "project/notes/list")
		if notes.Read != nil {
			noteRead = noteReadFunction(notes.Read, "project/notes/read")
		}
		if notes.Revisions != nil {
			noteRevisionsList = noteRevisionListFunction(notes.Revisions, "project/notes/revisions/list")
		}
		if notes.RevisionRead != nil {
			noteRevisionRead = noteRevisionReadFunction(notes.RevisionRead, "project/notes/revisions/read")
		}
		if notes.Create != nil {
			noteCreate = projectNoteCreateFunction(notes.Create, "project/notes/create")
		}
		if notes.Update != nil {
			noteUpdate = projectNoteUpdateFunction(notes.Update, "project/notes/update")
		}
		if notes.Remove != nil {
			noteRemove = noteRemoveFunction(notes.Remove, "project/notes/remove")
		}
	}
	tasksList, taskRead := unavailableTaskList("project/tasks/list"), unavailableRead("project/tasks/read")
	taskCreate, taskUpdate, taskRemove := unavailableCreate("project/tasks/create"), unavailableCreate("project/tasks/update"), unavailableRemove("project/tasks/remove")
	if tasks != nil {
		tasksList = taskListFunction(tasks.Tasks, "project/tasks/list")
		if tasks.Read != nil {
			taskRead = taskReadFunction(tasks.Read, "project/tasks/read")
		}
		if tasks.Create != nil {
			taskCreate = taskCreateFunction(tasks.Create, "project/tasks/create")
		}
		if tasks.Update != nil {
			taskUpdate = taskUpdateFunction(tasks.Update, "project/tasks/update")
		}
		if tasks.Remove != nil {
			taskRemove = taskRemoveFunction(tasks.Remove, "project/tasks/remove")
		}
	}
	secretsList, secretRead := projectSecretListFunction(nil, "project/secrets/list"), unavailableSecretRead("project/secrets/read")
	if secrets != nil {
		secretsList = projectSecretListFunction(secrets.Secrets, "project/secrets/list")
	}
	if secrets != nil && secrets.Read != nil {
		secretRead = projectSecretReadFunction(secrets.Read, "project/secrets/read")
	}
	exports := []lisp.HostExport{
		{Name: "info/get", Value: document(lisp.Function(infoGet), projectInfoGetDocumentation)}, {Name: "info/set", Value: document(lisp.Function(infoSet), projectInfoSetDocumentation)}, {Name: "files/list", Value: document(lisp.Function(filesList), projectFileListDocumentation)}, {Name: "files/info", Value: document(lisp.FunctionNonLeaky(filesInfo), projectFileInfoDocumentation)}, {Name: "files/read", Value: document(lisp.Function(fileRead), projectFileReadDocumentation)}, {Name: "files/create", Value: document(lisp.FunctionWithContext(fileCreate), projectFileCreateDocumentation)}, {Name: "files/update", Value: document(lisp.FunctionNonLeaky(fileUpdate), projectFileUpdateDocumentation)}, {Name: "files/remove", Value: document(lisp.Function(fileRemove), projectFileRemoveDocumentation)}, {Name: "files/search-file", Value: document(lisp.FunctionNonLeaky(fileSearch), projectFileSearchDocumentation)}, {Name: "notes/list", Value: document(lisp.Function(notesList), projectNoteListDocumentation)}, {Name: "notes/read", Value: document(lisp.Function(noteRead), projectNoteReadDocumentation)}, {Name: "notes/revisions/list", Value: document(lisp.Function(noteRevisionsList), projectNoteRevisionsListDocumentation)}, {Name: "notes/revisions/read", Value: document(lisp.Function(noteRevisionRead), projectNoteRevisionReadDocumentation)}, {Name: "notes/create", Value: document(lisp.FunctionNonLeaky(noteCreate), projectNoteCreateDocumentation)}, {Name: "notes/update", Value: document(lisp.FunctionNonLeaky(noteUpdate), projectNoteUpdateDocumentation)}, {Name: "notes/remove", Value: document(lisp.Function(noteRemove), projectNoteRemoveDocumentation)}, {Name: "tasks/list", Value: document(lisp.Function(tasksList), projectTaskListDocumentation)}, {Name: "tasks/read", Value: document(lisp.Function(taskRead), projectTaskReadDocumentation)}, {Name: "tasks/create", Value: document(lisp.FunctionNonLeaky(taskCreate), projectTaskCreateDocumentation)}, {Name: "tasks/update", Value: document(lisp.FunctionNonLeaky(taskUpdate), projectTaskUpdateDocumentation)}, {Name: "tasks/remove", Value: document(lisp.Function(taskRemove), projectTaskRemoveDocumentation)}, {Name: "secrets/list", Value: document(lisp.FunctionNonLeaky(secretsList), projectSecretListDocumentation)}, {Name: "secrets/read", Value: document(lisp.Function(secretRead), projectSecretReadDocumentation)},
	}
	return lisp.HostModule{ID: gatehouseProjectModuleID, Exports: append(exports, projectRecordsExports(records)...)}
}
