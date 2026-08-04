package lisp

import (
	"fmt"
	"strings"
)

type moduleResolver interface {
	loadSource(moduleReference) (error, string)
	loadNative(moduleReference) (error, Expr)
}

type moduleLoader struct {
	resolver moduleResolver
	loading  []moduleReference
	loaded   map[moduleReference][]moduleExport
}

type moduleExport struct {
	name  string
	value Expr
}

func runModule(source string, resolver moduleResolver) (error, []moduleExport) {
	loader := &moduleLoader{
		resolver: resolver,
		loaded:   make(map[moduleReference][]moduleExport),
	}
	return loader.evaluateSource(source)
}

func (loader *moduleLoader) evaluateSource(source string) (error, []moduleExport) {
	err, expression := Read(source)
	if err != nil {
		return err, nil
	}
	err, result := (&evaluator{}).eval(expression, modulePrelude(loader))
	if err != nil {
		return err, nil
	}
	return moduleExports(result)
}

func (loader *moduleLoader) load(reference moduleReference) (error, []moduleExport) {
	if exports, exists := loader.loaded[reference]; exists {
		return nil, exports
	}
	for index, active := range loader.loading {
		if active != reference {
			continue
		}
		cycle := append(append([]moduleReference{}, loader.loading[index:]...), reference)
		paths := make([]string, len(cycle))
		for index, item := range cycle {
			paths[index] = item.String()
		}
		return fmt.Errorf("cyclic module import: %s", strings.Join(paths, " -> ")), nil
	}
	if loader.resolver == nil {
		return fmt.Errorf("module resolver is unavailable"), nil
	}

	loader.loading = append(loader.loading, reference)
	defer func() {
		loader.loading = loader.loading[:len(loader.loading)-1]
	}()

	var err error
	var expression Expr
	switch reference.scheme {
	case moduleReferenceNative:
		err, expression = loader.resolver.loadNative(reference)
	default:
		var source string
		err, source = loader.resolver.loadSource(reference)
		if err == nil {
			err, expression = Read(source)
		}
		if err == nil {
			err, expression = (&evaluator{}).eval(expression, modulePrelude(loader))
		}
	}
	if err != nil {
		return fmt.Errorf("load %s: %w", reference.String(), err), nil
	}

	err, exports := moduleExports(expression)
	if err != nil {
		return fmt.Errorf("module %s: %w", reference.String(), err), nil
	}
	loader.loaded[reference] = exports
	return nil, exports
}

func modulePrelude(loader *moduleLoader) *environment {
	env := prelude()
	env.bind("import", importBuiltin(loader))
	return env
}

func importBuiltin(loader *moduleLoader) Expr {
	return Expr{kind: exprBuiltin, help: "Loads modules into a lexical body.", builtin: &builtin{
		special: true,
		call: func(evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
			return importModules(loader, evaluator, env, forms, span)
		},
	}}
}

func importModules(loader *moduleLoader, evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
	if loader == nil {
		return expressionError(span, "import requires a module loader"), Expr{}
	}
	if len(forms) < 2 {
		return expressionError(span, "import requires declarations and a body expression"), Expr{}
	}

	imports := &environment{parent: env, values: make(map[string]*Expr)}
	aliases := make(map[string]struct{}, len(forms)-1)
	for _, form := range forms[:len(forms)-1] {
		err, alias, reference := moduleImportDeclaration(form)
		if err != nil {
			return err, Expr{}
		}
		if _, exists := aliases[alias]; exists {
			return expressionError(form.span, "import alias %q is duplicated", alias), Expr{}
		}
		aliases[alias] = struct{}{}

		err, exports := loader.load(reference)
		if err != nil {
			return expressionError(form.span, "import %s: %v", reference.String(), err), Expr{}
		}
		for _, export := range exports {
			name := alias + "/" + export.name
			if _, exists := imports.values[name]; exists {
				return expressionError(form.span, "import binding %q is duplicated", name), Expr{}
			}
			imports.bind(name, export.value)
		}
	}
	return evaluator.eval(forms[len(forms)-1], imports)
}

func moduleImportDeclaration(expression Expr) (error, string, moduleReference) {
	err, values := expressions(expression)
	if err != nil {
		return err, "", moduleReference{}
	}
	if len(values) != 2 || values[0].kind != exprSymbol || values[1].kind != exprModuleReference {
		return expressionError(expression.span, "import declarations must contain a name and module reference"), "", moduleReference{}
	}
	return nil, values[0].text, *values[1].reference
}

func moduleExports(expression Expr) (error, []moduleExport) {
	err, values := expressions(expression)
	if err != nil {
		return err, nil
	}
	exports := make([]moduleExport, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.kind != exprPair || value.pair.first.kind != exprSymbol {
			return expressionError(value.span, "module exports must be pairs with symbol names"), nil
		}
		name := value.pair.first.text
		if _, exists := seen[name]; exists {
			return expressionError(value.span, "module export %q is duplicated", name), nil
		}
		seen[name] = struct{}{}
		exports = append(exports, moduleExport{name: name, value: value.pair.rest})
	}
	return nil, exports
}
