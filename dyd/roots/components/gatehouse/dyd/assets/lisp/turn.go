package lisp

import (
	"fmt"
)

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
