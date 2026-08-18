package agent

import "gatehouse/lisp"

const (
	gatehouseProjectModuleID = "native:gatehouse/project/v1"
	gatehouseSessionModuleID = "native:gatehouse/session/v1"
)

// File describes a file that an agent is authorized to inspect.
type File struct {
	ID          string
	Name        string
	MediaType   *string
	Size        int64
	Fingerprint string
}

// FileRead reads an authorized byte range from a file or note.
type FileRead func(id string, offset, length int64) (error, []byte)

// ProjectInfo describes the project linked to an agent session.
type ProjectInfo struct {
	Name        *string
	Description *string
	CreatedAt   string
}

// ProjectFiles contains the authorized files in a linked project.
type ProjectFiles struct {
	Files []File
	Read  FileRead
}

// ProjectNote describes an authorized project or session note.
type ProjectNote struct {
	ID         string
	Title      string
	Description string
	AuthorID   string
	AuthorName *string
	CreatedAt  string
}

// ProjectNotes contains the authorized notes in a linked project.
type ProjectNotes struct {
	Notes []ProjectNote
	Read  FileRead
}

// SessionNotes contains the authorized notes in the current session.
type SessionNotes struct {
	Notes []ProjectNote
	Read  FileRead
}

type capabilityDocumentation struct {
	signature   string
	description string
	example     string
	result      string
}

var (
	fileListDocumentation = capabilityDocumentation{"(session/files/list) -> List", "Returns successful files in the current session with id, name, optional media_type, size, and fingerprint.", "(session/files/list)", "((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	fileReadDocumentation = capabilityDocumentation{"(session/files/read id offset length) -> Bytes", "Reads bytes from a successful file in the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/files/read \"example-file-id\" 0 64))", "\"first bytes of the file\""}

	projectFileListDocumentation = capabilityDocumentation{"(project/files/list) -> List", "Returns successful files in the project linked to the current session with id, name, optional media_type, size, and fingerprint.", "(project/files/list)", "((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	projectInfoDocumentation     = capabilityDocumentation{"(project/info) -> List | Null", "Returns the linked project's optional name, optional description, and creation time, or null when no authorized project is linked.", "(project/info)", "((name . \"Roadmap\") (description) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectFileReadDocumentation = capabilityDocumentation{"(project/files/read id offset length) -> Bytes", "Reads bytes from a successful file in the project linked to the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/files/read \"example-file-id\" 0 64))", "\"first bytes of the file\""}

	projectNoteListDocumentation = capabilityDocumentation{"(project/notes/list) -> List", "Returns project notes with id, title, possibly empty description, author_id, optional author_name, and created_at.", "(project/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this project works\") (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectNoteReadDocumentation = capabilityDocumentation{"(project/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the project linked to the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (project/notes/read \"example-note-id\" 0 64))", "\"# Project guide\""}
	sessionNoteListDocumentation = capabilityDocumentation{"(session/notes/list) -> List", "Returns notes in the current session with id, title, possibly empty description, author_id, optional author_name, and created_at.", "(session/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this session works\") (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteReadDocumentation = capabilityDocumentation{"(session/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/notes/read \"example-note-id\" 0 64))", "\"# Session guide\""}
)

// NewProjectModule constructs the project capability module for one agent evaluation.
// It always exposes every project export so an unavailable project can be handled in Lisp.
func NewProjectModule(info *ProjectInfo, files *ProjectFiles, notes *ProjectNotes) lisp.HostModule {
	infoCall := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/info requires no arguments"), nil
		}
		if info == nil {
			return nil, lisp.Null()
		}
		if info.CreatedAt == "" {
			return lisp.Errorf("project/info has invalid project metadata"), nil
		}
		name, description := lisp.Null(), lisp.Null()
		if info.Name != nil {
			name = lisp.String(*info.Name)
		}
		if info.Description != nil {
			description = lisp.String(*info.Description)
		}
		return nil, lisp.List(
			lisp.Pair("name", name),
			lisp.Pair("description", description),
			lisp.Pair("created_at", lisp.String(info.CreatedAt)),
		)
	}

	filesList, fileRead := fileListFunction(nil, "project/files/list"), unavailableRead("project/files/read")
	if files != nil {
		filesList = fileListFunction(files.Files, "project/files/list")
		if files.Read != nil {
			fileRead = fileReadFunction(files.Read, "project/files/read")
		}
	}
	notesList, noteRead := noteListFunction(nil, "project/notes/list"), unavailableRead("project/notes/read")
	if notes != nil {
		notesList = noteListFunction(notes.Notes, "project/notes/list")
		if notes.Read != nil {
			noteRead = fileReadFunction(notes.Read, "project/notes/read")
		}
	}

	return lisp.HostModule{ID: gatehouseProjectModuleID, Exports: []lisp.HostExport{
		{Name: "info", Value: document(lisp.Function(infoCall), projectInfoDocumentation)},
		{Name: "files/list", Value: document(lisp.Function(filesList), projectFileListDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), projectFileReadDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), projectNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), projectNoteReadDocumentation)},
	}}
}

// NewSessionModule constructs the session capability module for one agent evaluation.
func NewSessionModule(files []File, read FileRead, notes *SessionNotes) lisp.HostModule {
	fileRead := unavailableRead("session/files/read")
	if read != nil {
		fileRead = fileReadFunction(read, "session/files/read")
	}
	notesList, noteRead := noteListFunction(nil, "session/notes/list"), unavailableRead("session/notes/read")
	if notes != nil {
		notesList = noteListFunction(notes.Notes, "session/notes/list")
		if notes.Read != nil {
			noteRead = fileReadFunction(notes.Read, "session/notes/read")
		}
	}

	return lisp.HostModule{ID: gatehouseSessionModuleID, Exports: []lisp.HostExport{
		{Name: "files/list", Value: document(lisp.Function(fileListFunction(files, "session/files/list")), fileListDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), fileReadDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), sessionNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), sessionNoteReadDocumentation)},
	}}
}

func document(expression lisp.Expr, documentation capabilityDocumentation) lisp.Expr {
	return lisp.Document(expression, documentation.signature, documentation.description, documentation.example, documentation.result)
}

func fileListFunction(files []File, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(files))
		for _, file := range files {
			if file.ID == "" || file.Name == "" || file.Size < 0 || file.Fingerprint == "" {
				return lisp.Errorf("%s has invalid file metadata", name), nil
			}
			mediaType := lisp.Null()
			if file.MediaType != nil {
				mediaType = lisp.String(*file.MediaType)
			}
			values = append(values, lisp.List(
				lisp.Pair("id", lisp.String(file.ID)),
				lisp.Pair("name", lisp.String(file.Name)),
				lisp.Pair("media_type", mediaType),
				lisp.Pair("size", lisp.Integer(file.Size)),
				lisp.Pair("fingerprint", lisp.String(file.Fingerprint)),
			))
		}
		return nil, lisp.List(values...)
	}
}

func noteListFunction(notes []ProjectNote, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(notes))
		for _, note := range notes {
			if note.ID == "" || note.Title == "" || note.AuthorID == "" || note.CreatedAt == "" {
				return lisp.Errorf("%s has invalid note metadata", name), nil
			}
			authorName := lisp.Null()
			if note.AuthorName != nil {
				authorName = lisp.String(*note.AuthorName)
			}
			values = append(values, lisp.List(
				lisp.Pair("id", lisp.String(note.ID)),
				lisp.Pair("title", lisp.String(note.Title)),
				lisp.Pair("description", lisp.String(note.Description)),
				lisp.Pair("author_id", lisp.String(note.AuthorID)),
				lisp.Pair("author_name", authorName),
				lisp.Pair("created_at", lisp.String(note.CreatedAt)),
			))
		}
		return nil, lisp.List(values...)
	}
}

func fileReadFunction(read FileRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("%s requires id, offset, and length", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, offset := lisp.RequireInteger(arguments[1])
		if err != nil {
			return err, nil
		}
		err, length := lisp.RequireInteger(arguments[2])
		if err != nil {
			return err, nil
		}
		if id == "" || offset < 0 || length < 1 || length > 64*1024 {
			return lisp.Errorf("%s requires a non-empty id, non-negative offset, and length from 1 through 65536", name), nil
		}
		err, value := read(id, offset, length)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return nil, lisp.Bytes(value)
	}
}

func unavailableRead(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
}
