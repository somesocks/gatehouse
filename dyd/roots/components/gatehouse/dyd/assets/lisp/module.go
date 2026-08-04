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
			nativeCryptoHMACSHA256ID: nativeCryptoHMACSHA256Module(),
			nativeCryptoSHA256ID:     nativeCryptoSHA256Module(),
			nativeRandomID:           nativeRandomModule(),
			nativeTimeID:             nativeTimeModule(),
		},
	}
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

	err, result := (&evaluator{}).eval(expression, parent)
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

func importBuiltin(loader *moduleLoader) Expr {
	return Expr{kind: exprBuiltin, help: "Loads modules into a lexical body.", builtin: &builtin{
		special: true,
		call: func(evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
			return importModules(loader, evaluator, env, forms, span)
		},
	}}
}

func importRestrict(evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
	if len(forms) != 2 {
		return expressionError(span, "import/restrict requires an allowlist and body expression"), Expr{}
	}
	err, values := expressions(forms[0])
	if err != nil {
		return expressionError(forms[0].span, "import/restrict allowlist must be a proper list"), Expr{}
	}
	allowed := make(map[moduleReference]struct{}, len(values))
	for _, value := range values {
		if value.kind != exprModuleReference {
			return expressionError(value.span, "import/restrict allowlist entries must be module references"), Expr{}
		}
		reference := *value.reference
		if _, exists := allowed[reference]; exists {
			return expressionError(value.span, "import/restrict allowlist contains duplicate module reference %s", reference.String()), Expr{}
		}
		allowed[reference] = struct{}{}
	}

	err, importer := env.lookup("import", span)
	if err != nil || importer.kind != exprBuiltin || !importer.builtin.special {
		return expressionError(span, "import/restrict requires an import binding"), Expr{}
	}
	restricted := &environment{parent: env, values: make(map[string]*Expr, 1)}
	restricted.bind("import", restrictedImportBuiltin(importer, allowed))
	return evaluator.eval(forms[1], restricted)
}

func restrictedImportBuiltin(importer Expr, allowed map[moduleReference]struct{}) Expr {
	return Expr{kind: exprBuiltin, help: importer.help, builtin: &builtin{
		special: true,
		call: func(evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
			if len(forms) < 2 {
				return importer.builtin.call(evaluator, env, forms, span)
			}
			for _, form := range forms[:len(forms)-1] {
				err, _, reference := moduleImportDeclaration(form)
				if err != nil {
					return err, Expr{}
				}
				if _, exists := allowed[reference]; !exists {
					return expressionError(form.span, "import %s is not allowed", reference.String()), Expr{}
				}
			}
			return importer.builtin.call(evaluator, env, forms, span)
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
