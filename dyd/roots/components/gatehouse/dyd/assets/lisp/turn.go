package lisp

import "fmt"

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

// RunTurn evaluates source with only the supplied tool modules and resources.
// Tools are available as tool-id/export and resources as resource/id.
func RunTurn(source string, tools []TurnTool, resources []TurnResource) (error, Expr) {
	env := bootstrap()
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
		// A tool may import safe native helpers while it is being loaded.
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
	bindTurnImports(env)

	err, expression := Read(source)
	if err != nil {
		return err, nil
	}
	return (&evaluator{}).eval(expression, env)
}

func bindTurnImports(env *environment) {
	env.bind("import", withHelp(&builtin{
		special: true,
		call: func(_ *evaluator, _ *environment, _ []Expr) (error, Expr) {
			return expressionError("imports are unavailable during turn evaluation"), nil
		},
	}, importDocumentation.text()))
	env.bind("import/search", importSearchBuiltin(map[moduleReference]struct{}{}))
}
