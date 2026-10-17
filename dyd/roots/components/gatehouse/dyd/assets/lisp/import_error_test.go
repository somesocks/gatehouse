package lisp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestImportedExceptionPayload(t *testing.T) {
	for _, test := range []struct {
		name, source string
		taint        Taint
	}{
		{"public payload", `(error/throw (list 'failure 7))`, TaintNone},
		{"secret payload", `(error/throw (taint/secret/mark (list 'failure 7)))`, TaintSecret},
		{"secret control flow", `(if (taint/secret/mark #t) (error/throw (list 'failure 7)) null)`, TaintSecret},
	} {
		t.Run(test.name, func(t *testing.T) {
			cache := testModuleCache(t, map[string]string{
				"native:fixture/inner/v1": test.source,
				"native:fixture/outer/v1": `(import (inner @native:fixture/inner/v1) null)`,
			})
			for _, source := range []string{
				`(import (fixture @native:fixture/inner/v1) null)`,
				`(import (fixture @native:fixture/outer/v1) null)`,
				`(import/define (import (fixture @native:fixture/outer/v1) null))`,
				`(import/restrict (@native:fixture/inner/v1 @native:fixture/outer/v1) (import (fixture @native:fixture/outer/v1) null))`,
			} {
				t.Run(source, func(t *testing.T) {
					err, result := runWithModuleCache(`(error/value (error/catch `+source+`))`, cache)
					want := List(Symbol("failure"), Integer(7))
					if err != nil || !equal(result, want) || TaintOf(result) != test.taint {
						t.Fatalf("caught payload = (%v, %v), taint = %v, want (failure 7) with taint %v", result, err, TaintOf(result), test.taint)
					}
				})
			}
		})
	}
}

func TestImportedExceptionContextAndCause(t *testing.T) {
	cache := testModuleCache(t, map[string]string{
		"native:fixture/inner/v1": `(error/throw (list 'failure 7))`,
		"native:fixture/outer/v1": `(import (inner @native:fixture/inner/v1) null)`,
	})
	err, _ := runWithModuleCache(`(import (outer @native:fixture/outer/v1) null)`, cache)
	var raised *raisedError
	if !errors.As(err, &raised) {
		t.Fatalf("import error = %v, want original raised exception in error chain", err)
	}
	for _, context := range []string{"native:fixture/inner/v1", "native:fixture/outer/v1"} {
		if !strings.Contains(err.Error(), context) {
			t.Errorf("import error %q lost context %q", err, context)
		}
	}
}

func TestImportDefinePreservesValidationCause(t *testing.T) {
	err, _ := Run(`(import/define (@native:fixture/v1 7) null)`)
	var validation Error
	if !errors.As(err, &validation) || validation.Message != "expected a proper list" {
		t.Fatalf("definition error = %v, want original export validation error in chain", err)
	}
	if !strings.Contains(err.Error(), "import/define @native:fixture/v1") {
		t.Fatalf("definition error %q lost module context", err)
	}
}

func TestImportCancellationPreservesIdentity(t *testing.T) {
	for _, source := range []string{
		`(import (fixture @native:fixture/v1) null)`,
		`(import/define (import (fixture @native:fixture/v1) null))`,
		`(import/restrict (@native:fixture/v1) (import (fixture @native:fixture/v1) null))`,
		`(error/catch (import/define (import (fixture @native:fixture/v1) null)))`,
	} {
		t.Run(source, func(t *testing.T) {
			cache := testModuleCache(t, map[string]string{"native:fixture/v1": `(interrupt)`})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			env := bootstrap()
			bindImports(env, cache)
			called := false
			env.bind("interrupt", Function(func([]Expr) (error, Expr) {
				called = true
				cancel()
				return nil, Null()
			}))
			err, program := Read(source)
			if err != nil {
				t.Fatal(err)
			}
			err, _ = newEvaluator(ctx).eval(program, env)
			if !called || !errors.Is(err, ErrInterrupted) || !errors.Is(err, context.Canceled) {
				t.Fatalf("import error = %v, initializer called = %t; want ErrInterrupted and context.Canceled", err, called)
			}
		})
	}
}
