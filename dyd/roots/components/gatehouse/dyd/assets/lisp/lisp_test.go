package lisp

import (
	"strings"
	"testing"
)

func TestReadBuildsLispData(t *testing.T) {
	err, expression := Read("'(alpha 2 \"three\")")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := expression.String(), "(quote (alpha 2 \"three\"))"; got != want {
		t.Fatalf("Read() = %s, want %s", got, want)
	}
}

func TestQuoteEvaluatesReaderShorthand(t *testing.T) {
	err, result := Run("'(alpha 2 \"three\")")
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != `(alpha 2 "three")` {
		t.Fatalf("reader shorthand = %s, want (alpha 2 \"three\")", got)
	}
}

func TestReadBuildsDottedPairs(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(1 . 2)", want: "(1 . 2)"},
		{source: "(1 2 . 3)", want: "(1 2 . 3)"},
		{source: "'(1 . 2)", want: "(quote (1 . 2))"},
		{source: "a.b", want: "a.b"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, expression := Read(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if got := expression.String(); got != test.want {
				t.Fatalf("Read(%q) = %s, want %s", test.source, got, test.want)
			}
		})
	}

	err, result := Run("(list (pair? '(1 . 2)) (list? '(1 . 2)) (head '(1 . 2)) (tail '(1 . 2)))")
	if err != nil || result.String() != "(#t #f 1 2)" {
		t.Fatalf("Run() = (%s, %v)", result, err)
	}
}

func TestReadModuleReferences(t *testing.T) {
	fingerprint := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		source      string
		scheme      moduleReferenceScheme
		uri         string
		fingerprint string
	}{
		{source: "@file:../shared.lisp", scheme: moduleReferenceFile, uri: "file:../shared.lisp"},
		{source: "@file:///work/shared.lisp#" + fingerprint, scheme: moduleReferenceFile, uri: "file:///work/shared.lisp", fingerprint: fingerprint},
		{source: "@../shared.lisp#" + fingerprint, scheme: moduleReferenceFingerprint, uri: "../shared.lisp", fingerprint: fingerprint},
		{source: "@#" + fingerprint, scheme: moduleReferenceFingerprint, uri: "", fingerprint: fingerprint},
		{source: "@native:net/v1", scheme: moduleReferenceNative, uri: "native:net/v1"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, expression := Read(test.source)
			if err != nil {
				t.Fatal(err)
			}
			base, _ := unwrap(expression)
			reference, ok := base.(*moduleReference)
			if !ok {
				t.Fatalf("Read(%q) = %T, want module reference", test.source, expression)
			}
			if reference.scheme != test.scheme || reference.uri != test.uri || reference.fingerprint != test.fingerprint {
				t.Fatalf("Read(%q) reference = %#v, want scheme %v, uri %q, fingerprint %q", test.source, reference, test.scheme, test.uri, test.fingerprint)
			}
			if got := expression.String(); got != test.source {
				t.Fatalf("Read(%q).String() = %q, want %q", test.source, got, test.source)
			}
		})
	}
}

func TestReadRejectsInvalidModuleReferences(t *testing.T) {
	fingerprint := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "@secret", contains: "without a URI scheme requires a sha256 fingerprint"},
		{source: "@secret:value", contains: "unsupported module reference URI scheme"},
		{source: "@https://example.com/module.lisp", contains: "unsupported module reference URI scheme"},
		{source: "@file:", contains: "requires a location"},
		{source: "@file://", contains: "requires a location"},
		{source: "@file://#" + fingerprint, contains: "requires a location"},
		{source: "@file:../shared.lisp#sha256:bad", contains: "invalid sha256 fingerprint"},
		{source: "@#sha256:bad", contains: "requires a sha256 fingerprint"},
		{source: "@sha256:bad", contains: "unsupported module reference URI scheme"},
		{source: "@native:", contains: "requires a name"},
		{source: "@native:net/v1#" + fingerprint, contains: "must not have a sha256 fingerprint"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Read(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Read(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestReadRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "", contains: "expected an expression"},
		{source: "(list 1", contains: "unterminated list"},
		{source: "(. value)", contains: "dotted list requires a preceding value"},
		{source: "(value .)", contains: "dotted list requires a tail"},
		{source: "(value . tail extra)", contains: "dotted list tail must be followed by a closing parenthesis"},
		{source: "(value . tail . extra)", contains: "dotted list tail must be followed by a closing parenthesis"},
		{source: "1 2", contains: "expected end of input"},
		{source: "\"unterminated", contains: "unterminated string"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Read(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Read(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestReadAttachesCommentsAndWhitespace(t *testing.T) {
	err, expression := Read(" \t;  leading comment  \n;\tcontinued comment\t\n(list ;  list function  \n 1 ;  one  \n\n ;  two  \n 2) ;  trailing comment  \n; continued trailing comment\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := expression.String(), "(list 1 2)"; got != want {
		t.Fatalf("Read() = %s, want %s", got, want)
	}
	if got, want := helpOf(expression), "leading comment\ncontinued comment\ntrailing comment\ncontinued trailing comment"; got != want {
		t.Fatalf("expression help = %q, want %q", got, want)
	}
	err, forms := expressions(expression)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := helpOf(forms[0]), "list function"; got != want {
		t.Fatalf("list function help = %q, want %q", got, want)
	}
	if got, want := helpOf(forms[1]), "one"; got != want {
		t.Fatalf("first argument help = %q, want %q", got, want)
	}
	if got, want := helpOf(forms[2]), "two"; got != want {
		t.Fatalf("second argument help = %q, want %q", got, want)
	}
}

func TestReadRejectsUnattachedComments(t *testing.T) {
	for _, source := range []string{
		"; missing target\n",
		"(\n  ; missing target\n)",
		"(; missing target\n  1)",
		"(list 1\n  ; missing target\n)",
	} {
		t.Run(source, func(t *testing.T) {
			err, _ := Read(source)
			if err == nil || !strings.Contains(err.Error(), "comment has no target") {
				t.Fatalf("Read(%q) error = %v, want unattached comment error", source, err)
			}
		})
	}
}

func TestReadDecodesEscapesAndUnicode(t *testing.T) {
	err, stringExpression := Read(`"a\"b\\c\n\r\t"`)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := unwrap(stringExpression)
	stringValue, ok := base.(*stringExpr)
	if !ok || stringValue.value != "a\"b\\c\n\r\t" {
		t.Fatalf("Read() = %#v, want decoded string", stringExpression)
	}

	err, symbolExpression := Read("λ")
	if err != nil {
		t.Fatal(err)
	}
	base, _ = unwrap(symbolExpression)
	symbolValue, ok := base.(*symbolExpr)
	if !ok || symbolValue.value != "λ" {
		t.Fatalf("Read() = %#v, want Unicode symbol", symbolExpression)
	}
}

func TestEvalUncaughtErrors(t *testing.T) {
	err, _ := Run("(error/throw \"failure\")")
	if err == nil || !strings.Contains(err.Error(), "thrown error: \"failure\"") {
		t.Fatalf("Run() error = %v, want thrown error", err)
	}
}

func TestEvalCanShadowImport(t *testing.T) {
	err, _ := Run("(let ((import null)) (import null null))")
	if err == nil || !strings.Contains(err.Error(), "not callable") {
		t.Fatalf("shadowed import error = %v, want not-callable error", err)
	}
}

func TestModuleImports(t *testing.T) {
	core := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	module := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cache := testModuleCache(t, map[string]string{
		core:   "(list (pair 'value 7) (pair 'increment (fn (value) (+ value 1))))",
		module: "(import (core @#" + core + ") (list (pair 'value core/value) (pair 'increment core/increment)))",
	})

	err, result := runWithModuleCache("(import (module @#"+module+") (same @#"+module+") (list module/value (fn (value) (module/increment value))))", cache)
	if err != nil {
		t.Fatal(err)
	}
	err, values := expressions(result)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[0].String(); got != "7" {
		t.Fatalf("value = %s, want 7", got)
	}
	err, result = (&evaluator{}).call(values[1], prelude(), []Expr{integer(4)})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "5" {
		t.Fatalf("next(4) = %s, want 5", got)
	}

	err, closure := runWithModuleCache("(fn () (import (module @#"+module+") module/value))", cache)
	if err != nil {
		t.Fatal(err)
	}
	err, result = (&evaluator{}).call(closure, prelude(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "7" {
		t.Fatalf("closure import value = %s, want 7", got)
	}

	err, result = runWithModuleCache("(let ((load import)) (load (module @#"+module+") module/value))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "7" {
		t.Fatalf("aliased import value = %s, want 7", got)
	}
}

func TestModuleCommentsProvideExportHelp(t *testing.T) {
	id := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	cache := testModuleCache(t, map[string]string{
		id: "(list\n  ; Returns the answer.\n  ; The result is always an integer.\n  (pair 'answer 42))",
	})

	err, result := runWithModuleCache("(import (module @#"+id+") module/answer)", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := helpOf(result), "Returns the answer.\nThe result is always an integer."; got != want {
		t.Fatalf("imported answer help = %q, want %q", got, want)
	}
}

func TestModuleImportsUseLexicalContext(t *testing.T) {
	id := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	cache := testModuleCache(t, map[string]string{
		id: "(list (pair 'value value))",
	})

	err, result := runWithModuleCache("(let ((first (let ((value 1)) (import (module @#"+id+") module/value))) (second (let ((value 2)) (import (module @#"+id+") module/value)))) (list first second))", cache)
	if err != nil {
		t.Fatal(err)
	}
	err, values := expressions(result)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[0].String(); got != "1" {
		t.Fatalf("first = %s, want 1", got)
	}
	if got := values[1].String(); got != "2" {
		t.Fatalf("second = %s, want 2", got)
	}
}

func TestModuleCacheAliases(t *testing.T) {
	id := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	cache := testModuleCache(t, map[string]string{
		id: "(list (pair 'value 7))",
	})
	cache.aliases["file:shared.lisp"] = id

	err, result := runWithModuleCache("(import (module @file:shared.lisp) module/value)", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "7" {
		t.Fatalf("value = %s, want 7", got)
	}
}

func TestImportRestrictMatchesReferences(t *testing.T) {
	id := "sha256:abababababababababababababababababababababababababababababababab"
	cache := testModuleCache(t, map[string]string{
		id: "(list (pair 'value 7))",
	})
	cache.aliases["file:allowed.lisp"] = id

	err, result := runWithModuleCache("(import/search \"allowed.lisp\")", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "(@file:allowed.lisp)" {
		t.Fatalf("alias module search = %s, want (@file:allowed.lisp)", got)
	}

	err, result = runWithModuleCache("(import/search \"abab\")", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "(@#"+id+")" {
		t.Fatalf("fingerprint module search = %s, want (@#%s)", got, id)
	}

	err, result = runWithModuleCache("(import/restrict (@file:allowed.lisp) (import (module @file:allowed.lisp) module/value))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "7" {
		t.Fatalf("allowed module value = %s, want 7", got)
	}

	err, result = runWithModuleCache("(import/restrict (@file:allowed.lisp) (import/search))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "(@file:allowed.lisp)" {
		t.Fatalf("allowed module search = %s, want (@file:allowed.lisp)", got)
	}

	err, _ = runWithModuleCache("(import/restrict (@file:allowed.lisp) (import (module @#"+id+") module/value))", cache)
	if err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("fingerprint import error = %v, want restricted import error", err)
	}
}

func TestImportSearchListsResolvableReferences(t *testing.T) {
	id := "sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"
	err, module := Read("(list (pair 'value 7))")
	if err != nil {
		t.Fatal(err)
	}
	cache := &moduleCache{
		aliases: map[string]string{"file:module.lisp": id},
		modules: map[string]Expr{
			id:                  module,
			"native:example/v1": module,
		},
	}

	err, result := runWithModuleCache("(import/search)", cache)
	if err != nil {
		t.Fatal(err)
	}
	want := "(@#" + id + " @file:module.lisp @native:example/v1)"
	if got := result.String(); got != want {
		t.Fatalf("module search = %s, want %s", got, want)
	}

	err, result = runWithModuleCache("(import/search \"example\")", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "(@native:example/v1)" {
		t.Fatalf("filtered module search = %s, want (@native:example/v1)", got)
	}
}

func TestModuleImportErrors(t *testing.T) {
	first := "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	second := "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	safe := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	duplicate := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	cache := testModuleCache(t, map[string]string{
		first:     "(import (second @#" + second + ") (list (pair 'value second/value)))",
		second:    "(import (first @#" + first + ") (list (pair 'value first/value)))",
		safe:      "(list)",
		duplicate: "(list (pair 'value 1) (pair 'value 2))",
	})

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(import (missing @#sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa) (list))", contains: "unknown module"},
		{source: "(import (one @#" + safe + ") (one @#" + safe + ") (list))", contains: "alias \"one\" is duplicated"},
		{source: "(import (first @#" + first + ") (list))", contains: "cyclic module import"},
		{source: "(import (bad 1) (list))", contains: "must contain a name and module reference"},
		{source: "(import (duplicate @#" + duplicate + ") null)", contains: "export \"value\" is duplicated"},
	} {
		t.Run(test.contains, func(t *testing.T) {
			err, _ := runWithModuleCache(test.source, cache)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}

	err, _ := Run("(import (missing @#sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee) null)")
	if err == nil || !strings.Contains(err.Error(), "unknown module") {
		t.Fatalf("Run() error = %v, want unknown module error", err)
	}
}

func TestEvalTaintRendering(t *testing.T) {
	err, result := Run("(taint/secret/mark \"secret\")")
	if err != nil {
		t.Fatal(err)
	}
	if !IsSecret(result) || result.String() != "#<secret>" {
		t.Fatalf("secret value = %#v, want redacted secret", result)
	}
}

func TestTaintBuiltinsAndPropagation(t *testing.T) {
	err, result := Run(`(list
  (taint/get null)
  (taint/get (taint/sensitive/mark 1))
  (taint/get (taint/secret/mark 1))
  (taint/get (bytes/length (taint/secret/mark (bytes/utf8/encode "secret"))))
  (taint/get (string/contains? (taint/secret/mark "secret") "sec"))
  (taint/get (fn/apply if (list (taint/sensitive/mark #t) 1 2)))
  (taint/get (error/value (error/catch (assert (taint/secret/mark #f))))))`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), "(none sensitive secret none secret sensitive secret)"; got != want {
		t.Fatalf("taint propagation = %s, want %s", got, want)
	}
}

func TestPreludeMetadataOperationsAreNonLeaky(t *testing.T) {
	for _, name := range []string{"bool?", "int?", "symbol?", "pair?", "null?", "list?", "list/length", "string?", "string/length", "bytes?", "bytes/length"} {
		for _, definition := range preludeBuiltins {
			if definition.name == name && definition.leaky {
				t.Errorf("%s is leaky, want non-leaky metadata operation", name)
			}
		}
	}
}

func TestEvalSecretErrorsAreRedacted(t *testing.T) {
	err, _ := Run("(int/div (taint/secret/mark \"secret\") 1)")
	if err == nil || !strings.Contains(err.Error(), "#<secret>") || strings.Contains(err.Error(), `"secret"`) {
		t.Fatalf("Run() error = %v, want redacted secret", err)
	}
}

func TestHelpEnvEnumeratesPreludeBindings(t *testing.T) {
	err, all := Run("(help/env)")
	if err != nil {
		t.Fatal(err)
	}
	err, bindings := expressions(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != len(preludeBuiltins)+3 {
		t.Fatalf("(help/env) returned %d bindings, want %d", len(bindings), len(preludeBuiltins)+3)
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		base, _ := unwrap(binding)
		symbol, ok := base.(*symbolExpr)
		if !ok {
			t.Fatalf("(help/env) returned %s, want symbols", binding)
		}
		seen[symbol.value] = struct{}{}
	}
	if _, exists := seen["null"]; !exists {
		t.Error("(help/env) did not return \"null\"")
	}
	if _, exists := seen["import"]; !exists {
		t.Error("(help/env) did not return \"import\"")
	}
	if _, exists := seen["import/search"]; !exists {
		t.Error("(help/env) did not return \"import/search\"")
	}
	for _, definition := range preludeBuiltins {
		if _, exists := seen[definition.name]; !exists {
			t.Errorf("(help/env) did not return %q", definition.name)
		}
	}
}

func TestHelpSearchEnumeratesPreludeBindings(t *testing.T) {
	err, all := Run("(help/search)")
	if err != nil {
		t.Fatal(err)
	}
	err, empty := Run("(help/search \"\")")
	if err != nil {
		t.Fatal(err)
	}
	if !equal(all, empty) {
		t.Fatalf("(help/search \"\") = %s, want %s", empty, all)
	}

	err, bindings := expressions(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != len(preludeBuiltins)+2 {
		t.Fatalf("(help/search) returned %d bindings, want %d", len(bindings), len(preludeBuiltins)+2)
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		base, _ := unwrap(binding)
		symbol, ok := base.(*symbolExpr)
		if !ok {
			t.Fatalf("(help/search) returned %s, want symbols", binding)
		}
		seen[symbol.value] = struct{}{}
	}
	for _, definition := range preludeBuiltins {
		if _, exists := seen[definition.name]; !exists {
			t.Errorf("(help/search) did not return %q", definition.name)
		}
	}
	if _, exists := seen["import"]; !exists {
		t.Error("(help/search) did not return \"import\"")
	}
	if _, exists := seen["import/search"]; !exists {
		t.Error("(help/search) did not return \"import/search\"")
	}
}

func testModuleCache(t *testing.T, sources map[string]string) *moduleCache {
	t.Helper()
	cache := newModuleCache()
	for id, source := range sources {
		err, expression := Read(source)
		if err != nil {
			t.Fatalf("Read(%q) error = %v", id, err)
		}
		cache.modules[id] = expression
	}
	return cache
}

func runWithModuleCache(source string, cache *moduleCache) (error, Expr) {
	err, expression := Read(source)
	if err != nil {
		return err, nil
	}
	env := prelude()
	bindImports(env, cache)
	return (&evaluator{}).eval(expression, env)
}
