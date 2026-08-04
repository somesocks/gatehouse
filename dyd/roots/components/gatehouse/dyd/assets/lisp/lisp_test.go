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
			if expression.kind != exprModuleReference {
				t.Fatalf("Read(%q) kind = %v, want module reference", test.source, expression.kind)
			}
			if expression.reference.scheme != test.scheme || expression.reference.uri != test.uri || expression.reference.fingerprint != test.fingerprint {
				t.Fatalf("Read(%q) reference = %#v, want scheme %v, uri %q, fingerprint %q", test.source, expression.reference, test.scheme, test.uri, test.fingerprint)
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

func TestReadIgnoresCommentsAndWhitespace(t *testing.T) {
	err, expression := Read(" \t; leading comment\n(list 1 ; middle comment\n 2) ; trailing comment\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := expression.String(), "(list 1 2)"; got != want {
		t.Fatalf("Read() = %s, want %s", got, want)
	}
}

func TestReadDecodesEscapesAndUnicode(t *testing.T) {
	err, stringExpression := Read(`"a\"b\\c\n\r\t"`)
	if err != nil {
		t.Fatal(err)
	}
	if stringExpression.kind != exprString || stringExpression.text != "a\"b\\c\n\r\t" {
		t.Fatalf("Read() = %#v, want decoded string", stringExpression)
	}

	err, symbolExpression := Read("λ")
	if err != nil {
		t.Fatal(err)
	}
	if symbolExpression.kind != exprSymbol || symbolExpression.text != "λ" {
		t.Fatalf("Read() = %#v, want Unicode symbol", symbolExpression)
	}
}

func TestReadTracksByteSpans(t *testing.T) {
	source := "  (list λ)"
	err, expression := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := expression.span, (Span{Start: 2, End: len(source)}); got != want {
		t.Fatalf("expression span = %#v, want %#v", got, want)
	}
	err, forms := expressions(expression)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := forms[0].span, (Span{Start: 3, End: 7}); got != want {
		t.Fatalf("list head span = %#v, want %#v", got, want)
	}
	if got, want := forms[1].span, (Span{Start: 8, End: 10}); got != want {
		t.Fatalf("list argument span = %#v, want %#v", got, want)
	}
}

func TestReadTracksErrorSpans(t *testing.T) {
	for _, test := range []struct {
		source string
		span   Span
	}{
		{source: ")", span: Span{Start: 0, End: 1}},
		{source: "1 2", span: Span{Start: 2, End: 3}},
		{source: `"\q"`, span: Span{Start: 1, End: 3}},
		{source: "'", span: Span{Start: 1, End: 1}},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Read(test.source)
			if err == nil {
				t.Fatal("Read() succeeded")
			}
			languageError, ok := err.(Error)
			if !ok {
				t.Fatalf("Read() error = %T, want lisp.Error", err)
			}
			if languageError.Span != test.span {
				t.Fatalf("Read() error span = %#v, want %#v", languageError.Span, test.span)
			}
		})
	}
}

func TestEvalBuiltinsAndNullTruthiness(t *testing.T) {
	requireRun(t, "(+ 1 2 3)", "6")
	requireRun(t, "(- 10 3 2)", "5")
	requireRun(t, "(* 2 3 4)", "24")
	requireRun(t, "(* 1 -1)", "-1")
	requireRun(t, "(head (tail (list 1 2 3)))", "2")
	requireRun(t, "(if #f 1 2)", "2")
	requireRun(t, "(null? null)", "#t")
	requireRun(t, "(not #f)", "#t")
}

func TestEvalIntegerOperations(t *testing.T) {
	requireRun(t, "(int? 1)", "#t")
	requireRun(t, "(int? \"1\")", "#f")
	requireRun(t, "(int/div 5 2)", "2")
	requireRun(t, "(int/div -5 2)", "-2")
	requireRun(t, "(int/div 5 -2)", "-2")
	requireRun(t, "(int/div -5 -2)", "2")
	requireRun(t, "(int/rem 5 2)", "1")
	requireRun(t, "(int/rem -5 2)", "-1")
	requireRun(t, "(int/rem 5 -2)", "1")
	requireRun(t, "(int/rem -5 -2)", "-1")
	requireRun(t, "(int/rem -9223372036854775808 -1)", "0")
	requireRun(t, "(int/abs -5)", "5")
	requireRun(t, "(int/abs 0)", "0")
	requireRun(t, "(int/min 3 -1 2)", "-1")
	requireRun(t, "(int/max 3 -1 2)", "3")
	requireRun(t, "(int/min 5)", "5")
	requireRun(t, "(int/max -5)", "-5")
	requireRun(t, "(> 3 2)", "#t")
	requireRun(t, "(> 2 3)", "#f")
	requireRun(t, "(>= 3 3)", "#t")
	requireRun(t, "(>= 2 3)", "#f")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(int/div 1 0)", contains: "non-zero divisor"},
		{source: "(int/rem 1 0)", contains: "non-zero divisor"},
		{source: "(int/div -9223372036854775808 -1)", contains: "integer overflow"},
		{source: "(int/div 1)", contains: "requires two integers"},
		{source: "(int/rem 1 \"2\")", contains: "expected an integer"},
		{source: "(int/abs -9223372036854775808)", contains: "integer overflow"},
		{source: "(int/abs)", contains: "requires one integer"},
		{source: "(int/min)", contains: "requires at least one integer"},
		{source: "(int/max 1 \"2\")", contains: "expected an integer"},
		{source: "(> 1)", contains: "requires at least two arguments"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestEvalTypePredicates(t *testing.T) {
	requireRun(t, "(bool? #t)", "#t")
	requireRun(t, "(bool? 1)", "#f")
	requireRun(t, "(symbol? 'name)", "#t")
	requireRun(t, "(symbol? \"name\")", "#f")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(bool?)", contains: "bool? requires one argument"},
		{source: "(symbol? 'one 'two)", contains: "symbol? requires one argument"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestEvalLexicalClosures(t *testing.T) {
	requireRun(t, "(let ((make-adder (fn (x) (fn (y) (+ x y))))) ((make-adder 2) 3))", "5")
	requireRun(t, "(let ((x 1) (y (+ x 1))) y)", "2")
}

func TestEvalRecursiveLetBindings(t *testing.T) {
	requireRun(t, "(let ((even? (fn (n) (if (= n 0) #t (odd? (- n 1))))) (odd? (fn (n) (if (= n 0) #f (even? (- n 1)))))) (even? 100))", "#t")

	err, _ := Run("(let ((first second) (second 2)) first)")
	if err == nil || !strings.Contains(err.Error(), "uninitialized") {
		t.Fatalf("Run() error = %v, want uninitialized binding error", err)
	}
}

func TestEvalBegin(t *testing.T) {
	requireRun(t, "(begin (+ 1 2) (+ 3 4))", "7")
}

func TestEvalBooleanLogic(t *testing.T) {
	requireRun(t, "(and)", "#t")
	requireRun(t, "(or)", "#f")
	requireRun(t, "(and #t #t)", "#t")
	requireRun(t, "(and #t #f)", "#f")
	requireRun(t, "(or #f #t)", "#t")
	requireRun(t, "(or #f #f)", "#f")
	requireRun(t, "(and #f missing)", "#f")
	requireRun(t, "(or #t missing)", "#t")

	for _, source := range []string{
		"(if null 1 2)",
		"(not null)",
		"(and #t null)",
		"(or #f null)",
	} {
		err, _ := Run(source)
		if err == nil || !strings.Contains(err.Error(), "expected a Boolean") {
			t.Fatalf("Run(%q) error = %v, want Boolean error", source, err)
		}
	}
}

func TestEvalSpecialFormControlFlow(t *testing.T) {
	requireRun(t, "(if #t 1 missing)", "1")
	requireRun(t, "(if #f missing 2)", "2")
	requireRun(t, "(let () 1)", "1")
	requireRun(t, "(let ((x 1)) (let ((x 2)) x))", "2")
}

func TestEvalSpecialFormValidation(t *testing.T) {
	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(quote)", contains: "quote requires one expression"},
		{source: "(quote 1 2)", contains: "quote requires one expression"},
		{source: "(let (x) x)", contains: "expected a proper list"},
		{source: "(let ((x 1) (x 2)) x)", contains: "duplicated"},
		{source: "(let () 1 2)", contains: "let requires bindings and one body expression"},
		{source: "(fn (1) 1)", contains: "fn parameters must be symbols"},
		{source: "(fn (x) x x)", contains: "fn requires parameters and one body expression"},
		{source: "(begin)", contains: "begin requires at least one expression"},
		{source: "(begin missing 1)", contains: "unknown binding"},
		{source: "(let ((id (fn (x) x))) (id))", contains: "function requires 1 arguments, got 0"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestEvalErrors(t *testing.T) {
	requireRun(t, "(error/catch 7)", "7")
	requireRun(t, "(error? (error/catch (error/throw (list 'missing 7))))", "#t")
	requireRun(t, "(error/value (error/catch (error/throw (list 'missing 7))))", "(missing 7)")
	requireRun(t, "(bytes? (error/value (error/catch (error/throw (bytes/hex/decode \"ff\")))))", "#t")
	requireRun(t, "(string/contains? (error/value (error/catch (int/div 1 0))) \"non-zero divisor\")", "#t")
	requireRun(t, "(error/value (error/catch (error/throw (error/catch (error/throw 7)))))", "7")
	requireRun(t, "(error? (error/catch (error/throw (secret/mark \"secret\"))))", "#t")
	requireRun(t, "(error/catch (error/throw (secret/mark \"secret\")))", "#<secret>")
	requireRun(t, "(error/value (error/catch (error/throw (secret/mark \"secret\"))))", "#<secret>")
	requireHelp(t, "(help (error/catch (error/throw 7)))", "error")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(error/catch)", contains: "requires one expression"},
		{source: "(error/catch 1 2)", contains: "requires one expression"},
		{source: "(error/throw)", contains: "requires one value"},
		{source: "(error/throw 1 2)", contains: "requires one value"},
		{source: "(error/value 1)", contains: "expected an Error"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}

	err, _ := Run("(error/throw \"failure\")")
	if err == nil || !strings.Contains(err.Error(), "thrown error: \"failure\"") {
		t.Fatalf("Run() error = %v, want thrown error", err)
	}
}

func TestEvalAssert(t *testing.T) {
	requireRun(t, "(assert #t)", "null")
	requireRun(t, "(assert #t 7)", "7")
	requireRun(t, "(assert #t (bytes/hex/decode \"ff\"))", "#<bytes 1>")
	requireRun(t, "(error/value (error/catch (assert #f missing)))", "\"assertion failed\"")
	requireHelp(t, "(help assert)", "Returns null or a supplied value when a condition is true, otherwise raises an Error.")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(assert)", contains: "requires a condition and optional value"},
		{source: "(assert #t 1 2)", contains: "requires a condition and optional value"},
		{source: "(assert 1 7)", contains: "expected a Boolean"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestEvalModuleReferences(t *testing.T) {
	fingerprint := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	requireRun(t, "@native:net/v1", "@native:net/v1")
	requireRun(t, "@file:../shared.lisp", "@file:../shared.lisp")
	requireRun(t, "@#"+fingerprint, "@#"+fingerprint)
	requireRun(t, "(= @#"+fingerprint+" @#"+fingerprint+")", "#t")
	requireRun(t, "(= @native:net/v1 @native:time/v1)", "#f")
	requireHelp(t, "(help @native:net/v1)", "module reference")
}

func TestModuleImports(t *testing.T) {
	core := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	module := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cache := testModuleCache(t, map[string]string{
		core:   "(list (pair 'value 7) (pair 'increment (fn (value) (+ value 1))))",
		module: "(import (core @#" + core + ") (list (pair 'value core/value) (pair 'increment core/increment)))",
	})

	err, exports := runModule("(import (module @#"+module+") (same @#"+module+") (list (pair 'value module/value) (pair 'next (fn (value) (module/increment value)))))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := moduleExportValue(t, exports, "value").String(); got != "7" {
		t.Fatalf("value = %s, want 7", got)
	}
	err, result := (&evaluator{}).call(moduleExportValue(t, exports, "next"), prelude(), []Expr{integer(4, Span{})}, Span{})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "5" {
		t.Fatalf("next(4) = %s, want 5", got)
	}

	err, exports = runModule("(list (pair 'value (fn () (import (module @#"+module+") module/value))))", cache)
	if err != nil {
		t.Fatal(err)
	}
	err, result = (&evaluator{}).call(moduleExportValue(t, exports, "value"), prelude(), nil, Span{})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "7" {
		t.Fatalf("closure import value = %s, want 7", got)
	}

	err, exports = runModule("(let ((load import)) (list (pair 'value (load (module @#"+module+") module/value))))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := moduleExportValue(t, exports, "value").String(); got != "7" {
		t.Fatalf("aliased import value = %s, want 7", got)
	}
}

func TestModuleImportsUseLexicalContext(t *testing.T) {
	id := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	cache := testModuleCache(t, map[string]string{
		id: "(list (pair 'value value))",
	})

	err, exports := runModule("(let ((first (let ((value 1)) (import (module @#"+id+") module/value))) (second (let ((value 2)) (import (module @#"+id+") module/value)))) (list (pair 'first first) (pair 'second second)))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := moduleExportValue(t, exports, "first").String(); got != "1" {
		t.Fatalf("first = %s, want 1", got)
	}
	if got := moduleExportValue(t, exports, "second").String(); got != "2" {
		t.Fatalf("second = %s, want 2", got)
	}
}

func TestModuleCacheAliases(t *testing.T) {
	id := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	cache := testModuleCache(t, map[string]string{
		id: "(list (pair 'value 7))",
	})
	cache.aliases["file:shared.lisp"] = id

	err, exports := runModule("(import (module @file:shared.lisp) (list (pair 'value module/value)))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := moduleExportValue(t, exports, "value").String(); got != "7" {
		t.Fatalf("value = %s, want 7", got)
	}
}

func TestNativeCryptoSHA256Module(t *testing.T) {
	cache := newModuleCache()
	err, exports := runModule("(import (sha256 @native:crypto/sha256/v1) (list (pair 'digest (bytes/hex/encode (sha256/digest (bytes/utf8/encode \"abc\"))))))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if got := moduleExportValue(t, exports, "digest").String(); got != "\"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\"" {
		t.Fatalf("digest = %s, want SHA-256 digest", got)
	}

	err, exports = runModule("(import (sha256 @native:crypto/sha256/v1) (list (pair 'digest (sha256/digest (secret/mark (bytes/utf8/encode \"abc\"))))))", cache)
	if err != nil {
		t.Fatal(err)
	}
	if value := moduleExportValue(t, exports, "digest"); !value.secret || value.String() != "#<secret>" {
		t.Fatalf("secret digest = %#v, want secret Bytes", value)
	}

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(import (sha256 @native:crypto/sha256/v1) (list (pair 'value (sha256/digest))))", contains: "requires one Bytes value"},
		{source: "(import (sha256 @native:crypto/sha256/v1) (list (pair 'value (sha256/digest \"abc\"))))", contains: "expected Bytes"},
		{source: "(import (unknown @native:crypto/unknown/v1) (list))", contains: "unknown module"},
	} {
		t.Run(test.contains, func(t *testing.T) {
			err, _ := runModule(test.source, cache)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("runModule(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestModuleImportErrors(t *testing.T) {
	first := "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	second := "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	safe := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	cache := testModuleCache(t, map[string]string{
		first:  "(import (second @#" + second + ") (list (pair 'value second/value)))",
		second: "(import (first @#" + first + ") (list (pair 'value first/value)))",
		safe:   "(list)",
	})

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(import (missing @#sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa) (list))", contains: "unknown module"},
		{source: "(import (one @#" + safe + ") (one @#" + safe + ") (list))", contains: "alias \"one\" is duplicated"},
		{source: "(import (first @#" + first + ") (list))", contains: "cyclic module import"},
		{source: "(import (bad 1) (list))", contains: "must contain a name and module reference"},
		{source: "(list (pair 'value 1) (pair 'value 2))", contains: "export \"value\" is duplicated"},
	} {
		t.Run(test.contains, func(t *testing.T) {
			err, _ := runModule(test.source, cache)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("runModule(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}

	err, _ := Run("(import (missing @#sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee) null)")
	if err == nil || !strings.Contains(err.Error(), "requires a module loader") {
		t.Fatalf("Run() error = %v, want module loader error", err)
	}
}

func TestEvalSecretTaint(t *testing.T) {
	for _, test := range []struct {
		source string
		secret bool
		value  string
	}{
		{source: "(secret/mark \"secret\")", secret: true, value: "#<secret>"},
		{source: "(string/upper (secret/mark \"secret\"))", secret: true, value: "#<secret>"},
		{source: "(string/length (string/upper (secret/mark \"secret\")))", value: "6"},
		{source: "(string/length (secret/mark \"secret\"))", value: "6"},
		{source: "(list/length (list (secret/mark \"secret\")))", value: "1"},
		{source: "(head (list (secret/mark \"secret\")))", secret: true, value: "#<secret>"},
		{source: "((fn (value) value) (secret/mark \"secret\"))", secret: true, value: "#<secret>"},
		{source: "(let ((value (secret/mark \"secret\"))) ((fn () value)))", secret: true, value: "#<secret>"},
		{source: "(list/map string/upper (list (secret/mark \"secret\")))", secret: true, value: "#<secret>"},
		{source: "(if (string/contains? (secret/mark \"secret\") \"e\") \"yes\" \"no\")", value: "\"yes\""},
		{source: "(secret/mark (list 1 2))", secret: true, value: "#<secret>"},
		{source: "(secret? (secret/mark \"secret\"))", value: "#t"},
		{source: "(secret? (secret/mark +))", value: "#t"},
		{source: "(help (secret/mark string/slice))", value: "\"Returns a Unicode code-point substring slice between an inclusive start and exclusive end.\""},
		{source: "(help/document (secret/mark \"secret\") \"Public help text.\")", secret: true, value: "#<secret>"},
		{source: "(help/search (secret/mark \"secret\"))", value: "(secret/mark secret?)"},
		{source: "(help/env (secret/mark \"secret\"))", value: "(secret/mark secret?)"},
		{source: "(bytes/hex/encode (secret/mark (bytes/hex/decode \"ff\")))", secret: true, value: "#<secret>"},
		{source: "(bytes/base64/encode (secret/mark (bytes/hex/decode \"ff\")))", secret: true, value: "#<secret>"},
		{source: "(bytes/base64url/encode (secret/mark (bytes/hex/decode \"ff\")))", secret: true, value: "#<secret>"},
		{source: "(bytes/base64/pad (secret/mark \"/wA\"))", secret: true, value: "#<secret>"},
		{source: "(bytes/length (secret/mark (bytes/hex/decode \"ff00\")))", value: "2"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, result := Run(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if result.secret != test.secret {
				t.Fatalf("Run(%q) secret = %t, want %t", test.source, result.secret, test.secret)
			}
			if got := result.String(); got != test.value {
				t.Fatalf("Run(%q) = %s, want %s", test.source, got, test.value)
			}
		})
	}
}

func TestEvalPredicatesDoNotPropagateSecretTaint(t *testing.T) {
	for _, source := range []string{
		"(bool? (secret/mark \"secret\"))",
		"(int? (secret/mark \"secret\"))",
		"(symbol? (secret/mark \"secret\"))",
		"(pair? (secret/mark \"secret\"))",
		"(null? (secret/mark \"secret\"))",
		"(list? (secret/mark \"secret\"))",
		"(string? (secret/mark \"secret\"))",
		"(string/contains? (secret/mark \"secret\") \"a\")",
		"(string/prefix? (secret/mark \"secret\") \"a\")",
		"(string/suffix? (secret/mark \"secret\") \"t\")",
		"(bytes? (secret/mark (bytes/hex/decode \"ff\")))",
	} {
		t.Run(source, func(t *testing.T) {
			err, result := Run(source)
			if err != nil {
				t.Fatal(err)
			}
			if result.secret {
				t.Fatalf("Run(%q) result is secret", source)
			}
		})
	}
}

func TestPreludePredicatesAreNonLeaky(t *testing.T) {
	for _, definition := range preludeBuiltins {
		if strings.HasSuffix(definition.name, "?") && definition.leaky {
			t.Errorf("%s is leaky, want non-leaky predicate", definition.name)
		}
	}
}

func TestEvalSecretErrorsAreRedacted(t *testing.T) {
	err, _ := Run("(int/div (secret/mark \"secret\") 1)")
	if err == nil || !strings.Contains(err.Error(), "#<secret>") || strings.Contains(err.Error(), `"secret"`) {
		t.Fatalf("Run() error = %v, want redacted secret", err)
	}
}

func TestEvalHelp(t *testing.T) {
	requireHelp(t, "(help string/slice)", "Returns a Unicode code-point substring slice between an inclusive start and exclusive end.")
	requireHelp(t, "(help (int/min 1 2))", "int")
	requireHelp(t, "(help 'string/slice)", "Returns a Unicode code-point substring slice between an inclusive start and exclusive end.")
	requireHelp(t, "(help 'unbound)", "symbol")
	requireHelp(t, "(help 1)", "int")
	requireHelp(t, "(help (fn (value) value))", "function (value)")
	requireHelp(t, "(let ((string/slice 1)) (help 'string/slice))", "int")
	requireHelp(t, "(let ((local (fn (value) value))) (help 'local))", "function (value)")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(help)", contains: "help requires one argument"},
		{source: "(help 1 2)", contains: "help requires one argument"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestEvalHelpSearch(t *testing.T) {
	requireRun(t, "(help/search \"substring\")", "(string/slice)")
	requireRun(t, "(help/search \"case\")", "(bytes/hex/encode string/lower string/upper)")
	requireRun(t, "(help/search \"accumulator\")", "(list/fold)")
	requireRun(t, "(help/search \"string\" \"slice\")", "(string/slice)")
	requireRun(t, "(help/search \"BOOL?\")", "null")
	requireRun(t, "(let ((alias +)) (help/search \"ALIAS\"))", "null")
	requireRun(t, "(let ((slice string/slice)) (help/search \"SUBSTRING\"))", "(slice string/slice)")
	requireRun(t, "(list/map help (help/search \"substring\"))", "(\"Returns a Unicode code-point substring slice between an inclusive start and exclusive end.\")")
	requireRun(t, "(let ((string/slice 1)) (help/search \"substring\"))", "null")
	requireRun(t, "((fn (string/slice) (help/search \"substring\")) 1)", "null")

	err, _ := Run("(help/search 1)")
	if err == nil || !strings.Contains(err.Error(), "expected a string") {
		t.Fatalf("Run() error = %v, want string error", err)
	}
}

func TestEvalHelpEnv(t *testing.T) {
	requireRun(t, "(help/env \"BOOL?\")", "(bool?)")
	requireRun(t, "(let ((alias +)) (help/env \"ALIAS\"))", "(alias)")
	requireRun(t, "(let ((string/slice 1)) (help/env \"STRING/SLICE\"))", "(string/slice)")
	requireRun(t, "((fn (parameter) (help/env \"PARAMETER\")) 1)", "(parameter)")

	err, all := Run("(help/env)")
	if err != nil {
		t.Fatal(err)
	}
	err, bindings := expressions(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != len(preludeBuiltins)+2 {
		t.Fatalf("(help/env) returned %d bindings, want %d", len(bindings), len(preludeBuiltins)+2)
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding.kind != exprSymbol {
			t.Fatalf("(help/env) returned %s, want symbols", binding)
		}
		seen[binding.text] = struct{}{}
	}
	if _, exists := seen["null"]; !exists {
		t.Error("(help/env) did not return \"null\"")
	}
	if _, exists := seen["import"]; !exists {
		t.Error("(help/env) did not return \"import\"")
	}
	for _, definition := range preludeBuiltins {
		if _, exists := seen[definition.name]; !exists {
			t.Errorf("(help/env) did not return %q", definition.name)
		}
	}
}

func TestEvalHelpDocument(t *testing.T) {
	requireHelp(t, "(let ((pi (help/document 3 \"Circle constant.\"))) (help 'pi))", "Circle constant.")
	requireHelp(t, "(let ((double (help/document (fn (value) (+ value value)) \"Doubles an integer.\"))) (help 'double))", "Doubles an integer.")
	requireRun(t, "(let ((double (help/document (fn (value) (+ value value)) \"Doubles an integer.\"))) (double 3))", "6")
	requireRun(t, "(let ((pi (help/document 3 \"Circle constant.\"))) (help/search \"circle\"))", "(pi)")
	requireRun(t, "(let ((sum (help/document + \"Sums integer values.\"))) (help 'sum))", "\"Sums integer values.\"")
	requireRun(t, "(= + (help/document + \"Sums integer values.\"))", "#t")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(help/document)", contains: "requires a value and help text"},
		{source: "(help/document 1)", contains: "requires a value and help text"},
		{source: "(help/document 1 \"text\" \"extra\")", contains: "requires a value and help text"},
		{source: "(help/document 1 2)", contains: "expected a string"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}

	err, _ := Run("(help/document 1 (secret/mark \"secret\"))")
	if err == nil || !strings.Contains(err.Error(), "requires public help text") {
		t.Fatalf("Run() error = %v, want public help text error", err)
	}
}

func TestEvalHelpSearchReturnsEveryDocumentedPreludeBinding(t *testing.T) {
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
	if len(bindings) != len(preludeBuiltins)+1 {
		t.Fatalf("(help/search) returned %d bindings, want %d", len(bindings), len(preludeBuiltins)+1)
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding.kind != exprSymbol {
			t.Fatalf("(help/search) returned %s, want symbols", binding)
		}
		seen[binding.text] = struct{}{}
	}
	for _, definition := range preludeBuiltins {
		if _, exists := seen[definition.name]; !exists {
			t.Errorf("(help/search) did not return %q", definition.name)
		}
	}
	if _, exists := seen["import"]; !exists {
		t.Error("(help/search) did not return \"import\"")
	}
}

func TestEvalTailRecursion(t *testing.T) {
	requireRun(t, "(let ((count (fn (n total) (if (= n 0) total (count (- n 1) (+ total 1)))))) (count 100000 0))", "100000")
}

func TestEvalRejectsInvalidCalls(t *testing.T) {
	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "missing", contains: "unknown binding"},
		{source: "(1 2)", contains: "not callable"},
		{source: "(fn (x x) x)", contains: "duplicated"},
		{source: "(if #t 1)", contains: "requires a condition"},
		{source: "(head null)", contains: "requires a pair"},
		{source: "(cons 1 2)", contains: "unknown binding"},
		{source: "(first (list 1))", contains: "unknown binding"},
		{source: "(rest (list 1))", contains: "unknown binding"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func requireRun(t *testing.T, source string, want string) {
	t.Helper()
	err, got := Run(source)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != want {
		t.Fatalf("Run(%q) = %s, want %s", source, got, want)
	}
}

func requireHelp(t *testing.T, source string, want string) {
	t.Helper()
	err, got := Run(source)
	if err != nil {
		t.Fatal(err)
	}
	if got.kind != exprString || got.text != want {
		t.Fatalf("Run(%q) = %s, want %q", source, got, want)
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

func moduleExportValue(t *testing.T, exports []moduleExport, name string) Expr {
	t.Helper()
	for _, export := range exports {
		if export.name == name {
			return export.value
		}
	}
	t.Fatalf("module exports do not contain %q", name)
	return Expr{}
}
