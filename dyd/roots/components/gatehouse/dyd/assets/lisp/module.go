package lisp

import (
	"fmt"
	"strings"
)

type moduleLoader struct {
	cache   *moduleCache
	loading []string
}

type moduleCache struct {
	aliases map[string]string
	modules map[string]Expr
}

type moduleExport struct {
	name  string
	value Expr
}

func newModuleCache() *moduleCache {
	return &moduleCache{
		aliases: make(map[string]string),
		modules: map[string]Expr{
			nativeCryptoSHA256ID: nativeCryptoSHA256Module(),
		},
	}
}

func runModule(source string, cache *moduleCache) (error, []moduleExport) {
	if cache == nil {
		cache = newModuleCache()
	}
	loader := &moduleLoader{
		cache: cache,
	}
	return loader.evaluateSource(source, prelude())
}

func (loader *moduleLoader) evaluateSource(source string, parent *environment) (error, []moduleExport) {
	err, expression := Read(source)
	if err != nil {
		return err, nil
	}
	err, result := (&evaluator{}).eval(expression, moduleEnvironment(loader, parent))
	if err != nil {
		return err, nil
	}
	return moduleExports(result)
}

func (loader *moduleLoader) load(reference moduleReference, parent *environment) (error, []moduleExport) {
	err, id := loader.moduleID(reference)
	if err != nil {
		return err, nil
	}
	for index, active := range loader.loading {
		if active != id {
			continue
		}
		cycle := append(append([]string{}, loader.loading[index:]...), id)
		return fmt.Errorf("cyclic module import: %s", strings.Join(cycle, " -> ")), nil
	}
	if loader.cache == nil {
		return fmt.Errorf("module cache is unavailable"), nil
	}
	expression, exists := loader.cache.modules[id]
	if !exists {
		return fmt.Errorf("unknown module %q", id), nil
	}

	loader.loading = append(loader.loading, id)
	defer func() {
		loader.loading = loader.loading[:len(loader.loading)-1]
	}()

	err, result := (&evaluator{}).eval(expression, moduleEnvironment(loader, parent))
	if err != nil {
		return fmt.Errorf("load %s: %v", reference.String(), err), nil
	}
	err, exports := moduleExports(result)
	if err != nil {
		return fmt.Errorf("module %s: %w", reference.String(), err), nil
	}
	return nil, exports
}

func (loader *moduleLoader) moduleID(reference moduleReference) (error, string) {
	if reference.fingerprint != "" {
		return nil, reference.fingerprint
	}
	if reference.scheme == moduleReferenceNative {
		return nil, reference.uri
	}
	if loader.cache == nil {
		return fmt.Errorf("module cache is unavailable"), ""
	}
	id, exists := loader.cache.aliases[reference.uri]
	if !exists {
		return fmt.Errorf("unknown module alias %q", reference.uri), ""
	}
	return nil, id
}

func moduleEnvironment(loader *moduleLoader, parent *environment) *environment {
	env := &environment{parent: parent, values: make(map[string]*Expr)}
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

		err, exports := loader.load(reference, env)
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
		exported := value.pair.rest
		if value.help != "" {
			exported.help = value.help
		}
		exports = append(exports, moduleExport{name: name, value: exported})
	}
	return nil, exports
}
