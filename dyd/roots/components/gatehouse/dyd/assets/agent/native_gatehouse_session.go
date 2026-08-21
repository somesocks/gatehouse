package agent

import (
	"strings"

	"gatehouse/lisp"
)

const gatehouseSessionModuleID = "native:gatehouse/session/v1"

// SessionNotes contains the authorized notes in the current session.
type SessionNotes struct {
	Notes  []SessionNote
	Read   SessionNoteRead
	Create SessionNoteCreate
	Remove NoteRemove
}

var (
	fileListDocumentation          = capabilityDocumentation{"(session/files/list) -> List", "Returns successful files in the current session with id, name, optional media_type, size, and fingerprint.", "(session/files/list)", "((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	fileReadDocumentation          = capabilityDocumentation{"(session/files/read id offset length) -> Bytes", "Reads bytes from a successful file in the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/files/read \"example-file-id\" 0 64))", "\"first bytes of the file\""}
	fileCreateDocumentation        = capabilityDocumentation{"(session/files/create name media_type contents) -> String", "Creates a file in the current session and returns its ID. Name and media_type must be non-empty strings.", "(session/files/create \"report.txt\" \"text/plain\" (bytes/utf8/encode \"Generated report\"))", "\"example-file-id\""}
	sessionNoteListDocumentation   = capabilityDocumentation{"(session/notes/list) -> List", "Returns notes in the current session with id, title, possibly empty description, sensitivity, author_id, optional author_name, and created_at.", "(session/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this session works\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteReadDocumentation   = capabilityDocumentation{"(session/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the current session. Sensitive note bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/notes/read \"example-note-id\" 0 64))", "\"# Session guide\""}
	sessionNoteCreateDocumentation = capabilityDocumentation{"(session/notes/create title description body) -> List", "Creates a shared Markdown note in the current session and returns its metadata. Description and body may be empty strings. A body derived from sensitive data creates a sensitive note.", "(session/notes/create \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteRemoveDocumentation = capabilityDocumentation{"(session/notes/remove id) -> Boolean", "Removes a shared note from the current session. Returns true when the note was removed and false when it is unavailable.", "(session/notes/remove \"example-note-id\")", "#t"}
)

// NewSessionModule constructs the session capability module for one agent evaluation.
func NewSessionModule(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes) lisp.HostModule {
	fileRead := unavailableRead("session/files/read")
	if read != nil {
		fileRead = fileReadFunction(read, "session/files/read")
	}
	fileCreate := unavailableCreate("session/files/create")
	if create != nil {
		fileCreate = sessionFileCreateFunction(create, "session/files/create")
	}
	notesList, noteRead := sessionNoteListFunction(nil, "session/notes/list"), unavailableRead("session/notes/read")
	noteCreate := unavailableCreate("session/notes/create")
	noteRemove := unavailableRemove("session/notes/remove")
	if notes != nil {
		notesList = sessionNoteListFunction(notes.Notes, "session/notes/list")
		if notes.Read != nil {
			noteRead = noteReadFunction(notes.Read, "session/notes/read")
		}
		if notes.Create != nil {
			noteCreate = sessionNoteCreateFunction(notes.Create, "session/notes/create")
		}
		if notes.Remove != nil {
			noteRemove = noteRemoveFunction(notes.Remove, "session/notes/remove")
		}
	}

	return lisp.HostModule{ID: gatehouseSessionModuleID, Exports: []lisp.HostExport{
		{Name: "files/list", Value: document(lisp.Function(fileListFunction(files, "session/files/list")), fileListDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), fileReadDocumentation)},
		{Name: "files/create", Value: document(lisp.Function(fileCreate), fileCreateDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), sessionNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), sessionNoteReadDocumentation)},
		{Name: "notes/create", Value: document(lisp.Function(noteCreate), sessionNoteCreateDocumentation)},
		{Name: "notes/remove", Value: document(lisp.Function(noteRemove), sessionNoteRemoveDocumentation)},
	}}
}

func sessionFileCreateFunction(create SessionFileCreate, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("%s requires name, media_type, and contents", name), nil
		}
		err, fileName := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, mediaType := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, contents := lisp.RequireBytes(arguments[2])
		if err != nil {
			return err, nil
		}
		if strings.TrimSpace(fileName) == "" || strings.TrimSpace(mediaType) == "" {
			return lisp.Errorf("%s requires non-empty name and media_type", name), nil
		}
		defer clear(contents)
		err, id := create(fileName, mediaType, contents)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return nil, lisp.String(id)
	}
}

func noteRemoveFunction(remove NoteRemove, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("%s requires id", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		if id == "" {
			return lisp.Errorf("%s requires a non-empty id", name), nil
		}
		err, removed := remove(id)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return nil, lisp.Boolean(removed)
	}
}

func sessionNoteCreateFunction(create SessionNoteCreate, name string) func([]lisp.Expr) (error, lisp.Expr) {
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
		return sessionNoteValue(note, name)
	}
}

func unavailableCreate(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
}

func unavailableRemove(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
}
