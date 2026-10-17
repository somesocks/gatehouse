package lisp

import (
	"errors"
	"strings"
	"testing"
)

func TestImportDefineScopedTaint(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         Taint
	}{
		{"unused definition", `(import/define (@native:fixture/v1 (taint/secret/mark null)) 42)`, TaintSecret},
		{"empty namespace", `(import/define (@native:fixture/v1 (taint/secret/mark null)) (import (fixture @native:fixture/v1) (help/env "fixture/")))`, TaintSecret},
		{"selected namespace", `(import/define (@native:fixture/v1 (if (taint/secret/mark #t) (list (pair 'yes 1)) (list (pair 'no 2)))) (import (fixture @native:fixture/v1) (help/env "fixture/")))`, TaintSecret},
		{"accumulated definitions", `(import/define (@native:one/v1 (taint/sensitive/mark null)) (@native:two/v1 (taint/secret/mark null)) (@native:three/v1 null) 42)`, TaintSecret},
		{"sensitive definition", `(import/define (@native:fixture/v1 (taint/sensitive/mark null)) 42)`, TaintSensitive},
		{"restricted namespace", `(import/define (@native:fixture/v1 (taint/secret/mark null)) (import/restrict () (import/search)))`, TaintSecret},
		{"restricted inherited search", `(let ((import/search (fn () (taint/secret/mark null)))) (import/restrict () 42))`, TaintSecret},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, result := Run(test.source)
			if err != nil || TaintOf(result) != test.want {
				t.Fatalf("Run() = (%v, %v), taint = %v, want %v", result, err, TaintOf(result), test.want)
			}
		})
	}
}

func TestImportDefineTaintedFailures(t *testing.T) {
	for _, source := range []string{
		`(import/define (@native:fixture/v1 (taint/secret/mark (list (pair 'PRIVATE-NAME 1) (pair 'PRIVATE-NAME 2)))) null)`,
		`(import/define (@native:fixture/v1 (taint/secret/mark 7)) null)`,
		`(import/define (@native:fixture/v1 (taint/secret/mark null)) (error/throw "PRIVATE-NAME"))`,
		`(import/define (@native:fixture/v1 (taint/secret/mark null)) (bad) null)`,
		`(import/define (@native:fixture/v1 (taint/secret/mark null)) (@native:fixture/v1 null) null)`,
		`(import/define (@native:fixture/v1 (taint/secret/mark null)) (@native:second/v1 (error/throw "PRIVATE-NAME")) null)`,
	} {
		t.Run(source, func(t *testing.T) {
			err, _ := Run(source)
			if err == nil || !IsSecret(caughtError(err)) || strings.Contains(err.Error(), "PRIVATE-NAME") {
				t.Fatalf("Run() error = %v, want a secret-tainted, redacted error", err)
			}
		})
	}
}

func TestImportDefineCapturedHelpersRetainTaint(t *testing.T) {
	var helpers []Expr
	err, _ := Evaluate(`(import/define (@native:fixture/v1 (taint/secret/mark null)) (capture import import/search))`, EvalOptions{
		Bindings: []Binding{{Name: "capture", Value: Function(func(arguments []Expr) (error, Expr) {
			helpers = append(helpers, arguments...)
			return nil, Null()
		})}},
	})
	if err != nil || len(helpers) != 2 {
		t.Fatalf("capturing helpers: %v, count = %d", err, len(helpers))
	}
	for index, helper := range helpers {
		if !IsSecret(helper) {
			t.Errorf("helper %d lost its definition taint", index)
		}
	}
	_, declaration := Read(`(fixture @native:fixture/v1)`)
	for _, test := range []struct {
		name      string
		index     int
		arguments []Expr
	}{
		{"import", 0, []Expr{declaration, Integer(42)}},
		{"search", 1, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, result := newEvaluator(nil).call(helpers[test.index], prelude(), test.arguments)
			if err != nil || !IsSecret(result) {
				t.Fatalf("escaped helper = (%v, %v), want secret result", result, err)
			}
		})
	}
	// Deriving another importer must preserve a captured parent's taint, too.
	for _, source := range []string{
		`(let ((import saved-import) (import/search saved-search)) (import/define 42))`,
		`(let ((import saved-import) (import/search saved-search)) (import/restrict () 42))`,
	} {
		err, result := Evaluate(source, EvalOptions{Bindings: []Binding{
			{Name: "saved-import", Value: helpers[0]}, {Name: "saved-search", Value: helpers[1]},
		}})
		if err != nil || !IsSecret(result) {
			t.Fatalf("derived importer = (%v, %v), want secret result", result, err)
		}
	}
}

func TestDefinedSearchPreservesInheritedAnnotations(t *testing.T) {
	for _, source := range []string{
		`(let ((import/search (fn () (taint/secret/mark null)))) (import/define (import/search)))`,
		`(let ((import/search (fn () (taint/secret/mark (list @native:fixture/v1))))) (import/define (import/search)))`,
		`(let ((import/search (fn () (taint/secret/mark (list 1))))) (import/define (error/catch (import/search))))`,
	} {
		err, result := Run(source)
		if err != nil || !IsSecret(result) {
			t.Errorf("Run(%s) = (%v, %v), want secret result", source, result, err)
		}
	}
	for _, definitions := range []string{"", `(@native:fixture/v1 null)`} {
		source := `(let ((import/search (fn () (list
  (help/document @native:fixture/v1 "Inherited reference.")
  @native:fixture/v1))))
  (import/define ` + definitions + ` (list (list/length (import/search)) (help (head (import/search))))))`
		err, result := Run(source)
		if err != nil || result.String() != `(1 "Inherited reference.")` {
			t.Errorf("search annotations = (%v, %v), want one documented reference", result, err)
		}
	}
}

func TestInheritedImportFailuresRetainTaint(t *testing.T) {
	for _, source := range []string{
		`(taint/secret/mark (list (pair 'PRIVATE-NAME 1) (pair 'PRIVATE-NAME 2)))`,
		`(taint/secret/mark 7)`,
		`(error/throw (taint/secret/mark "PRIVATE-NAME"))`,
		`(if (taint/secret/mark #t) (error/throw "PRIVATE-NAME") null)`,
	} {
		t.Run(source, func(t *testing.T) {
			cache := testModuleCache(t, map[string]string{"native:fixture/v1": source})
			err, _ := runWithModuleCache(`(import/define (import (fixture @native:fixture/v1) null))`, cache)
			if err == nil || !IsSecret(caughtError(err)) || strings.Contains(err.Error(), "PRIVATE-NAME") {
				t.Fatalf("inherited import error = %v, want secret redacted error", err)
			}
		})
	}
}

func TestInheritedImportJoinedErrorTaint(t *testing.T) {
	cache := testModuleCache(t, map[string]string{"native:fixture/v1": `(fail)`})
	env := bootstrap()
	bindImports(env, cache)
	env.bind("fail", Function(func([]Expr) (error, Expr) {
		return errors.Join(
			taintError(Errorf("sensitive cause"), TaintSensitive),
			taintError(Errorf("PRIVATE-NAME"), TaintSecret),
		), nil
	}))
	_, program := Read(`(import/define (import (fixture @native:fixture/v1) null))`)
	err, _ := newEvaluator(nil).eval(program, env)
	if err == nil || !IsSecret(caughtError(err)) || strings.Contains(err.Error(), "PRIVATE-NAME") {
		t.Fatalf("joined import error = %v, want secret redacted error", err)
	}
}

func TestImportBindingCollisionTaint(t *testing.T) {
	exports := func(name string, taint Taint) string {
		source := `(list (pair '` + name + ` 1))`
		switch taint {
		case TaintSensitive:
			return `(taint/sensitive/mark ` + source + `)`
		case TaintSecret:
			return `(taint/secret/mark ` + source + `)`
		default:
			return source
		}
	}
	for _, test := range []struct {
		name              string
		left, right, want Taint
	}{
		{"public", TaintNone, TaintNone, TaintNone},
		{"sensitive left", TaintSensitive, TaintNone, TaintSensitive},
		{"secret left", TaintSecret, TaintNone, TaintSecret},
		{"secret right", TaintNone, TaintSecret, TaintSecret},
		{"mixed taints", TaintSensitive, TaintSecret, TaintSecret},
	} {
		t.Run(test.name, func(t *testing.T) {
			left := exports("b/PRIVATE-NAME", test.left)
			right := exports("PRIVATE-NAME", test.right)
			cache := testModuleCache(t, map[string]string{
				"native:fixture/left/v1":  left,
				"native:fixture/right/v1": right,
			})
			for _, declarations := range []string{
				`(a @native:fixture/left/v1) (a/b @native:fixture/right/v1)`,
				`(a/b @native:fixture/right/v1) (a @native:fixture/left/v1)`,
			} {
				for name, source := range map[string]string{
					"inherited":     `(import ` + declarations + ` null)`,
					"defined left":  `(import/define (@native:fixture/left/v1 ` + left + `) (import ` + declarations + ` null))`,
					"defined right": `(import/define (@native:fixture/right/v1 ` + right + `) (import ` + declarations + ` null))`,
				} {
					t.Run(name+"/"+declarations, func(t *testing.T) {
						err, _ := runWithModuleCache(source, cache)
						if err == nil {
							t.Fatal("import succeeded despite a binding collision")
						}
						if got := TaintOf(caughtError(err)); got != test.want {
							t.Errorf("collision error taint = %v, want %v: %v", got, test.want, err)
						}
						exposesName := strings.Contains(err.Error(), "a/b/PRIVATE-NAME")
						if exposesName != (test.want == TaintNone) {
							t.Errorf("collision diagnostic = %q, want name visible only for public collisions", err)
						}
					})
				}
			}
		})
	}
}
