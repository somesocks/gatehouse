package lisp

import (
	"fmt"

	"gatehouse/diagnostics"
)

func init() {
	diagnostics.Register("lisp.turn")
	diagnostics.Register("lisp.file_read")
}

// TurnTool is an authorized module made available for one evaluation.
type TurnTool struct {
	ID     string
	Source string
}

// TurnResource is an authorized value made available for one evaluation.
type TurnResource struct {
	ID     string
	Value  []byte
	Secret bool
}

type TurnFile struct {
	ID          string
	Name        string
	MediaType   *string
	Size        int64
	Fingerprint string
}

type TurnFileRead func(id string, offset, length int64) (error, []byte)

// TurnProjectFiles is the optional project-file capability for a turn.
// Supplying it exposes project/files/list and project/files/read.
type TurnProjectFiles struct {
	Files []TurnFile
	Read  TurnFileRead
}

// TurnProjectInfo is the optional linked-project metadata for a turn.
// Supplying it exposes project/info.
type TurnProjectInfo struct {
	Name        *string
	Description *string
	CreatedAt   string
}

type TurnProjectNote struct {
	ID         string
	Title      string
	Description string
	AuthorID   string
	AuthorName *string
	CreatedAt  string
}

type TurnProjectNotes struct {
	Notes []TurnProjectNote
	Read  TurnFileRead
}

var turnFileListDocumentation = doc(
	"(file/list) -> List",
	"Returns successful files in the current session with id, name, optional media_type, size, and fingerprint.",
	"(file/list)",
	"((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))",
)

var turnFileReadDocumentation = doc(
	"(file/read id offset length) -> Bytes",
	"Reads bytes from a successful file in the current session. Length must be from 1 through 65536 bytes.",
	"(bytes/utf8/decode (file/read \"example-file-id\" 0 64))",
	"\"first bytes of the file\"",
)

var turnProjectFileListDocumentation = doc(
	"(project/files/list) -> List",
	"Returns successful files in the project linked to the current session with id, name, optional media_type, size, and fingerprint.",
	"(project/files/list)",
	"((id . \"example-file-id\") (name . \"report.txt\") (media_type . \"text/plain\") (size . 12) (fingerprint . \"sha256:...\"))",
)

var turnProjectInfoDocumentation = doc(
	"(project/info) -> List",
	"Returns the linked project's optional name, optional description, and creation time.",
	"(project/info)",
	"((name . \"Roadmap\") (description) (created_at . \"2026-01-01T00:00:00.000Z\"))",
)

var turnProjectFileReadDocumentation = doc(
	"(project/files/read id offset length) -> Bytes",
	"Reads bytes from a successful file in the project linked to the current session. Length must be from 1 through 65536 bytes.",
	"(bytes/utf8/decode (project/files/read \"example-file-id\" 0 64))",
	"\"first bytes of the file\"",
)

var turnProjectNoteListDocumentation = doc(
	"(project/notes/list) -> List",
	"Returns project notes with id, title, possibly empty description, author_id, optional author_name, and created_at.",
	"(project/notes/list)",
	"((id . \"example-note-id\") (title . \"Guide\") (description . \"How this project works\") (author_id . \"example-principal-id\") (author_name . \"Ada\") (created_at . \"2026-01-01T00:00:00.000Z\"))",
)

var turnProjectNoteReadDocumentation = doc(
	"(project/notes/read id offset length) -> Bytes",
	"Reads Markdown source from a note in the project linked to the current session. Length must be from 1 through 65536 bytes.",
	"(bytes/utf8/decode (project/notes/read \"example-note-id\" 0 64))",
	"\"# Project guide\"",
)

const defaultTurnPrelude = `(import/restrict
  (@native:json/v1
   @native:seq/v1
   @native:crypto/digest/sha256/v1
   @native:crypto/mac/hmac/sha256/v1
   @native:crypto/cipher/aes/128/v1
   @native:crypto/cipher/cbc/v1
   @native:crypto/cipher/ctr/v1
   @native:crypto/padding/pkcs7/v1)
  (import
    (json @native:json/v1)
    (seq @native:seq/v1)
    (crypto/digest/sha256 @native:crypto/digest/sha256/v1)
    (crypto/mac/hmac/sha256 @native:crypto/mac/hmac/sha256/v1)
    (crypto/cipher/aes/128 @native:crypto/cipher/aes/128/v1)
    (crypto/cipher/cbc @native:crypto/cipher/cbc/v1)
    (crypto/cipher/ctr @native:crypto/cipher/ctr/v1)
    (crypto/padding/pkcs7 @native:crypto/padding/pkcs7/v1)
    (let ((import null)
          (import/restrict null)
          (import/search null))
      (eval agent/program))))`

// RunTurn evaluates source using the default turn prelude.
func RunTurn(source string, tools []TurnTool, resources []TurnResource) (error, Expr) {
	return RunTurnWithPrelude(source, defaultTurnPrelude, tools, resources)
}

// RunTurnWithPrelude evaluates source through a prelude with only the supplied tools and resources.
// The parsed source is bound as agent/program, tools as tool-id/export, and resources as resource/id.
func RunTurnWithPrelude(source, prelude string, tools []TurnTool, resources []TurnResource) (error, Expr) {
	return RunTurnWithPreludeAndFilesAndProjectFiles(source, prelude, tools, resources, nil, nil, nil)
}

// RunTurnWithFiles evaluates source with session-file capabilities.
func RunTurnWithFiles(source string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead) (error, Expr) {
	return RunTurnWithFilesAndProjectFiles(source, tools, resources, files, read, nil)
}

// RunTurnWithFilesAndProjectFiles evaluates source with session files and optional linked-project files.
func RunTurnWithFilesAndProjectFiles(source string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead, projectFiles *TurnProjectFiles) (error, Expr) {
	return RunTurnWithFilesAndProjectFilesAndNotes(source, tools, resources, files, read, nil, projectFiles, nil)
}

// RunTurnWithFilesAndProjectFilesAndNotes evaluates source with session files and optional linked-project metadata, files, and notes.
func RunTurnWithFilesAndProjectFilesAndNotes(source string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead, projectInfo *TurnProjectInfo, projectFiles *TurnProjectFiles, projectNotes *TurnProjectNotes) (error, Expr) {
	return RunTurnWithPreludeAndFilesAndProjectFilesAndNotes(source, defaultTurnPrelude, tools, resources, files, read, projectInfo, projectFiles, projectNotes)
}

// RunTurnWithPreludeAndFiles evaluates source through a prelude with session-file capabilities.
func RunTurnWithPreludeAndFiles(source, prelude string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead) (error, Expr) {
	return RunTurnWithPreludeAndFilesAndProjectFilesAndNotes(source, prelude, tools, resources, files, read, nil, nil, nil)
}

// RunTurnWithPreludeAndFilesAndProjectFiles evaluates source through a prelude with session files and optional linked-project files.
func RunTurnWithPreludeAndFilesAndProjectFiles(source, prelude string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead, projectFiles *TurnProjectFiles) (error, Expr) {
	return RunTurnWithPreludeAndFilesAndProjectFilesAndNotes(source, prelude, tools, resources, files, read, nil, projectFiles, nil)
}

// RunTurnWithPreludeAndFilesAndProjectFilesAndNotes evaluates source through a prelude with session files and optional linked-project metadata, files, and notes.
func RunTurnWithPreludeAndFilesAndProjectFilesAndNotes(source, prelude string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead, projectInfo *TurnProjectInfo, projectFiles *TurnProjectFiles, projectNotes *TurnProjectNotes) (error, Expr) {
	call, err := diagnostics.Begin("lisp.turn", "")
	if err != nil {
		return err, nil
	}
	err, result := runTurnWithPreludeAndFilesAndProjectFilesAndNotes(source, prelude, tools, resources, files, read, projectInfo, projectFiles, projectNotes)
	return call.End(err), result
}

func runTurnWithPreludeAndFilesAndProjectFilesAndNotes(source, prelude string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead, projectInfo *TurnProjectInfo, projectFiles *TurnProjectFiles, projectNotes *TurnProjectNotes) (error, Expr) {
	err, program := Read(source)
	if err != nil {
		return err, nil
	}
	err, preludeProgram := Read(prelude)
	if err != nil {
		return fmt.Errorf("read turn prelude: %w", err), nil
	}
	env := bootstrap()
	env.bind("agent/program", program)
	for _, resource := range resources {
		if resource.ID == "" {
			return fmt.Errorf("turn resource ID must not be blank"), nil
		}
		value := Expr(bytesValue(string(resource.Value)))
		if resource.Secret {
			value = withSecret(value)
		}
		env.bind("resource/"+resource.ID, value)
	}
	if read == nil && len(files) > 0 {
		return fmt.Errorf("turn file reader is required"), nil
	}
	if read != nil {
		env.bind("file/list", withHelp(&builtin{call: pure(turnFileList(files, "file/list"))}, turnFileListDocumentation.text()))
		env.bind("file/read", withHelp(&builtin{call: pure(turnFileRead(read, "file/read"))}, turnFileReadDocumentation.text()))
	}
	if projectInfo != nil {
		env.bind("project/info", withHelp(&builtin{call: pure(turnProjectInfo(*projectInfo))}, turnProjectInfoDocumentation.text()))
	}
	if projectFiles != nil {
		if projectFiles.Read == nil {
			return fmt.Errorf("turn project file reader is required"), nil
		}
		env.bind("project/files/list", withHelp(&builtin{call: pure(turnFileList(projectFiles.Files, "project/files/list"))}, turnProjectFileListDocumentation.text()))
		env.bind("project/files/read", withHelp(&builtin{call: pure(turnFileRead(projectFiles.Read, "project/files/read"))}, turnProjectFileReadDocumentation.text()))
	}
	if projectNotes != nil {
		if projectNotes.Read == nil {
			return fmt.Errorf("turn project note reader is required"), nil
		}
		env.bind("project/notes/list", withHelp(&builtin{call: pure(turnProjectNoteList(projectNotes.Notes))}, turnProjectNoteListDocumentation.text()))
		env.bind("project/notes/read", withHelp(&builtin{call: pure(turnFileRead(projectNotes.Read, "project/notes/read"))}, turnProjectNoteReadDocumentation.text()))
	}

	cache := newModuleCache()
	for _, tool := range tools {
		if tool.ID == "" {
			return fmt.Errorf("turn tool ID must not be blank"), nil
		}
		err, module := Read(tool.Source)
		if err != nil {
			return fmt.Errorf("read tool %q: %w", tool.ID, err), nil
		}
		// Tool modules can import native helpers while they are being loaded.
		toolEnv := &environment{parent: env, values: make(map[string]*Expr)}
		bindImports(toolEnv, cache)
		err, result := (&evaluator{}).eval(module, toolEnv)
		if err != nil {
			return fmt.Errorf("load tool %q: %w", tool.ID, err), nil
		}
		err, exports := moduleExports(result)
		if err != nil {
			return fmt.Errorf("load tool %q: %w", tool.ID, err), nil
		}
		for _, export := range exports {
			name := tool.ID + "/" + export.name
			if _, exists := env.values[name]; exists {
				return fmt.Errorf("tool binding %q is duplicated", name), nil
			}
			env.bind(name, export.value)
		}
	}
	bindImports(env, cache)
	return (&evaluator{}).eval(preludeProgram, env)
}

func turnProjectInfo(info TurnProjectInfo) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 0 {
			return expressionError("project/info requires no arguments"), nil
		}
		if info.CreatedAt == "" {
			return expressionError("project/info has invalid project metadata"), nil
		}
		name, description := Expr(null()), Expr(null())
		if info.Name != nil {
			name = stringValue(*info.Name)
		}
		if info.Description != nil {
			description = stringValue(*info.Description)
		}
		return nil, list([]Expr{
			pairValue(symbol("name"), name),
			pairValue(symbol("description"), description),
			pairValue(symbol("created_at"), stringValue(info.CreatedAt)),
		})
	}
}

func turnProjectNoteList(notes []TurnProjectNote) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 0 {
			return expressionError("project/notes/list requires no arguments"), nil
		}
		values := make([]Expr, 0, len(notes))
		for _, note := range notes {
			if note.ID == "" || note.Title == "" || note.AuthorID == "" || note.CreatedAt == "" {
				return expressionError("project/notes/list has invalid note metadata"), nil
			}
			authorName := Expr(null())
			if note.AuthorName != nil {
				authorName = stringValue(*note.AuthorName)
			}
			values = append(values, list([]Expr{
				pairValue(symbol("id"), stringValue(note.ID)),
				pairValue(symbol("title"), stringValue(note.Title)),
				pairValue(symbol("description"), stringValue(note.Description)),
				pairValue(symbol("author_id"), stringValue(note.AuthorID)),
				pairValue(symbol("author_name"), authorName),
				pairValue(symbol("created_at"), stringValue(note.CreatedAt)),
			}))
		}
		return nil, list(values)
	}
}

func turnFileList(files []TurnFile, name string) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 0 {
			return expressionError("%s", name+" requires no arguments"), nil
		}
		values := make([]Expr, 0, len(files))
		for _, file := range files {
			if file.ID == "" || file.Name == "" || file.Size < 0 || file.Fingerprint == "" {
				return expressionError("%s", name+" has invalid file metadata"), nil
			}
			mediaType := Expr(null())
			if file.MediaType != nil {
				mediaType = stringValue(*file.MediaType)
			}
			values = append(values, list([]Expr{
				pairValue(symbol("id"), stringValue(file.ID)),
				pairValue(symbol("name"), stringValue(file.Name)),
				pairValue(symbol("media_type"), mediaType),
				pairValue(symbol("size"), integer(file.Size)),
				pairValue(symbol("fingerprint"), stringValue(file.Fingerprint)),
			}))
		}
		return nil, list(values)
	}
}

func turnFileRead(read TurnFileRead, name string) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 3 {
			return expressionError("%s", name+" requires id, offset, and length"), nil
		}
		err, id := requireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, offset := requireInteger(arguments[1])
		if err != nil {
			return err, nil
		}
		err, length := requireInteger(arguments[2])
		if err != nil {
			return err, nil
		}
		if id == "" || offset < 0 || length < 1 || length > 64*1024 {
			return expressionError("%s", name+" requires a non-empty id, non-negative offset, and length from 1 through 65536"), nil
		}
		call, diagnosticErr := diagnostics.Begin("lisp.file_read", id)
		if diagnosticErr != nil {
			return expressionError("%s", name+" failed"), nil
		}
		err, value := read(id, offset, length)
		err = call.End(err)
		if err != nil {
			return expressionError("%s", name+" failed"), nil
		}
		return nil, bytesValue(string(value))
	}
}
