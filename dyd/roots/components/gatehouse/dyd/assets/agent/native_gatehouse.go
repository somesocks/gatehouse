package agent

import "gatehouse/lisp"

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

// SessionFileCreate creates a file in the current session and returns its ID.
type SessionFileCreate func(name, mediaType string, contents []byte) (error, string)

// NoteCreate creates an authorized note.
type NoteCreate func(title, description, body string) (error, ProjectNote)

// NoteRemove removes an authorized note.
type NoteRemove func(id string) (error, bool)

// ProjectNote describes an authorized project or session note.
type ProjectNote struct {
	ID         string
	Title      string
	Description string
	AuthorID   string
	AuthorName *string
	CreatedAt  string
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
	if note.ID == "" || note.Title == "" || note.AuthorID == "" || note.CreatedAt == "" {
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
		lisp.Pair("author_id", lisp.String(note.AuthorID)),
		lisp.Pair("author_name", authorName),
		lisp.Pair("created_at", lisp.String(note.CreatedAt)),
	)
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
