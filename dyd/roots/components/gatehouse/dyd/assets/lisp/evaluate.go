package lisp

import "fmt"

type EvalOptions struct {
	Prelude       string
	Bindings      []Binding
	SourceModules []SourceModule
	HostModules   []HostModule
}

type Binding struct {
	Name  string
	Value Expr
}

type SourceModule struct {
	ID     string
	Source string
}

type HostModule struct {
	ID      string
	Exports []HostExport
}

type HostExport struct {
	Name  string
	Value Expr
}

// Evaluate evaluates source with isolated bindings and modules.
func Evaluate(source string, options EvalOptions) (error, Expr) {
	err, program := Read(source)
	if err != nil {
		return err, nil
	}

	env := bootstrap()
	env.bind("program", program)
	for _, binding := range options.Bindings {
		if binding.Name == "" || binding.Value == nil {
			return fmt.Errorf("binding is invalid"), nil
		}
		if _, exists := env.values[binding.Name]; exists {
			return fmt.Errorf("binding %q is duplicated", binding.Name), nil
		}
		env.bind(binding.Name, binding.Value)
	}

	cache := newModuleCache()
	for _, module := range options.HostModules {
		if module.ID == "" {
			return fmt.Errorf("host module ID must not be blank"), nil
		}
		if _, exists := cache.modules[module.ID]; exists {
			return fmt.Errorf("host module %q is duplicated", module.ID), nil
		}
		exports := make([]Expr, 0, len(module.Exports))
		seen := make(map[string]struct{}, len(module.Exports))
		for _, export := range module.Exports {
			if export.Name == "" || export.Value == nil {
				return fmt.Errorf("host module %q has an invalid export", module.ID), nil
			}
			if _, exists := seen[export.Name]; exists {
				return fmt.Errorf("host module %q export %q is duplicated", module.ID, export.Name), nil
			}
			seen[export.Name] = struct{}{}
			exports = append(exports, Pair(export.Name, export.Value))
		}
		cache.modules[module.ID] = list([]Expr{symbol("quote"), list(exports)})
	}

	seen := make(map[string]struct{}, len(options.SourceModules))
	for _, module := range options.SourceModules {
		if module.ID == "" {
			return fmt.Errorf("source module ID must not be blank"), nil
		}
		if _, exists := seen[module.ID]; exists {
			return fmt.Errorf("source module %q is duplicated", module.ID), nil
		}
		seen[module.ID] = struct{}{}
		err, expression := Read(module.Source)
		if err != nil {
			return fmt.Errorf("read source module %q: %w", module.ID, err), nil
		}
		moduleEnv := &environment{parent: env, values: make(map[string]*Expr)}
		bindImports(moduleEnv, cache)
		err, result := (&evaluator{}).eval(expression, moduleEnv)
		if err != nil {
			return fmt.Errorf("load source module %q: %w", module.ID, err), nil
		}
		err, exports := moduleExports(result)
		if err != nil {
			return fmt.Errorf("load source module %q: %w", module.ID, err), nil
		}
		for _, export := range exports {
			name := module.ID + "/" + export.name
			if _, exists := env.values[name]; exists {
				return fmt.Errorf("source module binding %q is duplicated", name), nil
			}
			env.bind(name, export.value)
		}
	}

	bindImports(env, cache)
	if options.Prelude == "" {
		return (&evaluator{}).eval(program, env)
	}
	err, prelude := Read(options.Prelude)
	if err != nil {
		return fmt.Errorf("read evaluation prelude: %w", err), nil
	}
	return (&evaluator{}).eval(prelude, env)
}

func String(value string) Expr { return stringValue(value) }
func Bytes(value []byte) Expr  { return bytesValue(string(value)) }
func Boolean(value bool) Expr  { return boolean(value) }
func Integer(value int64) Expr { return integer(value) }
func Null() Expr               { return null() }
func List(values ...Expr) Expr { return list(values) }
func Pair(name string, value Expr) Expr {
	return pairValue(symbol(name), value)
}
func Function(call func([]Expr) (error, Expr)) Expr {
	return &builtin{leaky: true, call: pure(func(_ *evaluator, arguments []Expr) (error, Expr) {
		return call(arguments)
	})}
}
func Document(expression Expr, signature, description, example, result string) Expr {
	return withHelp(expression, doc(signature, description, example, result).text())
}
func RequireString(expression Expr) (error, string) { return requireString(expression) }
func RequireInteger(expression Expr) (error, int64) { return requireInteger(expression) }
func RequireBytes(expression Expr) (error, []byte) {
	err, value := requireBytes(expression)
	return err, []byte(value)
}
func Errorf(format string, args ...any) error       { return expressionError(format, args...) }
func MarkSecret(expression Expr) Expr                { return withSecret(expression) }
