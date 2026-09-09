package agent

import (
	"io"
	"strings"

	"gatehouse/lisp"
	"gatehouse/typed_id"
)

const gatehouseSessionModuleID = "native:gatehouse/session/v1"

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

var (
	fileListDocumentation                 = capabilityDocumentation{"(session/files/list) -> List", "Returns successful files in the current session with sfi_ IDs, name, optional media_type, size, and fingerprint.", "(session/files/list)", "((id . \"sfi_example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	fileInfoDocumentation                 = capabilityDocumentation{"(session/files/info id) -> List | Null", "Returns successful current-session file metadata for an sfi_ ID, or null when the file is unavailable.", "(session/files/info \"sfi_example-file-id\")", "((id . \"sfi_example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))"}
	fileReadDocumentation                 = capabilityDocumentation{"(session/files/read id offset length) -> Bytes", "Reads bytes from a successful current-session file with an sfi_ ID. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/files/read \"sfi_example-file-id\" 0 64))", "\"first bytes of the file\""}
	fileCreateDocumentation               = capabilityDocumentation{"(session/files/create name media_type chunks) -> String", "Creates a file from a finite sequence of Bytes chunks and returns its ID. Name and media_type must be non-empty strings.", "(session/files/create \"report.txt\" \"text/plain\" (seq/from (bytes/utf8/encode \"Generated report\")))", "\"example-file-id\""}
	sessionEventReadDocumentation         = capabilityDocumentation{"(session/events/read id offset length) -> Bytes", "Reads UTF-8 bytes from a message text or tool result in the current session. Length must be from 1 through 4096 bytes.", "(bytes/utf8/decode (session/events/read \"example-event-id\" 0 64))", "\"event output\""}
	sessionSecretListDocumentation        = capabilityDocumentation{"(session/secrets/list) -> List", "Returns public metadata for secrets in the current session. Secret values are not included.", "(session/secrets/list)", "((id . \"ssc_0123456789abcdefghjkmnpqrs\") (description . \"Deployment token\") (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionSecretReadDocumentation        = capabilityDocumentation{"(session/secrets/read id) -> Bytes", "Reads an encrypted session secret by ID. The returned Bytes are secret-tainted.", "(session/secrets/read \"ssc_0123456789abcdefghjkmnpqrs\")", "#<secret>"}
	sessionNoteListDocumentation          = capabilityDocumentation{"(session/notes/list) -> List", "Returns notes in the current session with id, title, possibly empty description, current revision sensitivity, author_id, optional author_name, created_at, and revision.", "(session/notes/list)", "((id . \"example-note-id\") (title . \"Guide\") (description . \"How this session works\") (sensitive . #f) (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\") (revision . 1))"}
	sessionNoteReadDocumentation          = capabilityDocumentation{"(session/notes/read id offset length) -> Bytes", "Reads Markdown source from a note in the current session. Sensitive note bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/notes/read \"example-note-id\" 0 64))", "\"# Session guide\""}
	sessionNoteRevisionsListDocumentation = capabilityDocumentation{"(session/notes/revisions/list id) -> List", "Returns revision, title, possibly empty description, sensitivity, author_id, optional author_name, and created_at for an authorized session note, newest first.", "(session/notes/revisions/list \"example-note-id\")", "((revision . 2) (title . \"Decision\") (description . \"Updated reasoning\") (sensitive . #f) (author_id . \"example-agent-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteRevisionReadDocumentation  = capabilityDocumentation{"(session/notes/revisions/read id revision offset length) -> Bytes", "Reads Markdown bytes from an authorized session note revision. Sensitive revision bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/notes/revisions/read \"example-note-id\" 1 0 64))", "\"# Original session guide\""}
	sessionNoteCreateDocumentation        = capabilityDocumentation{"(session/notes/create title description body) -> List", "Creates a shared Markdown note in the current session and returns its metadata. Description and body may be empty strings. A body derived from sensitive data creates a sensitive revision.", "(session/notes/create \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (sensitive . #f) (author_id . \"example-agent-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\") (revision . 1))"}
	sessionNoteUpdateDocumentation        = capabilityDocumentation{"(session/notes/update id title description body) -> List", "Replaces a shared Markdown note in the current session and returns its updated metadata. Title and description must not be sensitive; a sensitive body creates a sensitive revision.", "(session/notes/update \"example-note-id\" \"Decision\" \"Updated reasoning\" \"# Decision\")", "((id . \"example-note-id\") (revision . 2) (title . \"Decision\") (description . \"Updated reasoning\") (sensitive . #f) (author_id . \"example-agent-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionNoteRemoveDocumentation        = capabilityDocumentation{"(session/notes/remove id) -> Boolean", "Removes a shared note from the current session. Returns true when the note was removed and false when it is unavailable.", "(session/notes/remove \"example-note-id\")", "#t"}
	sessionTaskListDocumentation          = capabilityDocumentation{"(session/tasks/list) -> List", "Returns flat task metadata in the current session, including whether the Markdown description is sensitive.", "(session/tasks/list)", "((id . \"stk_example-task-id\") (title . \"Review design\") (sensitive . #f) (status . \"ready\") (creator_id . \"example-agent-id\") (updater_id . \"example-agent-id\") (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionTaskReadDocumentation          = capabilityDocumentation{"(session/tasks/read id offset length) -> Bytes", "Reads the Markdown task description in the current session. Sensitive task bytes are marked sensitive. Length must be from 1 through 65536 bytes.", "(bytes/utf8/decode (session/tasks/read \"stk_example-task-id\" 0 64))", "\"Task details\""}
	sessionTaskCreateDocumentation        = capabilityDocumentation{"(session/tasks/create title description status) -> List", "Creates a flat session task. Title and status must not be sensitive. The Markdown description may be sensitive but must never be secret.", "(session/tasks/create \"Review design\" \"# Review\" \"ready\")", "((id . \"stk_example-task-id\") (title . \"Review design\") (sensitive . #f) (status . \"ready\") (creator_id . \"example-agent-id\") (updater_id . \"example-agent-id\") (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionTaskUpdateDocumentation        = capabilityDocumentation{"(session/tasks/update id title description status) -> List", "Replaces a flat session task. ID, title, and status must not be sensitive. The Markdown description may be sensitive but must never be secret.", "(session/tasks/update \"stk_example-task-id\" \"Review design\" \"# Review\" \"done\")", "((id . \"stk_example-task-id\") (title . \"Review design\") (sensitive . #f) (status . \"done\") (creator_id . \"example-agent-id\") (updater_id . \"example-agent-id\") (created_at . \"2026-01-01T00:00:00.000Z\") (updated_at . \"2026-01-01T00:00:00.000Z\"))"}
	sessionTaskRemoveDocumentation        = capabilityDocumentation{"(session/tasks/remove id) -> Boolean", "Removes a flat task from the current session. Returns true when the task was removed and false when it is unavailable.", "(session/tasks/remove \"stk_example-task-id\")", "#t"}
)

// NewSessionModule constructs the session capability module for one agent evaluation.
func NewSessionModule(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, notes, nil, nil, eventReads...)
}

func NewSessionModuleWithSecrets(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, secrets *SessionSecrets, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, notes, nil, secrets, eventReads...)
}

func NewSessionModuleWithTasks(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, tasks *SessionTasks, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, notes, tasks, nil, eventReads...)
}

func NewSessionModuleWithSecretsAndTasks(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, tasks *SessionTasks, secrets *SessionSecrets, eventReads ...SessionEventRead) lisp.HostModule {
	return newSessionModule(files, read, create, notes, tasks, secrets, eventReads...)
}

func newSessionModule(files []File, read FileRead, create SessionFileCreate, notes *SessionNotes, tasks *SessionTasks, secrets *SessionSecrets, eventReads ...SessionEventRead) lisp.HostModule {
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
	eventRead := unavailableRead("session/events/read")
	if len(eventReads) > 0 && eventReads[0] != nil {
		eventRead = sessionEventReadFunction(eventReads[0], "session/events/read")
	}
	notesList, noteRead := sessionNoteListFunction(nil, "session/notes/list"), unavailableRead("session/notes/read")
	noteRevisionsList, noteRevisionRead := unavailableNoteRevisionList("session/notes/revisions/list"), unavailableRead("session/notes/revisions/read")
	noteCreate := unavailableCreate("session/notes/create")
	noteUpdate := unavailableCreate("session/notes/update")
	noteRemove := unavailableRemove("session/notes/remove")
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
	taskCreate := unavailableCreate("session/tasks/create")
	taskUpdate := unavailableCreate("session/tasks/update")
	taskRemove := unavailableRemove("session/tasks/remove")
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
		{Name: "files/list", Value: document(lisp.Function(fileListFunction(files, "session/files/list")), fileListDocumentation)},
		{Name: "files/info", Value: document(lisp.FunctionNonLeaky(fileInfoFunction(files, "session/files/info", typed_id.SessionFile, "session")), fileInfoDocumentation)},
		{Name: "files/read", Value: document(lisp.Function(fileRead), fileReadDocumentation)},
		{Name: "files/create", Value: document(lisp.FunctionWithContext(fileCreate), fileCreateDocumentation)},
		{Name: "events/read", Value: document(lisp.Function(eventRead), sessionEventReadDocumentation)},
		{Name: "secrets/list", Value: document(lisp.FunctionNonLeaky(secretsList), sessionSecretListDocumentation)},
		{Name: "secrets/read", Value: document(lisp.Function(secretRead), sessionSecretReadDocumentation)},
		{Name: "notes/list", Value: document(lisp.Function(notesList), sessionNoteListDocumentation)},
		{Name: "notes/read", Value: document(lisp.Function(noteRead), sessionNoteReadDocumentation)},
		{Name: "notes/revisions/list", Value: document(lisp.Function(noteRevisionsList), sessionNoteRevisionsListDocumentation)},
		{Name: "notes/revisions/read", Value: document(lisp.Function(noteRevisionRead), sessionNoteRevisionReadDocumentation)},
		{Name: "notes/create", Value: document(lisp.FunctionNonLeaky(noteCreate), sessionNoteCreateDocumentation)},
		{Name: "notes/update", Value: document(lisp.FunctionNonLeaky(noteUpdate), sessionNoteUpdateDocumentation)},
		{Name: "notes/remove", Value: document(lisp.Function(noteRemove), sessionNoteRemoveDocumentation)},
		{Name: "tasks/list", Value: document(lisp.Function(tasksList), sessionTaskListDocumentation)},
		{Name: "tasks/read", Value: document(lisp.Function(taskRead), sessionTaskReadDocumentation)},
		{Name: "tasks/create", Value: document(lisp.FunctionNonLeaky(taskCreate), sessionTaskCreateDocumentation)},
		{Name: "tasks/update", Value: document(lisp.FunctionNonLeaky(taskUpdate), sessionTaskUpdateDocumentation)},
		{Name: "tasks/remove", Value: document(lisp.Function(taskRemove), sessionTaskRemoveDocumentation)},
	}}
}

func sessionNoteUpdateFunction(update SessionNoteUpdate, name string) func([]lisp.Expr) (error, lisp.Expr) {
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
		return sessionNoteValue(note, name)
	}
}

func sessionSecretReadFunction(read SessionSecretRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
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

func unavailableSecretRead(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
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
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone || lisp.TaintOf(arguments[1]) != lisp.TaintNone {
			return lisp.Errorf("%s name and media_type must not be sensitive", name), nil
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
		if lisp.TaintOf(head) != lisp.TaintNone {
			return 0, lisp.Errorf("session/files/create contents must not be sensitive")
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
