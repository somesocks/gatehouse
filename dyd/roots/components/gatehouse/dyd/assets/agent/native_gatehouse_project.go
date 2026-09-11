package agent

import (
	"gatehouse/lisp"
	"gatehouse/typed_id"
)

const gatehouseProjectModuleID = "native:gatehouse/project/v1"

// ProjectInfo describes the project linked to an agent session.
type ProjectInfo struct {
	Name        *string
	Description *string
	CreatedAt   string
	Set         ProjectInfoSet
}

// ProjectInfoSet replaces the name and description of an authorized project.
type ProjectInfoSet func(name, description string) (error, ProjectInfo)

// ProjectFiles contains the authorized files in a linked project.
type ProjectFiles struct {
	Files []File
	Read  FileRead
}

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

// ProjectSecret describes authorized public project secret metadata.
type ProjectSecret struct {
	ID          string
	Description string
	AuthorID    string
	AuthorName  *string
	CreatedAt   string
	UpdatedAt   string
}

var (
	projectFileListDocumentation          = capabilityDocumentation{"(project/files/list) -> List", "Returns successful files in the project linked to the current session with pfi_ IDs, name, optional media_type, size, and fingerprint.", "(project/files/list)", "((id . \"pfi_example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	projectFileInfoDocumentation          = capabilityDocumentation{"(project/files/info id) -> List | Null", "Returns successful linked-project file metadata for a pfi_ ID, or null when the file is unavailable.", "(project/files/info \"pfi_example-file-id\")", "((id . \"pfi_example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	projectInfoGetDocumentation           = capabilityDocumentation{"(project/info/get) -> List | Null", "Returns the linked project's optional name, optional description, and creation time, or null when no authorized project is linked.", "(project/info/get)", "((name . \"Roadmap\") (description) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectInfoSetDocumentation           = capabilityDocumentation{"(project/info/set name description) -> List", "Replaces the linked project's name and description and returns its metadata. Empty strings clear the corresponding value.", "(project/info/set \"Roadmap\" \"Current priorities and plans\")", "((name . \"Roadmap\") (description . \"Current priorities and plans\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectFileReadDocumentation          = capabilityDocumentation{"(project/files/read id offset length) -> Bytes", "Reads bytes from a successful linked-project file with a pfi_ ID. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/files/read \"pfi_example-file-id\" 0 64))", "\"first bytes of the file\""}
	projectSecretListDocumentation        = capabilityDocumentation{"(project/secrets/list) -> List", "Returns public metadata for secrets in the project linked to the current session. Secret values are not included.", "(project/secrets/list)", "((id . \"psc_0123456789abcdefghjkmnpqrs\") (description . \"Deployment token\") (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectSecretReadDocumentation        = capabilityDocumentation{"(project/secrets/read id) -> Bytes", "Reads an encrypted project secret by ID. The returned Bytes are secret-tainted.", "(project/secrets/read \"psc_0123456789abcdefghjkmnpqrs\")", "#<secret>"}
	projectNoteListDocumentation          = capabilityDocumentation{"(project/notes/list) -> List", "Returns project notes with id, title, possibly empty description, current revision sensitivity, author_id, optional author_name, created_at, and revision.", "(project/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this project works\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\") (revision . 1))"}
	projectNoteReadDocumentation          = capabilityDocumentation{"(project/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the project linked to the current session. Sensitive note bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/notes/read \"example-note-id\" 0 64))", "\"# Project guide\""}
	projectNoteRevisionsListDocumentation = capabilityDocumentation{"(project/notes/revisions/list id) -> List", "Returns revision, title, possibly empty description, sensitivity, author_id, optional author_name, and created_at for an authorized project note, newest first.", "(project/notes/revisions/list \"example-note-id\")", "((revision . 2) (title . \"Decision\") (description . \"Updated reasoning\") (sensitive . #f) (author_id . \"example-agent-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectNoteRevisionReadDocumentation  = capabilityDocumentation{"(project/notes/revisions/read id revision offset length) -> Bytes", "Reads Markdown bytes from an authorized project note revision. Sensitive revision bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/notes/revisions/read \"example-note-id\" 1 0 64))", "\"# Original project guide\""}
	projectNoteCreateDocumentation        = capabilityDocumentation{"(project/notes/create title description body) -> List", "Creates a shared Markdown note in the project linked to the current session and returns its metadata. Description and body may be empty strings. A body derived from sensitive data creates a sensitive revision.", "(project/notes/create \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (sensitive . #f) (author_id . \"example-agent-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\") (revision . 1))"}
	projectNoteUpdateDocumentation        = capabilityDocumentation{"(project/notes/update id title description body) -> List", "Replaces a shared Markdown note in the linked project and returns its updated metadata. Title and description must not be sensitive; a sensitive body creates a sensitive revision.", "(project/notes/update \"example-note-id\" \"Decision\" \"Updated reasoning\" \"# Decision\")", "((id . \"example-note-id\") (revision . 2) (title . \"Decision\") (description . \"Updated reasoning\") (sensitive . #f) (author_id . \"example-agent-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectNoteRemoveDocumentation        = capabilityDocumentation{"(project/notes/remove id) -> Boolean", "Removes a shared note from the project linked to the current session. Returns true when the note was removed and false when it is unavailable.", "(project/notes/remove \"example-note-id\")", "#t"}
	projectTaskListDocumentation          = capabilityDocumentation{"(project/tasks/list) -> List", "Returns flat task metadata in the linked project, including whether the Markdown description is sensitive.", "(project/tasks/list)", "((id . \"ptk_example-task-id\") (title . \"Review design\") (sensitive . #f) (status . \"ready\") (creator_id . \"example-agent-id\") (updater_id . \"example-agent-id\") (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectTaskReadDocumentation          = capabilityDocumentation{"(project/tasks/read id offset length) -> Bytes", "Reads the Markdown task description in the project linked to the current session. Sensitive task bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/tasks/read \"ptk_example-task-id\" 0 64))", "\"Task details\""}
	projectTaskCreateDocumentation        = capabilityDocumentation{"(project/tasks/create title description status) -> List", "Creates a flat project task. Title and status must not be sensitive. The Markdown description may be sensitive but must never be secret.", "(project/tasks/create \"Review design\" \"# Review\" \"ready\")", "((id . \"ptk_example-task-id\") (title . \"Review design\") (sensitive . #f) (status . \"ready\") (creator_id . \"example-agent-id\") (updater_id . \"example-agent-id\") (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectTaskUpdateDocumentation        = capabilityDocumentation{"(project/tasks/update id title description status) -> List", "Replaces a flat project task. ID, title, and status must not be sensitive. The Markdown description may be sensitive but must never be secret.", "(project/tasks/update \"ptk_example-task-id\" \"Review design\" \"# Review\" \"done\")", "((id . \"ptk_example-task-id\") (title . \"Review design\") (sensitive . #f) (status . \"done\") (creator_id . \"example-agent-id\") (updater_id . \"example-agent-id\") (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectTaskRemoveDocumentation        = capabilityDocumentation{"(project/tasks/remove id) -> Boolean", "Removes a flat task from the linked project. Returns true when the task was removed and false when it is unavailable.", "(project/tasks/remove \"ptk_example-task-id\")", "#t"}
)

// NewProjectModule constructs the project capability module for one agent evaluation.
// It always exposes every project export so an unavailable project can be handled in Lisp.
func NewProjectModule(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModule(info, files, notes, projectTasksArgument(tasks), nil)
}

func NewProjectModuleWithSecrets(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, secrets *ProjectSecrets, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModule(info, files, notes, projectTasksArgument(tasks), secrets)
}

// NewProjectModuleWithRecords constructs the project module with record capabilities.
func NewProjectModuleWithRecords(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes, records *ProjectRecords, tasks ...*ProjectTasks) lisp.HostModule {
	return newProjectModuleWithRecords(info, files, notes, projectTasksArgument(tasks), nil, records)
}

// NewProjectModuleWithSecretsAndRecords constructs the project module with secret and record capabilities.
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
	infoGet := projectInfoGetFunction(info)
	infoSet := unavailableCreate("project/info/set")
	if info != nil && info.Set != nil {
		infoSet = projectInfoSetFunction(info.Set)
	}

	filesList, filesInfo, fileRead := fileListFunction(nil, "project/files/list"), fileInfoFunction(nil, "project/files/info", typed_id.ProjectFile, "project"), unavailableRead("project/files/read")
	if files != nil {
		filesList = fileListFunction(files.Files, "project/files/list")
		filesInfo = fileInfoFunction(files.Files, "project/files/info", typed_id.ProjectFile, "project")
		if files.Read != nil {
			fileRead = fileReadFunction(files.Read, "project/files/read", typed_id.ProjectFile, "project")
		}
	}
	notesList, noteRead := noteListFunction(nil, "project/notes/list"), unavailableRead("project/notes/read")
	noteRevisionsList, noteRevisionRead := unavailableNoteRevisionList("project/notes/revisions/list"), unavailableRead("project/notes/revisions/read")
	noteCreate := unavailableCreate("project/notes/create")
	noteUpdate := unavailableCreate("project/notes/update")
	noteRemove := unavailableRemove("project/notes/remove")
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
	taskCreate := unavailableCreate("project/tasks/create")
	taskUpdate := unavailableCreate("project/tasks/update")
	taskRemove := unavailableRemove("project/tasks/remove")
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
		{Name: "info/get", Value: document(lisp.Function(infoGet), projectInfoGetDocumentation)},
		{Name: "info/set", Value: document(lisp.Function(infoSet), projectInfoSetDocumentation)},
		{Name: "files/list", Value: document(lisp.Function(filesList), projectFileListDocumentation)},
		{Name: "files/info", Value: document(lisp.FunctionNonLeaky(filesInfo), projectFileInfoDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), projectFileReadDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), projectNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), projectNoteReadDocumentation)},
		{Name: "notes/revisions/list", Value: document(lisp.Function(noteRevisionsList), projectNoteRevisionsListDocumentation)},
		{Name: "notes/revisions/read", Value: document(lisp.Function(noteRevisionRead), projectNoteRevisionReadDocumentation)},
		{Name: "notes/create", Value: document(lisp.FunctionNonLeaky(noteCreate), projectNoteCreateDocumentation)},
		{Name: "notes/update", Value: document(lisp.FunctionNonLeaky(noteUpdate), projectNoteUpdateDocumentation)},
		{Name: "notes/remove", Value: document(lisp.Function(noteRemove), projectNoteRemoveDocumentation)},
		{Name: "tasks/list", Value: document(lisp.Function(tasksList), projectTaskListDocumentation)},
		{Name: "tasks/read", Value: document(lisp.Function(taskRead), projectTaskReadDocumentation)},
		{Name: "tasks/create", Value: document(lisp.FunctionNonLeaky(taskCreate), projectTaskCreateDocumentation)},
		{Name: "tasks/update", Value: document(lisp.FunctionNonLeaky(taskUpdate), projectTaskUpdateDocumentation)},
		{Name: "tasks/remove", Value: document(lisp.Function(taskRemove), projectTaskRemoveDocumentation)},
		{Name: "secrets/list", Value: document(lisp.FunctionNonLeaky(secretsList), projectSecretListDocumentation)},
		{Name: "secrets/read", Value: document(lisp.Function(secretRead), projectSecretReadDocumentation)},
	}
	exports = append(exports, projectRecordsExports(records)...)
	return lisp.HostModule{ID: gatehouseProjectModuleID, Exports: exports}
}

func projectNoteUpdateFunction(update ProjectNoteUpdate, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 4 {
			return lisp.Errorf("%s requires id, title, description, and body", name), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone || lisp.TaintOf(arguments[1]) != lisp.TaintNone || lisp.TaintOf(arguments[2]) != lisp.TaintNone {
			return lisp.Errorf("%s id, title, and description must not be sensitive", name), nil
		}
		if lisp.IsSecret(arguments[3]) {
			return lisp.Errorf("%s body must not be secret", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, title := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, description := lisp.RequireString(arguments[2])
		if err != nil {
			return err, nil
		}
		err, body := lisp.RequireString(arguments[3])
		if err != nil {
			return err, nil
		}
		err, note := update(id, title, description, body, lisp.IsSensitive(arguments[3]))
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return noteValue(note, name)
	}
}

func projectSecretListFunction(secrets []ProjectSecret, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(secrets))
		for _, secret := range secrets {
			err, value := projectSecretValue(secret, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func projectSecretValue(secret ProjectSecret, name string) (error, lisp.Expr) {
	if secret.ID == "" || secret.Description == "" || secret.AuthorID == "" || secret.CreatedAt == "" || secret.UpdatedAt == "" {
		return lisp.Errorf("%s has invalid secret metadata", name), nil
	}
	authorName := lisp.Null()
	if secret.AuthorName != nil {
		authorName = lisp.String(*secret.AuthorName)
	}
	return nil, lisp.List(
		lisp.Pair("id", lisp.String(secret.ID)),
		lisp.Pair("description", lisp.String(secret.Description)),
		lisp.Pair("author_id", lisp.String(secret.AuthorID)),
		lisp.Pair("author_name", authorName),
		lisp.Pair("created_at", lisp.String(secret.CreatedAt)),
		lisp.Pair("updated_at", lisp.String(secret.UpdatedAt)),
	)
}

func projectSecretReadFunction(read ProjectSecretRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("%s requires an id", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		if id == "" {
			return lisp.Errorf("%s requires a non-empty id", name), nil
		}
		err, value := read(id)
		if err != nil {
			return lisp.Errorf("%s is unavailable", name), nil
		}
		result := lisp.MarkSecret(lisp.Bytes(value))
		clear(value)
		return nil, result
	}
}

func projectNoteCreateFunction(create ProjectNoteCreate, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("%s requires title, description, and body", name), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone || lisp.TaintOf(arguments[1]) != lisp.TaintNone {
			return lisp.Errorf("%s title and description must not be sensitive", name), nil
		}
		if lisp.IsSecret(arguments[2]) {
			return lisp.Errorf("%s body must not be secret", name), nil
		}
		sensitive := lisp.IsSensitive(arguments[2])
		err, title := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, description := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, body := lisp.RequireString(arguments[2])
		if err != nil {
			return err, nil
		}
		err, note := create(title, description, body, sensitive)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return noteValue(note, name)
	}
}

func projectInfoGetFunction(info *ProjectInfo) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/info/get requires no arguments"), nil
		}
		if info == nil {
			return nil, lisp.Null()
		}
		return projectInfoValue(*info, "project/info/get")
	}
}

func projectInfoSetFunction(set ProjectInfoSet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/info/set requires name and description"), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone || lisp.TaintOf(arguments[1]) != lisp.TaintNone {
			return lisp.Errorf("project/info/set name and description must not be sensitive"), nil
		}
		err, name := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, description := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, info := set(name, description)
		if err != nil {
			return lisp.Errorf("project/info/set failed"), nil
		}
		return projectInfoValue(info, "project/info/set")
	}
}

func projectInfoValue(info ProjectInfo, name string) (error, lisp.Expr) {
	if info.CreatedAt == "" {
		return lisp.Errorf("%s has invalid project metadata", name), nil
	}
	projectName, description := lisp.Null(), lisp.Null()
	if info.Name != nil {
		projectName = lisp.String(*info.Name)
	}
	if info.Description != nil {
		description = lisp.String(*info.Description)
	}
	return nil, lisp.List(
		lisp.Pair("name", projectName),
		lisp.Pair("description", description),
		lisp.Pair("created_at", lisp.String(info.CreatedAt)),
	)
}
