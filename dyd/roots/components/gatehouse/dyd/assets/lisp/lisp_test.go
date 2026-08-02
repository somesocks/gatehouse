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

func TestEvalQuoteAndPairOperations(t *testing.T) {
	requireRun(t, "'(one 2)", "(one 2)")
	requireRun(t, "(pair 'one (pair 2 null))", "(one 2)")
	requireRun(t, "(pair 1 2)", "(1 . 2)")
	requireRun(t, "(head (pair 1 2))", "1")
	requireRun(t, "(tail (pair 1 2))", "2")
	requireRun(t, "(tail (list 1 2 3))", "(2 3)")
	requireRun(t, "(pair? (pair 1 2))", "#t")
	requireRun(t, "(pair? (list 1 2))", "#t")
	requireRun(t, "(pair? null)", "#f")
	requireRun(t, "(and (pair? null) (head null))", "#f")
}

func TestEvalListOperations(t *testing.T) {
	requireRun(t, "(list? null)", "#t")
	requireRun(t, "(list? (list 1 2))", "#t")
	requireRun(t, "(list? (pair 1 2))", "#f")
	requireRun(t, "(list/length null)", "0")
	requireRun(t, "(list/length (list 1 2 3))", "3")
	requireRun(t, "(list/concat)", "null")
	requireRun(t, "(list/concat (list 1 2) null (list 3))", "(1 2 3)")
	requireRun(t, "(list/fold + 0 (list 1 2 3))", "6")
	requireRun(t, "(list/fold (fn (values value) (pair value values)) null (list 1 2 3))", "(3 2 1)")

	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(list/length (pair 1 2))", contains: "expected a proper list"},
		{source: "(list/concat (pair 1 2))", contains: "expected a proper list"},
		{source: "(list/fold + 0 (pair 1 2))", contains: "expected a proper list"},
		{source: "(list/fold + 0)", contains: "requires a function, initial value, and list"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
	}
}

func TestEvalStructuralEquality(t *testing.T) {
	requireRun(t, "(= 1 1)", "#t")
	requireRun(t, "(= 1 \"1\")", "#f")
	requireRun(t, "(= 'name 'name)", "#t")
	requireRun(t, "(= 'name \"name\")", "#f")
	requireRun(t, "(= null null)", "#t")
	requireRun(t, "(= '(1 (2 3)) (list 1 (list 2 3)))", "#t")
	requireRun(t, "(= '(1 2) '(1 3))", "#f")
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

func TestEvalMapAndFilter(t *testing.T) {
	requireRun(t, "(list/map (fn (x) (* x x)) '(1 2 3))", "(1 4 9)")
	requireRun(t, "(list/filter (fn (x) (< x 3)) '(1 2 3))", "(1 2)")
	requireRun(t, "(list/filter (fn (x) #f) '(1 2 3))", "null")

	err, _ := Run("(list/filter (fn (x) x) '(1))")
	if err == nil || !strings.Contains(err.Error(), "expected a Boolean") {
		t.Fatalf("Run() error = %v, want Boolean error", err)
	}
	for _, source := range []string{
		"(map (fn (x) x) null)",
		"(filter (fn (x) #t) null)",
	} {
		err, _ := Run(source)
		if err == nil || !strings.Contains(err.Error(), "unknown binding") {
			t.Fatalf("Run(%q) error = %v, want unknown binding error", source, err)
		}
	}
}

func TestEvalStrings(t *testing.T) {
	requireRun(t, "(string? \"text\")", "#t")
	requireRun(t, "(string? 1)", "#f")
	requireRun(t, "(string/length \"aλb\")", "3")
	requireRun(t, "(string/concat)", "\"\"")
	requireRun(t, "(string/concat \"gate\" \"house\")", "\"gatehouse\"")
	requireRun(t, "(string/slice \"aλbc\" 1 3)", "\"λb\"")
	requireRun(t, "(string/trim \" text \")", "\"text\"")
	requireRun(t, "(string/lower \"Straße\")", "\"straße\"")
	requireRun(t, "(string/upper \"Straße\")", "\"STRAßE\"")
	requireRun(t, "(string/contains? \"gatehouse\" \"house\")", "#t")
	requireRun(t, "(string/contains? \"gatehouse\" \"\")", "#t")
	requireRun(t, "(string/prefix? \"gatehouse\" \"gate\")", "#t")
	requireRun(t, "(string/suffix? \"gatehouse\" \"house\")", "#t")
	requireRun(t, "(string/split \"a,,b,\" \",\")", "(\"a\" \"\" \"b\" \"\")")
	requireRun(t, "(string/split \"aλb\" \"\")", "(\"a\" \"λ\" \"b\")")
	requireRun(t, "(string/split \"\" \"\")", "null")
	requireRun(t, "(string/join (list \"one\" \"two\" \"three\") \", \")", "\"one, two, three\"")
	requireRun(t, "(string/join null \",\")", "\"\"")
	requireRun(t, "(string/replace \"a-b-c\" \"-\" \"\")", "\"abc\"")
	requireRun(t, "(string/replace \"one two one\" \"one\" \"1\")", "\"1 two 1\"")
}

func TestEvalHelp(t *testing.T) {
	requireHelp(t, "(help string/slice)", "Returns the code-point slice between an inclusive start and exclusive end.")
	requireHelp(t, "(help (int/min 1 2))", "int")
	requireHelp(t, "(help 'string/slice)", "Returns the code-point slice between an inclusive start and exclusive end.")
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
	requireRun(t, "(help/search \"case\")", "(string/lower string/upper)")
	requireRun(t, "(help/search \"accumulator\")", "(list/fold)")
	requireRun(t, "(help/search \"string\" \"slice\")", "(string/slice)")
	requireRun(t, "(list/length (help/search))", "44")
	requireRun(t, "(list/length (help/search \"\"))", "44")
	requireRun(t, "(list/map help (help/search \"substring\"))", "(\"Returns the code-point slice between an inclusive start and exclusive end.\")")
	requireRun(t, "(let ((string/slice 1)) (help/search \"substring\"))", "null")
	requireRun(t, "((fn (string/slice) (help/search \"substring\")) 1)", "null")

	err, _ := Run("(help/search 1)")
	if err == nil || !strings.Contains(err.Error(), "expected a string") {
		t.Fatalf("Run() error = %v, want string error", err)
	}
}

func TestEvalStringErrors(t *testing.T) {
	for _, test := range []struct {
		source   string
		contains string
	}{
		{source: "(string/length 1)", contains: "expected a string"},
		{source: "(string/slice \"abc\" -1 2)", contains: "indices are out of range"},
		{source: "(string/slice \"abc\" 2 1)", contains: "indices are out of range"},
		{source: "(string/join (list \"a\" 1) \",\")", contains: "expected a string"},
		{source: "(string/replace \"abc\" \"\" \"x\")", contains: "non-empty old text"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(%q) error = %v, want %q", test.source, err, test.contains)
			}
		})
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
