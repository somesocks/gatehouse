package lisp

import (
	"fmt"
	"sort"
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

var importDocumentation = doc(
	"(import (alias @reference) ... body) -> Value",
	"Loads module exports as alias/export bindings while evaluating body.",
	"(import (random @native:random/v1) (help/env \"random/\"))",
	"(random/bool random/bytes random/int)",
)

var importSearchDocumentation = doc(
	"(import/search term...) -> List",
	"Returns module references available to the current importer.",
	"(import/search \"time\")",
	"(@native:time/v1)",
)

func newModuleCache() *moduleCache {
	return &moduleCache{
		aliases: make(map[string]string),
		modules: map[string]Expr{
			nativeCryptoCipherAES128ID: nativeCryptoCipherAES128Module(),
			nativeCryptoHMACSHA256ID:   nativeCryptoHMACSHA256Module(),
			nativeCryptoPaddingPKCS7ID: nativeCryptoPaddingPKCS7Module(),
			nativeCryptoSHA256ID:       nativeCryptoSHA256Module(),
			nativeRandomID:             nativeRandomModule(),
			nativeSeqID:                nativeSeqModule(),
			nativeTimeID:               nativeTimeModule(),
		},
	}
}

func (cache *moduleCache) references() map[moduleReference]struct{} {
	references := make(map[moduleReference]struct{})
	if cache == nil {
		return references
	}
	for id := range cache.modules {
		switch {
		case strings.HasPrefix(id, "native:"):
			references[moduleReference{scheme: moduleReferenceNative, uri: id}] = struct{}{}
		case isSHA256Fingerprint(id):
			references[moduleReference{scheme: moduleReferenceFingerprint, fingerprint: id}] = struct{}{}
		}
	}
	for alias, id := range cache.aliases {
		if _, exists := cache.modules[id]; !exists {
			continue
		}
		err, expression := moduleReferenceValue(alias)
		reference, _ := unwrap(expression)
		module, ok := reference.(*moduleReference)
		if err != nil || !ok || module.scheme != moduleReferenceFile || module.fingerprint != "" {
			continue
		}
		references[*module] = struct{}{}
	}
	return references
}

func bindImports(env *environment, cache *moduleCache) {
	env.bind("import", importBuiltin(&moduleLoader{cache: cache}))
	env.bind("import/search", importSearchBuiltin(cache.references()))
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
	return withHelp(&builtin{
		special: true,
		call: func(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
			return importModules(loader, evaluator, env, forms)
		},
	}, importDocumentation.text())
}

func importRestrict(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
	if len(forms) != 2 {
		return expressionError("import/restrict requires an allowlist and body expression"), nil
	}
	err, values := expressions(forms[0])
	if err != nil {
		return expressionError("import/restrict allowlist must be a proper list"), nil
	}
	allowed := make(map[moduleReference]struct{}, len(values))
	for _, value := range values {
		base, _ := unwrap(value)
		reference, ok := base.(*moduleReference)
		if !ok {
			return expressionError("import/restrict allowlist entries must be module references"), nil
		}
		if _, exists := allowed[*reference]; exists {
			return expressionError("import/restrict allowlist contains duplicate module reference %s", reference.String()), nil
		}
		allowed[*reference] = struct{}{}
	}

	err, importer := env.lookup("import")
	base, _ := unwrap(importer)
	importerBuiltin, ok := base.(*builtin)
	if err != nil || !ok || !importerBuiltin.special {
		return expressionError("import/restrict requires an import binding"), nil
	}
	err, allowed = intersectImportAllowlist(evaluator, env, allowed)
	if err != nil {
		return err, nil
	}
	restricted := &environment{parent: env, values: make(map[string]*Expr, 2)}
	restricted.bind("import", restrictedImportBuiltin(importer, allowed))
	restricted.bind("import/search", importSearchBuiltin(allowed))
	return evaluator.eval(forms[1], restricted)
}

func restrictedImportBuiltin(importer Expr, allowed map[moduleReference]struct{}) Expr {
	base, _ := unwrap(importer)
	importerBuiltin := base.(*builtin)
	return withHelp(&builtin{
		special: true,
		call: func(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
			if len(forms) < 2 {
				return importerBuiltin.call(evaluator, env, forms)
			}
			for _, form := range forms[:len(forms)-1] {
				err, _, reference := moduleImportDeclaration(form)
				if err != nil {
					return err, nil
				}
				if _, exists := allowed[reference]; !exists {
					return expressionError("import %s is not allowed", reference.String()), nil
				}
			}
			return importerBuiltin.call(evaluator, env, forms)
		},
	}, helpOf(importer))
}

func intersectImportAllowlist(evaluator *evaluator, env *environment, allowed map[moduleReference]struct{}) (error, map[moduleReference]struct{}) {
	search, exists := env.visibleValue("import/search")
	if !exists {
		return nil, allowed
	}
	err, result := evaluator.call(search, env, nil)
	if err != nil {
		return err, nil
	}
	err, outer := importSearchReferences(result)
	if err != nil {
		return err, nil
	}
	intersection := make(map[moduleReference]struct{}, len(allowed))
	for reference := range allowed {
		if _, exists := outer[reference]; exists {
			intersection[reference] = struct{}{}
		}
	}
	return nil, intersection
}

func importSearchBuiltin(allowed map[moduleReference]struct{}) Expr {
	references := sortedModuleReferences(allowed)
	return withHelp(&builtin{
		call: pure(func(_ *evaluator, arguments []Expr) (error, Expr) {
			err, terms := helpTerms(arguments)
			if err != nil {
				return err, nil
			}
			result := make([]Expr, 0, len(references))
			for _, reference := range references {
				if !matchesTerms(reference.String(), terms) {
					continue
				}
				result = append(result, moduleReferenceExpr(reference))
			}
			return nil, list(result)
		}),
	}, importSearchDocumentation.text())
}

func importSearchReferences(expression Expr) (error, map[moduleReference]struct{}) {
	err, values := expressions(expression)
	if err != nil {
		return expressionError("import/search must return a proper list"), nil
	}
	references := make(map[moduleReference]struct{}, len(values))
	for _, value := range values {
		base, _ := unwrap(value)
		reference, ok := base.(*moduleReference)
		if !ok {
			return expressionError("import/search must return module references"), nil
		}
		references[*reference] = struct{}{}
	}
	return nil, references
}

func sortedModuleReferences(allowed map[moduleReference]struct{}) []moduleReference {
	references := make([]moduleReference, 0, len(allowed))
	for reference := range allowed {
		references = append(references, reference)
	}
	sort.Slice(references, func(left int, right int) bool {
		return references[left].String() < references[right].String()
	})
	return references
}

func moduleReferenceExpr(reference moduleReference) Expr {
	value := reference
	return &value
}

func importModules(loader *moduleLoader, evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
	if loader == nil {
		return expressionError("import requires a module loader"), nil
	}
	if len(forms) < 2 {
		return expressionError("import requires declarations and a body expression"), nil
	}

	imports := &environment{parent: env, values: make(map[string]*Expr)}
	aliases := make(map[string]struct{}, len(forms)-1)
	for _, form := range forms[:len(forms)-1] {
		err, alias, reference := moduleImportDeclaration(form)
		if err != nil {
			return err, nil
		}
		if _, exists := aliases[alias]; exists {
			return expressionError("import alias %q is duplicated", alias), nil
		}
		aliases[alias] = struct{}{}

		err, exports := loader.load(reference, env)
		if err != nil {
			return expressionError("import %s: %v", reference.String(), err), nil
		}
		for _, export := range exports {
			name := alias + "/" + export.name
			if _, exists := imports.values[name]; exists {
				return expressionError("import binding %q is duplicated", name), nil
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
	if len(values) != 2 {
		return expressionError("import declarations must contain a name and module reference"), "", moduleReference{}
	}
	name, _ := unwrap(values[0])
	reference, _ := unwrap(values[1])
	symbol, symbolOK := name.(*symbolExpr)
	module, moduleOK := reference.(*moduleReference)
	if !symbolOK || !moduleOK {
		return expressionError("import declarations must contain a name and module reference"), "", moduleReference{}
	}
	return nil, symbol.value, *module
}

func moduleExports(expression Expr) (error, []moduleExport) {
	err, values := expressions(expression)
	if err != nil {
		return err, nil
	}
	exports := make([]moduleExport, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		base, _ := unwrap(value)
		pair, ok := base.(*pair)
		if !ok {
			return expressionError("module exports must be pairs with symbol names"), nil
		}
		first, _ := unwrap(pair.first)
		name, ok := first.(*symbolExpr)
		if !ok {
			return expressionError("module exports must be pairs with symbol names"), nil
		}
		if _, exists := seen[name.value]; exists {
			return expressionError("module export %q is duplicated", name.value), nil
		}
		seen[name.value] = struct{}{}
		exported := pair.rest
		if help := helpOf(value); help != "" {
			exported = withHelp(exported, help)
		}
		exports = append(exports, moduleExport{name: name.value, value: exported})
	}
	return nil, exports
}
