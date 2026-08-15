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
	return RunTurnWithPreludeAndFiles(source, prelude, tools, resources, nil, nil)
}

func RunTurnWithFiles(source string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead) (error, Expr) {
	return RunTurnWithPreludeAndFiles(source, defaultTurnPrelude, tools, resources, files, read)
}

func RunTurnWithPreludeAndFiles(source, prelude string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead) (error, Expr) {
	call, err := diagnostics.Begin("lisp.turn", "")
	if err != nil {
		return err, nil
	}
	err, result := runTurnWithPreludeAndFiles(source, prelude, tools, resources, files, read)
	return call.End(err), result
}

func runTurnWithPreludeAndFiles(source, prelude string, tools []TurnTool, resources []TurnResource, files []TurnFile, read TurnFileRead) (error, Expr) {
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
		env.bind("file/list", withHelp(&builtin{call: pure(turnFileList(files))}, turnFileListDocumentation.text()))
		env.bind("file/read", withHelp(&builtin{call: pure(turnFileRead(read))}, turnFileReadDocumentation.text()))
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

func turnFileList(files []TurnFile) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 0 {
			return expressionError("file/list requires no arguments"), nil
		}
		values := make([]Expr, 0, len(files))
		for _, file := range files {
			if file.ID == "" || file.Name == "" || file.Size < 0 || file.Fingerprint == "" {
				return expressionError("file/list has invalid file metadata"), nil
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

func turnFileRead(read TurnFileRead) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 3 {
			return expressionError("file/read requires id, offset, and length"), nil
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
			return expressionError("file/read requires a non-empty id, non-negative offset, and length from 1 through 65536"), nil
		}
		call, diagnosticErr := diagnostics.Begin("lisp.file_read", id)
		if diagnosticErr != nil {
			return expressionError("file/read failed"), nil
		}
		err, value := read(id, offset, length)
		err = call.End(err)
		if err != nil {
			return expressionError("file/read failed"), nil
		}
		return nil, bytesValue(string(value))
	}
}
