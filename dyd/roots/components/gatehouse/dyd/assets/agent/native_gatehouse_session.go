package agent

import (
	"io"
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
	fileInfoDocumentation          = capabilityDocumentation{"(session/files/info id) -> List | Null", "Returns successful current-session file metadata by ID, or null when the file is unavailable.", "(session/files/info \"example-file-id\")", "((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	fileReadDocumentation          = capabilityDocumentation{"(session/files/read id offset length) -> Bytes", "Reads bytes from a successful file in the current session. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/files/read \"example-file-id\" 0 64))", "\"first bytes of the file\""}
	fileCreateDocumentation        = capabilityDocumentation{"(session/files/create name media_type chunks) -> String", "Creates a file from a finite sequence of Bytes chunks and returns its ID. Name and media_type must be non-empty strings.", "(session/files/create \"report.txt\" \"text/plain\" (seq/from (bytes/utf8/encode \"Generated report\")))", "\"example-file-id\""}
	sessionEventReadDocumentation  = capabilityDocumentation{"(session/events/read id offset length) -> Bytes", "Reads UTF-8 bytes from a message text or tool result in the current session. Length must be from 1 through 4096 bytes.", "(bytes/utf8/decode (session/events/read \"example-event-id\" 0 64))", "\"event output\""}
	sessionNoteListDocumentation   = capabilityDocumentation{"(session/notes/list) -> List", "Returns notes in the current session with id, title, possibly empty description, sensitivity, author_id, optional author_name, and created_at.", "(session/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this session works\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteReadDocumentation   = capabilityDocumentation{"(session/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the current session. Sensitive note bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/notes/read \"example-note-id\" 0 64))", "\"# Session guide\""}
	sessionNoteCreateDocumentation = capabilityDocumentation{"(session/notes/create title description body) -> List", "Creates a shared Markdown note in the current session and returns its metadata. Description and body may be empty strings. A body derived from sensitive data creates a sensitive note.", "(session/notes/create \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteRemoveDocumentation = capabilityDocumentation{"(session/notes/remove id) -> Boolean", "Removes a shared note from the current session. Returns true when the note was removed and false when it is unavailable.", "(session/notes/remove \"example-note-id\")", "#t"}
)

// NewSessionModule constructs the session capability module for one agent evaluation.
func NewSessionModule(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, eventReads ...SessionEventRead) lisp.HostModule {
	fileRead := unavailableRead("session/files/read")
	if read != nil {
		fileRead = fileReadFunction(read, "session/files/read")
	}
	fileCreate := func(_ *lisp.FunctionContext, _ []lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("session/files/create is unavailable"), nil
	}
	if create != nil {
		fileCreate = sessionFileCreateFunction(create, "session/files/create")
	}
	eventRead := unavailableRead("session/events/read")
	if len(eventReads) > 0 && eventReads[0] != nil {
		eventRead = sessionEventReadFunction(eventReads[0], "session/events/read")
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
		{Name: "files/info", Value: document(lisp.Function(fileInfoFunction(files, "session/files/info")), fileInfoDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), fileReadDocumentation)},
		{Name: "files/create", Value: document(lisp.FunctionWithContext(fileCreate), fileCreateDocumentation)},
		{Name: "events/read", Value: document(lisp.Function(eventRead), sessionEventReadDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), sessionNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), sessionNoteReadDocumentation)},
		{Name: "notes/create", Value: document(lisp.Function(noteCreate), sessionNoteCreateDocumentation)},
		{Name: "notes/remove", Value: document(lisp.Function(noteRemove), sessionNoteRemoveDocumentation)},
	}}
}

func sessionEventReadFunction(read SessionEventRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
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
		if id == "" || offset < 0 || length < 1 || length > 4096 {
			return lisp.Errorf("%s requires a non-empty id, non-negative offset, and length from 1 through 4096", name), nil
		}
		err, value := read(id, offset, length)
		if err != nil {
			return lisp.Errorf("%s is unavailable", name), nil
		}
		return nil, lisp.Bytes(value)
	}
}

func sessionFileCreateFunction(create SessionFileCreate, name string) func(*lisp.FunctionContext, []lisp.Expr) (error, lisp.Expr) {
	return func(context *lisp.FunctionContext, arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("%s requires name, media_type, and chunks", name), nil
		}
		err, fileName := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, mediaType := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		if strings.TrimSpace(fileName) == "" || strings.TrimSpace(mediaType) == "" {
			return lisp.Errorf("%s requires non-empty name and media_type", name), nil
		}
		err, id := create(fileName, mediaType, &sessionFileSequenceReader{context: context, sequence: arguments[2]})
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return nil, lisp.String(id)
	}
}

type sessionFileSequenceReader struct {
	context        *lisp.FunctionContext
	sequence, tail lisp.Expr
	remaining      []byte
	done           bool
}

func (reader *sessionFileSequenceReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	for len(reader.remaining) == 0 {
		if reader.done {
			return 0, io.EOF
		}
		if reader.tail != nil {
			err, sequence := reader.context.Call(reader.tail)
			if err != nil {
				return 0, err
			}
			reader.sequence, reader.tail = sequence, nil
		}
		if lisp.IsNull(reader.sequence) {
			reader.done = true
			return 0, io.EOF
		}
		head, tail, ok := lisp.DeconstructPair(reader.sequence)
		if !ok {
			return 0, lisp.Errorf("session/files/create requires a sequence")
		}
		err, contents := lisp.RequireBytes(head)
		if err != nil {
			return 0, err
		}
		reader.remaining, reader.tail = contents, tail
	}
	count := copy(destination, reader.remaining)
	clear(reader.remaining[:count])
	reader.remaining = reader.remaining[count:]
	return count, nil
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
