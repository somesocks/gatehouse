package agent

import (
	"io"

	"gatehouse/lisp"
	"gatehouse/sessionsearch"
	"gatehouse/typed_id"
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

// NoteRead reads an authorized note byte range and reports whether it is sensitive.
type NoteRead func(id string, offset, length int64) (error, []byte, bool)

// ProjectNoteRead reads an authorized project note byte range and reports whether it is sensitive.
type ProjectNoteRead = NoteRead

// SessionNoteRead reads an authorized session note byte range and reports whether it is sensitive.
type SessionNoteRead = NoteRead

// NoteRevisionRead reads an authorized historical note revision byte range and reports whether it is sensitive.
type NoteRevisionRead func(id string, revision int, offset, length int64) (error, []byte, bool)

// ProjectNoteRevisionRead reads an authorized historical project note revision byte range.
type ProjectNoteRevisionRead = NoteRevisionRead

// SessionNoteRevisionRead reads an authorized historical session note revision byte range.
type SessionNoteRevisionRead = NoteRevisionRead

// SessionSecretRead reads an authorized session secret value.
type SessionSecretRead func(id string) (error, []byte)

// SessionFileCreate creates a file in the current session and returns its ID.
type SessionFileCreate func(name, mediaType string, source io.Reader) (error, string)

// SessionEventRead reads an authorized byte range from a current-session event.
type SessionEventRead func(id string, offset, length int64) (error, []byte)

// SessionEventSearchResult is one bounded page of current-session event matches.
type SessionEventSearchResult struct {
	Events     []sessionsearch.EventResult
	NextCursor string
}

// SessionEventSearch searches authorized current-session event bodies.
type SessionEventSearch func(expression, cursor string) (error, SessionEventSearchResult)

// ProjectNoteCreate creates an authorized project note.
type ProjectNoteCreate func(title, description, body string, sensitive bool) (error, ProjectNote)

// SessionNoteCreate creates an authorized session note.
type SessionNoteCreate func(title, description, body string, sensitive bool) (error, SessionNote)

// ProjectNoteUpdate replaces an authorized project note.
type ProjectNoteUpdate func(id, title, description, body string, sensitive bool) (error, ProjectNote)

// SessionNoteUpdate replaces an authorized session note.
type SessionNoteUpdate func(id, title, description, body string, sensitive bool) (error, SessionNote)

// ProjectNoteRevisionsGet returns authorized project note revision summaries.
type ProjectNoteRevisionsGet func(id string) (error, []NoteRevision)

// SessionNoteRevisionsGet returns authorized session note revision summaries.
type SessionNoteRevisionsGet func(id string) (error, []NoteRevision)

// SessionSecrets contains the authorized secrets in the current session.
type SessionSecrets struct {
	Secrets []SessionSecret
	Read    SessionSecretRead
}

// SessionSecret describes authorized public session secret metadata.
type SessionSecret struct {
	ID          string
	Description string
	AuthorID    string
	AuthorName  *string
	CreatedAt   string
	UpdatedAt   string
}

// NoteRemove removes an authorized note.
type NoteRemove func(id string) (error, bool)

// ProjectNote describes an authorized project or session note.
type ProjectNote struct {
	ID          string
	Title       string
	Description string
	Sensitive   bool
	AuthorID    string
	AuthorName  *string
	CreatedAt   string
	Revision    int
}

// SessionNote describes an authorized session note.
type SessionNote struct {
	ID          string
	Title       string
	Description string
	Sensitive   bool
	AuthorID    string
	AuthorName  *string
	CreatedAt   string
	Revision    int
}

// NoteRevision describes an authorized historical note revision.
type NoteRevision struct {
	Revision    int
	Title       string
	Description string
	Sensitive   bool
	AuthorID    string
	AuthorName  *string
	CreatedAt   string
}

type capabilityDocumentation struct {
	signature   string
	description string
	example     string
	result      string
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
			err, value := fileValue(file, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func fileInfoFunction(files []File, name, kind, label string) func([]lisp.Expr) (error, lisp.Expr) {
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
		if !typed_id.Valid(kind, id) {
			return lisp.Errorf("not a %s file", label), nil
		}
		for _, file := range files {
			if file.ID == id {
				return fileValue(file, name)
			}
		}
		return nil, lisp.Null()
	}
}

func fileValue(file File, name string) (error, lisp.Expr) {
	if file.ID == "" || file.Name == "" || file.Size < 0 || file.Fingerprint == "" {
		return lisp.Errorf("%s has invalid file metadata", name), nil
	}
	mediaType := lisp.Null()
	if file.MediaType != nil {
		mediaType = lisp.String(*file.MediaType)
	}
	return nil, lisp.List(
		lisp.Pair("id", lisp.String(file.ID)),
		lisp.Pair("name", lisp.String(file.Name)),
		lisp.Pair("media_type", mediaType),
		lisp.Pair("size", lisp.Integer(file.Size)),
		lisp.Pair("fingerprint", lisp.String(file.Fingerprint)),
	)
}

func noteListFunction(notes []ProjectNote, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(notes))
		for _, note := range notes {
			err, value := noteValue(note, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func noteValue(note ProjectNote, name string) (error, lisp.Expr) {
	if note.ID == "" || note.Title == "" || note.AuthorID == "" || note.CreatedAt == "" || note.Revision < 1 {
		return lisp.Errorf("%s has invalid note metadata", name), nil
	}
	authorName := lisp.Null()
	if note.AuthorName != nil {
		authorName = lisp.String(*note.AuthorName)
	}
	return nil, lisp.List(
		lisp.Pair("id", lisp.String(note.ID)),
		lisp.Pair("title", lisp.String(note.Title)),
		lisp.Pair("description", lisp.String(note.Description)),
		lisp.Pair("sensitive", lisp.Boolean(note.Sensitive)),
		lisp.Pair("author_id", lisp.String(note.AuthorID)),
		lisp.Pair("author_name", authorName),
		lisp.Pair("created_at", lisp.String(note.CreatedAt)),
		lisp.Pair("revision", lisp.Integer(int64(note.Revision))),
	)
}

func sessionNoteListFunction(notes []SessionNote, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(notes))
		for _, note := range notes {
			err, value := sessionNoteValue(note, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func sessionNoteValue(note SessionNote, name string) (error, lisp.Expr) {
	if note.ID == "" || note.Title == "" || note.AuthorID == "" || note.CreatedAt == "" || note.Revision < 1 {
		return lisp.Errorf("%s has invalid note metadata", name), nil
	}
	authorName := lisp.Null()
	if note.AuthorName != nil {
		authorName = lisp.String(*note.AuthorName)
	}
	return nil, lisp.List(
		lisp.Pair("id", lisp.String(note.ID)),
		lisp.Pair("title", lisp.String(note.Title)),
		lisp.Pair("description", lisp.String(note.Description)),
		lisp.Pair("sensitive", lisp.Boolean(note.Sensitive)),
		lisp.Pair("author_id", lisp.String(note.AuthorID)),
		lisp.Pair("author_name", authorName),
		lisp.Pair("created_at", lisp.String(note.CreatedAt)),
		lisp.Pair("revision", lisp.Integer(int64(note.Revision))),
	)
}

func noteRevisionListFunction(get func(string) (error, []NoteRevision), name string) func([]lisp.Expr) (error, lisp.Expr) {
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
		err, revisions := get(id)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		values := make([]lisp.Expr, 0, len(revisions))
		for _, revision := range revisions {
			err, value := noteRevisionValue(revision, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func noteRevisionValue(revision NoteRevision, name string) (error, lisp.Expr) {
	if revision.Revision < 1 || revision.Title == "" || revision.AuthorID == "" || revision.CreatedAt == "" {
		return lisp.Errorf("%s has invalid note revision metadata", name), nil
	}
	authorName := lisp.Null()
	if revision.AuthorName != nil {
		authorName = lisp.String(*revision.AuthorName)
	}
	return nil, lisp.List(
		lisp.Pair("revision", lisp.Integer(int64(revision.Revision))),
		lisp.Pair("title", lisp.String(revision.Title)),
		lisp.Pair("description", lisp.String(revision.Description)),
		lisp.Pair("sensitive", lisp.Boolean(revision.Sensitive)),
		lisp.Pair("author_id", lisp.String(revision.AuthorID)),
		lisp.Pair("author_name", authorName),
		lisp.Pair("created_at", lisp.String(revision.CreatedAt)),
	)
}

func sessionSecretListFunction(secrets []SessionSecret, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(secrets))
		for _, secret := range secrets {
			err, value := sessionSecretValue(secret, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func sessionSecretValue(secret SessionSecret, name string) (error, lisp.Expr) {
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

func fileReadFunction(read FileRead, name, kind, label string) func([]lisp.Expr) (error, lisp.Expr) {
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
		if !typed_id.Valid(kind, id) {
			return lisp.Errorf("not a %s file", label), nil
		}
		err, value := read(id, offset, length)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return nil, lisp.Bytes(value)
	}
}

func noteReadFunction(read NoteRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
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
		err, value, sensitive := read(id, offset, length)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		result := lisp.Bytes(value)
		if sensitive {
			result = lisp.MarkSensitive(result)
		}
		return nil, result
	}
}

func noteRevisionReadFunction(read NoteRevisionRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 4 {
			return lisp.Errorf("%s requires id, revision, offset, and length", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, revision := lisp.RequireInteger(arguments[1])
		if err != nil {
			return err, nil
		}
		err, offset := lisp.RequireInteger(arguments[2])
		if err != nil {
			return err, nil
		}
		err, length := lisp.RequireInteger(arguments[3])
		if err != nil {
			return err, nil
		}
		if id == "" || revision < 1 || offset < 0 || length < 1 || length > 64*1024 {
			return lisp.Errorf("%s requires a non-empty id, positive revision, non-negative offset, and length from 1 through 65536", name), nil
		}
		err, value, sensitive := read(id, int(revision), offset, length)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		result := lisp.Bytes(value)
		if sensitive {
			result = lisp.MarkSensitive(result)
		}
		return nil, result
	}
}

func unavailableNoteRevisionList(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
}

func unavailableRead(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
}
