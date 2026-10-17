package lisp

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed import_test.lisp
var importTests string

//go:embed import_define_test.lisp
var importDefineTests string

func TestImportLisp(t *testing.T) {
	err, result := Run(importTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Import test result = %s, want null", result)
	}
}

func TestImportDefineLisp(t *testing.T) {
	err, result := Run(importDefineTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Import define test result = %s, want null", result)
	}
}

func TestImportDefineErrors(t *testing.T) {
	for _, test := range []struct {
		name, source, contains string
	}{
		{"missing body", `(import/define)`, "requires a body expression"},
		{"declaration shape", `(import/define 1 null)`, "declarations must contain"},
		{"improper declaration", `(import/define (@native:fixture/v1 . 1) null)`, "declarations must contain"},
		{"missing exports", `(import/define (@native:fixture/v1) null)`, "declarations must contain"},
		{"extra exports", `(import/define (@native:fixture/v1 null null) null)`, "declarations must contain"},
		{"reference type", `(import/define (name null) null)`, "must start with a module reference"},
		{"duplicate reference", `(import/define (@native:fixture/v1 null) (@native:fixture/v1 null) null)`, "duplicate module reference"},
		{"export expression", `(import/define (@native:fixture/v1 (error/throw "fixture failed")) null)`, "fixture failed"},
		{"export list", `(import/define (@native:fixture/v1 1) null)`, "proper list"},
		{"export pair", `(import/define (@native:fixture/v1 (list 1)) null)`, "pairs with symbol names"},
		{"export name", `(import/define (@native:fixture/v1 (list (pair 1 2))) null)`, "pairs with symbol names"},
		{"duplicate export", `(import/define (@native:fixture/v1 (list (pair 'value 1) (pair 'value 2))) null)`, `export "value" is duplicated`},
		{"hidden importer", `(let ((import null)) (import/define null))`, "requires an import binding"},
		{"import arity", `(import/define (@native:fixture/v1 null) (import))`, "requires declarations and a body"},
		{"import alias", `(import/define (@native:fixture/v1 null) (import (value @native:fixture/v1) (value @native:json/v1) null))`, `alias "value" is duplicated`},
		{"binding collision", `(import/define (@native:fixture/one/v1 (list (pair 'b/c 1))) (@native:fixture/two/v1 (list (pair 'c 2))) (import (a @native:fixture/one/v1) (a/b @native:fixture/two/v1) null))`, `binding "a/b/c" is duplicated`},
		{"local then inherited collision", `(import/define (@native:fixture/v1 (list (pair 'b/null 1))) (import (a @native:fixture/v1) (a/b @native:json/v1) null))`, `binding "a/b/null" is duplicated`},
		{"inherited then local collision", `(import/define (@native:fixture/v1 (list (pair 'b/null 1))) (import (a/b @native:json/v1) (a @native:fixture/v1) null))`, `binding "a/b/null" is duplicated`},
		{"search terms", `(import/define (import/search 1))`, "expected a string"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestImportDefineEvaluatesExportsOnceInOrder(t *testing.T) {
	var calls []int64
	err, result := Evaluate(`(import/define
  (@native:fixture/one/v1 (list (pair 'value (initialize 1))))
  (@native:fixture/two/v1 (list (pair 'value (initialize 2))))
  (list
    (import (one @native:fixture/one/v1) (two @native:fixture/two/v1) (+ one/value two/value))
    (import (one @native:fixture/one/v1) one/value)))`, EvalOptions{
		Bindings: []Binding{{Name: "initialize", Value: Function(func(arguments []Expr) (error, Expr) {
			err, value := RequireInteger(arguments[0])
			if err != nil {
				return err, nil
			}
			calls = append(calls, value)
			return nil, Integer(value)
		})}},
	})
	if err != nil || result.String() != "(3 1)" {
		t.Fatalf("Evaluate() = (%v, %v), want (3 1)", result, err)
	}
	if len(calls) != 2 || calls[0] != 1 || calls[1] != 2 {
		t.Fatalf("initialization calls = %v, want [1 2]", calls)
	}
}

func TestImportDefineOverridesHostModuleInPrelude(t *testing.T) {
	calls := 0
	options := EvalOptions{
		Prelude: `(import/define (@native:fixture/v1 (list (pair 'read (fn () "mock")))) (eval program))`,
		HostModules: []HostModule{{ID: "native:fixture/v1", Exports: []HostExport{{Name: "read", Value: Function(func([]Expr) (error, Expr) {
			calls++
			return nil, String("host")
		})}}}},
	}
	source := `(import (fixture @native:fixture/v1) (fixture/read))`
	err, result := Evaluate(source, options)
	if err != nil || result.String() != `"mock"` || calls != 0 {
		t.Fatalf("mock Evaluate() = (%v, %v), host calls = %d", result, err, calls)
	}
	options.Prelude = ""
	err, result = Evaluate(source, options)
	if err != nil || result.String() != `"host"` || calls != 1 {
		t.Fatalf("ordinary Evaluate() = (%v, %v), host calls = %d", result, err, calls)
	}
}

func TestImportDefineExactReferencesAndTransitiveImports(t *testing.T) {
	id := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	cache := testModuleCache(t, map[string]string{
		id:                            "(list (pair 'value 7))",
		"native:fixture/dependent/v1": `(import (dependency @file:fixture.lisp) (list (pair 'value dependency/value)))`,
	})
	cache.aliases["file:fixture.lisp"] = id
	err, result := runWithModuleCache(`(import/define
  (@file:fixture.lisp (list (pair 'value 42)))
  (import
    (mock @file:fixture.lisp)
    (original @#`+id+`)
    (dependent @native:fixture/dependent/v1)
    (list mock/value original/value dependent/value)))`, cache)
	if err != nil || result.String() != "(42 7 42)" {
		t.Fatalf("aliased imports = (%v, %v), want (42 7 42)", result, err)
	}
	err, result = runWithModuleCache(`(import/define
  (@#`+id+` (list (pair 'value 99)))
  (import (mock @#`+id+`) (original @file:fixture.lisp) (list mock/value original/value)))`, cache)
	if err != nil || result.String() != "(99 7)" {
		t.Fatalf("fingerprint imports = (%v, %v), want (99 7)", result, err)
	}
}

func TestImportDefineDelegatesInParentScope(t *testing.T) {
	cache := testModuleCache(t, map[string]string{
		"native:fixture/one/v1": "(list (pair 'value 7))",
		"native:fixture/two/v1": "(list (pair 'value 9))",
	})
	env := bootstrap()
	bindImports(env, cache)
	_, parent := env.lookup("import")
	var aliases []string
	calls := 0
	env.bind("import", &builtin{special: true, call: valueCall(func(evaluation *evaluator, caller *environment, forms []Expr) (error, Expr) {
		calls++
		for _, form := range forms[:len(forms)-1] {
			err, alias, _ := moduleImportDeclaration(form)
			if err != nil {
				return err, nil
			}
			aliases = append(aliases, alias)
		}
		body := &builtin{special: true, call: valueCall(func(evaluation *evaluator, imported *environment, _ []Expr) (error, Expr) {
			scope := &environment{parent: imported, values: make(map[string]*Expr)}
			scope.bind("parent/context", Integer(11))
			return evaluation.eval(forms[len(forms)-1], scope)
		})}
		delegated := append([]Expr(nil), forms...)
		delegated[len(delegated)-1] = list([]Expr{body})
		return evaluation.call(parent, caller, delegated)
	})})
	err, program := Read(`(import/define
  (@native:fixture/local/v1 (list (pair 'value 42)))
  (import
    (first @native:fixture/one/v1)
    (local @native:fixture/local/v1)
    (second @native:fixture/two/v1)
    (list first/value local/value second/value parent/context)))`)
	if err != nil {
		t.Fatal(err)
	}
	err, result := newEvaluator(nil).eval(program, env)
	if err != nil || result.String() != "(7 42 9 11)" {
		t.Fatalf("delegated import = (%v, %v), want (7 42 9 11)", result, err)
	}
	if calls != 1 || strings.Join(aliases, ",") != "first,second" {
		t.Fatalf("parent calls = %d, aliases = %v, want one call with [first second]", calls, aliases)
	}
	// A parent may put its module bindings behind an extra frame. Collisions
	// with those bindings still fail, while caller bindings may be shadowed.
	for _, test := range []struct {
		name, source, wantError string
	}{
		{"collision", `(import/define (@native:fixture/local/v1 (list (pair 'nested/value 42))) (import (parent @native:fixture/local/v1) (parent/nested @native:fixture/one/v1) null))`, `binding "parent/nested/value" is duplicated`},
		{"caller shadowing", `(let ((local/value 99)) (import/define (@native:fixture/local/v1 (list (pair 'value 42))) (import (local @native:fixture/local/v1) (first @native:fixture/one/v1) (assert (= local/value 42)))))`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, program := Read(test.source)
			if err != nil {
				t.Fatal(err)
			}
			err, _ = newEvaluator(nil).eval(program, env)
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("delegated import error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestModuleExportContainerTaints(t *testing.T) {
	for _, test := range []struct {
		name, exports, want string
	}{
		{"secret list", `(taint/secret/mark (list (pair 'value 42)))`, "secret"},
		{"secret pair", `(list (taint/secret/mark (pair 'value 42)))`, "secret"},
		{"sensitive list", `(taint/sensitive/mark (list (pair 'value 42)))`, "sensitive"},
		{"secret branch", `(if (taint/secret/mark #t) (list (pair 'value 42)) (list (pair 'value 7)))`, "secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sources := map[string]string{
				"defined":   `(taint/get (import/define (@native:fixture/v1 ` + test.exports + `) (import (fixture @native:fixture/v1) fixture/value)))`,
				"delegated": `(taint/get (import/define (@native:fixture/v1 ` + test.exports + `) (import/define (import (fixture @native:fixture/v1) fixture/value))))`,
			}
			for name, source := range sources {
				t.Run(name, func(t *testing.T) {
					err, result := Run(source)
					if err != nil || result.String() != test.want {
						t.Fatalf("Run() = (%v, %v), want %s", result, err, test.want)
					}
				})
			}
			t.Run("source module", func(t *testing.T) {
				cache := testModuleCache(t, map[string]string{"native:fixture/v1": test.exports})
				err, result := runWithModuleCache(`(import (fixture @native:fixture/v1) (taint/get fixture/value))`, cache)
				if err != nil || result.String() != test.want {
					t.Fatalf("source module import = (%v, %v), want %s", result, err, test.want)
				}
			})
		})
	}
}
