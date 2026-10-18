package lisp

import (
	"fmt"
	"sort"
	"strings"
)

type moduleLoader struct {
	cache     *moduleCache
	evaluator *evaluator
	loading   []string
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
	"(import (alias @reference) ... body...) -> Value",
	"Leading (alias @reference) forms load exports as alias/export bindings. Evaluates the remaining body expressions in order and returns the last value.",
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
			nativeArchiveZIPID:         nativeArchiveZIPModule(),
			nativeJSONID:               nativeJSONModule(),
			nativeXMLID:                nativeXMLModule(),
			nativeCryptoDigestCRC32ID:  nativeCryptoDigestCRC32Module(),
			nativeNetHTTPID:            nativeNetHTTPModule(),
			nativeCryptoCipherAES128ID: nativeCryptoCipherAES128Module(),
			nativeCryptoCipherCBCID:    nativeCryptoCipherCBCModule(),
			nativeCryptoCipherCTRID:    nativeCryptoCipherCTRModule(),
			nativeCryptoHMACSHA256ID:   nativeCryptoHMACSHA256Module(),
			nativeCryptoPaddingPKCS7ID: nativeCryptoPaddingPKCS7Module(),
			nativeCryptoSHA256ID:       nativeCryptoSHA256Module(),
			nativeCompressDeflateID:    nativeCompressDeflateModule(),
			nativeRandomID:             nativeRandomModule(),
			nativeSeqID:                nativeSeqModule(),
			nativeTimeID:               nativeTimeModule(),
			nativeVectorID:             nativeVectorModule(),
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

	evaluator := loader.evaluator
	if evaluator == nil {
		evaluator = newEvaluator(nil)
	}
	err, result := evaluator.eval(expression, parent)
	if err != nil {
		return fmt.Errorf("load %s: %w", reference.String(), err), nil
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
		call: valueCall(func(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
			return importModules(loader, evaluator, env, forms)
		}),
	}, importDocumentation.text())
}

func importDefine(evaluator *evaluator, env *environment, forms []Expr) (err error, result Expr) {
	taint := TaintNone
	defer func() {
		if err != nil {
			err = taintError(err, taint)
		} else {
			result = withTaint(result, taint)
		}
	}()
	if len(forms) == 0 {
		return expressionError("import/define requires a body expression"), nil
	}
	err, importer := env.lookup("import")
	taint = joinTaint(taint, TaintOf(importer))
	base, _ := unwrap(importer)
	importerBuiltin, ok := base.(*builtin)
	if err != nil || !ok || !importerBuiltin.special {
		return expressionError("import/define requires an import binding"), nil
	}
	err, search := env.lookup("import/search")
	if err != nil {
		return err, nil
	}
	taint = joinTaint(taint, TaintOf(search))

	definitions := make(map[moduleReference][]moduleExport, len(forms)-1)
	for _, form := range forms[:len(forms)-1] {
		taint = joinTaint(taint, TaintOf(form))
		err, declaration := expressions(form)
		if err != nil || len(declaration) != 2 {
			return expressionError("import/define declarations must contain a module reference and export expression"), nil
		}
		base, _ := unwrap(declaration[0])
		reference, ok := base.(*moduleReference)
		if !ok {
			return expressionError("import/define declarations must start with a module reference"), nil
		}
		if _, exists := definitions[*reference]; exists {
			return expressionError("import/define contains duplicate module reference %s", reference.String()), nil
		}
		err, value := evaluator.eval(declaration[1], env)
		if err != nil {
			return err, nil
		}
		taint = joinTaint(taint, TaintOf(value))
		err, exports := moduleExports(value)
		if err != nil {
			return taintError(fmt.Errorf("import/define %s: %w", reference.String(), err), errorTaint(err)), nil
		}
		definitions[*reference] = exports
	}

	defined := &environment{parent: env, values: make(map[string]*Expr, 2)}
	defined.bind("import", withTaint(definedImportBuiltin(importer, definitions), taint))
	defined.bind("import/search", withTaint(definedImportSearchBuiltin(search, definitions), taint))
	return evaluator.eval(forms[len(forms)-1], defined)
}

func definedImportBuiltin(importer Expr, definitions map[moduleReference][]moduleExport) Expr {
	return withHelp(&builtin{
		special: true,
		call: valueCall(func(evaluation *evaluator, env *environment, forms []Expr) (error, Expr) {
			err, forms := normalizeImportForms(forms)
			if err != nil {
				return err, nil
			}
			locals := &environment{values: make(map[string]*Expr)}
			aliases := make(map[string]struct{}, len(forms)-1)
			delegated := make([]Expr, 0, len(forms))
			for _, form := range forms[:len(forms)-1] {
				err, alias, reference := moduleImportDeclaration(form)
				if err != nil {
					return err, nil
				}
				if _, exists := aliases[alias]; exists {
					return expressionError("import alias %q is duplicated", alias), nil
				}
				aliases[alias] = struct{}{}
				if exports, exists := definitions[reference]; exists {
					if err := bindModuleExports(locals, alias, exports); err != nil {
						return err, nil
					}
				} else {
					delegated = append(delegated, form)
				}
			}
			body := forms[len(forms)-1]
			if len(delegated) == 0 {
				locals.parent = env
				return evaluation.eval(body, locals)
			}
			if len(locals.values) == 0 {
				return evaluation.call(importer, env, append(delegated, body))
			}
			continuation := &builtin{special: true, call: valueCall(func(evaluation *evaluator, imported *environment, _ []Expr) (error, Expr) {
				// Check only bindings introduced by the delegated import. Existing
				// caller bindings may be shadowed normally, in any number of frames.
				for name, local := range locals.values {
					for scope := imported; scope != nil && scope != env; scope = scope.parent {
						if existing, exists := scope.values[name]; exists {
							taint := TaintOf(*local)
							if existing != nil {
								taint = joinTaint(taint, TaintOf(*existing))
							}
							return taintError(expressionError("import binding %q is duplicated", name), taint), nil
						}
					}
				}
				return evaluation.eval(body, &environment{parent: imported, values: locals.values})
			})}
			return evaluation.call(importer, env, append(delegated, list([]Expr{continuation})))
		}),
	}, helpOf(importer))
}

func definedImportSearchBuiltin(search Expr, definitions map[moduleReference][]moduleExport) Expr {
	return withHelp(&builtin{
		leaky: true,
		call: valueCall(func(evaluator *evaluator, env *environment, arguments []Expr) (error, Expr) {
			err, terms := helpTerms(arguments)
			if err != nil {
				return err, nil
			}
			err, value := evaluator.call(search, env, arguments)
			if err != nil {
				return err, nil
			}
			err, references := importSearchReferences(value)
			if err != nil {
				return taintError(err, TaintOf(value)), nil
			}
			_, inherited := expressions(value)
			annotated := make(map[moduleReference]Expr, len(inherited))
			for _, expression := range inherited {
				base, _ := unwrap(expression)
				reference := *base.(*moduleReference)
				if first, exists := annotated[reference]; exists {
					// Keep the first reference's documentation and join duplicate taints.
					expression = withTaint(first, TaintOf(expression))
				}
				annotated[reference] = expression
			}
			for reference := range definitions {
				if matchesTerms(reference.String(), terms) {
					references[reference] = struct{}{}
				}
			}
			result := make([]Expr, 0, len(references))
			for _, reference := range sortedModuleReferences(references) {
				expression, exists := annotated[reference]
				if !exists {
					expression = moduleReferenceExpr(reference)
				}
				result = append(result, expression)
			}
			return nil, withTaint(list(result), TaintOf(value))
		}),
	}, importSearchDocumentation.text())
}

func importRestrict(evaluator *evaluator, env *environment, forms []Expr) (err error, result Expr) {
	taint := TaintNone
	defer func() {
		if err != nil {
			err = taintError(err, taint)
		} else {
			result = withTaint(result, taint)
		}
	}()
	if len(forms) != 2 {
		return expressionError("import/restrict requires an allowlist and body expression"), nil
	}
	taint = joinTaint(taint, TaintOf(forms[0]))
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
	taint = joinTaint(taint, TaintOf(importer))
	base, _ := unwrap(importer)
	importerBuiltin, ok := base.(*builtin)
	if err != nil || !ok || !importerBuiltin.special {
		return expressionError("import/restrict requires an import binding"), nil
	}
	err, allowed, searchTaint := intersectImportAllowlist(evaluator, env, allowed)
	taint = joinTaint(taint, searchTaint)
	if err != nil {
		return err, nil
	}
	restricted := &environment{parent: env, values: make(map[string]*Expr, 2)}
	restricted.bind("import", withTaint(restrictedImportBuiltin(importer, allowed), taint))
	restricted.bind("import/search", withTaint(importSearchBuiltin(allowed), taint))
	return evaluator.eval(forms[1], restricted)
}

func restrictedImportBuiltin(importer Expr, allowed map[moduleReference]struct{}) Expr {
	return withHelp(&builtin{
		special: true,
		call: valueCall(func(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
			err, forms := normalizeImportForms(forms)
			if err != nil {
				return err, nil
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
			return evaluator.call(importer, env, forms)
		}),
	}, helpOf(importer))
}

func intersectImportAllowlist(evaluator *evaluator, env *environment, allowed map[moduleReference]struct{}) (error, map[moduleReference]struct{}, Taint) {
	search, exists := env.visibleValue("import/search")
	if !exists {
		return nil, allowed, TaintNone
	}
	err, result := evaluator.call(search, env, nil)
	if err != nil {
		return err, nil, TaintOf(search)
	}
	err, outer := importSearchReferences(result)
	if err != nil {
		return err, nil, TaintOf(result)
	}
	intersection := make(map[moduleReference]struct{}, len(allowed))
	for reference := range allowed {
		if _, exists := outer[reference]; exists {
			intersection[reference] = struct{}{}
		}
	}
	return nil, intersection, TaintOf(result)
}

func importSearchBuiltin(allowed map[moduleReference]struct{}) Expr {
	references := sortedModuleReferences(allowed)
	return withHelp(&builtin{
		leaky: true,
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
	err, forms := normalizeImportForms(forms)
	if err != nil {
		return err, nil
	}
	loader.evaluator = evaluator

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
			return taintError(fmt.Errorf("import %s: %w", reference.String(), err), errorTaint(err)), nil
		}
		if err := bindModuleExports(imports, alias, exports); err != nil {
			return err, nil
		}
	}
	return evaluator.eval(forms[len(forms)-1], imports)
}

func bindModuleExports(env *environment, alias string, exports []moduleExport) error {
	for _, export := range exports {
		name := alias + "/" + export.name
		if existing, exists := env.values[name]; exists {
			taint := TaintOf(export.value)
			if existing != nil {
				taint = joinTaint(taint, TaintOf(*existing))
			}
			return taintError(expressionError("import binding %q is duplicated", name), taint)
		}
		env.bind(name, export.value)
	}
	return nil
}

// Keep delegation single-body: scoped importers can forward one expression to
// their parent while all body forms still run in the resulting imported scope.
func normalizeImportForms(forms []Expr) (error, []Expr) {
	if len(forms) < 2 {
		return expressionError("import requires declarations and a body expression"), nil
	}
	for index, form := range forms {
		err, _, _ := moduleImportDeclaration(form)
		if err == nil {
			continue
		}
		if index == 0 {
			return err, nil
		}
		body := forms[index:]
		if len(body) == 1 {
			return nil, forms
		}
		// Call begin directly so a shadowed binding cannot alter sequencing, and
		// discarded body values retain the same taint semantics as begin.
		sequence := &builtin{special: true, call: func(evaluator *evaluator, env *environment, _ []Expr) (error, callOutcome) {
			return beginValues(evaluator, env, body)
		}}
		normalized := append([]Expr(nil), forms[:index]...)
		return nil, append(normalized, list([]Expr{sequence}))
	}
	return expressionError("import requires declarations and a body expression"), nil
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

func moduleExports(expression Expr) (err error, exports []moduleExport) {
	defer func() {
		if err != nil {
			err = taintError(err, TaintOf(expression))
		}
	}()
	err, values := expressions(expression)
	if err != nil {
		return err, nil
	}
	exports = make([]moduleExport, 0, len(values))
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
		exported := withTaint(pair.rest, joinTaint(TaintOf(expression), TaintOf(value)))
		if help := helpOf(value); help != "" {
			exported = withHelp(exported, help)
		}
		exports = append(exports, moduleExport{name: name.value, value: exported})
	}
	return nil, exports
}
