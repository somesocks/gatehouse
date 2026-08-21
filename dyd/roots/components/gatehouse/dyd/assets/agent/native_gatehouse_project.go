package agent

import "gatehouse/lisp"

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
	Notes  []ProjectNote
	Read   ProjectNoteRead
	Create ProjectNoteCreate
	Remove NoteRemove
}

var (
	projectFileListDocumentation = capabilityDocumentation{"(project/files/list) -> List", "Returns successful files in the project linked to the current session with id, name, optional media_type, size, and fingerprint.", "(project/files/list)", "((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	projectInfoGetDocumentation  = capabilityDocumentation{"(project/info/get) -> List | Null", "Returns the linked project's optional name, optional description, and creation time, or null when no authorized project is linked.", "(project/info/get)", "((name . \"Roadmap\") (description) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectInfoSetDocumentation  = capabilityDocumentation{"(project/info/set name description) -> List", "Replaces the linked project's name and description and returns its metadata. Empty strings clear the corresponding value.", "(project/info/set \"Roadmap\" \"Current priorities and plans\")", "((name . \"Roadmap\") (description . \"Current priorities and plans\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectFileReadDocumentation = capabilityDocumentation{"(project/files/read id offset length) -> Bytes", "Reads bytes from a successful file in the project linked to the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/files/read \"example-file-id\" 0 64))", "\"first bytes of the file\""}
	projectNoteListDocumentation = capabilityDocumentation{"(project/notes/list) -> List", "Returns project notes with id, title, possibly empty description, sensitivity, author_id, optional author_name, and created_at.", "(project/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this project works\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectNoteReadDocumentation = capabilityDocumentation{"(project/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the project linked to the current session. Sensitive note bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/notes/read \"example-note-id\" 0 64))", "\"# Project guide\""}
	projectNoteCreateDocumentation = capabilityDocumentation{"(project/notes/create title description body) -> List", "Creates a shared Markdown note in the project linked to the current session and returns its metadata. Description and body may be empty strings. A body derived from sensitive data creates a sensitive note.", "(project/notes/create \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectNoteRemoveDocumentation = capabilityDocumentation{"(project/notes/remove id) -> Boolean", "Removes a shared note from the project linked to the current session. Returns true when the note was removed and false when it is unavailable.", "(project/notes/remove \"example-note-id\")", "#t"}
)

// NewProjectModule constructs the project capability module for one agent evaluation.
// It always exposes every project export so an unavailable project can be handled in Lisp.
func NewProjectModule(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes) lisp.HostModule {
	infoGet := projectInfoGetFunction(info)
	infoSet := unavailableCreate("project/info/set")
	if info != nil && info.Set != nil {
		infoSet = projectInfoSetFunction(info.Set)
	}

	filesList, fileRead := fileListFunction(nil, "project/files/list"), unavailableRead("project/files/read")
	if files != nil {
		filesList = fileListFunction(files.Files, "project/files/list")
		if files.Read != nil {
			fileRead = fileReadFunction(files.Read, "project/files/read")
		}
	}
	notesList, noteRead := noteListFunction(nil, "project/notes/list"), unavailableRead("project/notes/read")
	noteCreate := unavailableCreate("project/notes/create")
	noteRemove := unavailableRemove("project/notes/remove")
	if notes != nil {
		notesList = noteListFunction(notes.Notes, "project/notes/list")
		if notes.Read != nil {
			noteRead = noteReadFunction(notes.Read, "project/notes/read")
		}
		if notes.Create != nil {
			noteCreate = projectNoteCreateFunction(notes.Create, "project/notes/create")
		}
		if notes.Remove != nil {
			noteRemove = noteRemoveFunction(notes.Remove, "project/notes/remove")
		}
	}

	return lisp.HostModule{ID: gatehouseProjectModuleID, Exports: []lisp.HostExport{
		{Name: "info/get", Value: document(lisp.Function(infoGet), projectInfoGetDocumentation)},
		{Name: "info/set", Value: document(lisp.Function(infoSet), projectInfoSetDocumentation)},
		{Name: "files/list", Value: document(lisp.Function(filesList), projectFileListDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), projectFileReadDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), projectNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), projectNoteReadDocumentation)},
		{Name: "notes/create", Value: document(lisp.Function(noteCreate), projectNoteCreateDocumentation)},
		{Name: "notes/remove", Value: document(lisp.Function(noteRemove), projectNoteRemoveDocumentation)},
	}}
}

func projectNoteCreateFunction(create ProjectNoteCreate, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("%s requires title, description, and body", name), nil
		}
		if lisp.IsSecret(arguments[0]) || lisp.IsSecret(arguments[1]) {
			return lisp.Errorf("%s title and description must not be sensitive", name), nil
		}
		sensitive := lisp.IsSecret(arguments[2])
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
