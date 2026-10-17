package lisp

import "testing"

func TestEvalPreservesTaintAnnotation(t *testing.T) {
	err, result := Eval(MarkSecret(integer(1)))
	if err != nil {
		t.Fatal(err)
	}
	if !IsSecret(result) {
		t.Fatalf("Eval(MarkSecret(1)) = %s, want a secret value", result)
	}
}

func TestTaintDoesNotDowngrade(t *testing.T) {
	if !IsSecret(MarkSensitive(MarkSecret(integer(1)))) {
		t.Fatal("MarkSensitive(MarkSecret(1)) downgraded the taint")
	}
}

func TestCallableTaint(t *testing.T) {
	for _, test := range []struct {
		name, callee, arguments string
		failure                 bool
	}{
		{"closure", `(fn () 7)`, ``, false},
		{"closure tail call", `(fn () ((fn () 7)))`, ``, false},
		{"builtin", `list/length`, `(list 1 2)`, false},
		{"special builtin tail", `if`, `#t 7 9`, false},
		{"closure arity", `(fn (value) value)`, ``, true},
		{"closure earlier error", `(fn () (error/throw "private") 7)`, ``, true},
		{"closure tail error", `(fn () (error/throw "private"))`, ``, true},
		{"builtin error", `int/div`, `1 0`, true},
		{"special builtin error", `if`, ``, true},
		{"noncallable", `7`, ``, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, result := Run(`((taint/secret/mark ` + test.callee + `) ` + test.arguments + `)`)
			if test.failure {
				if err == nil || !IsSecret(caughtError(err)) {
					t.Errorf("direct call error = %v, want secret error", err)
				}
			} else if err != nil || !IsSecret(result) {
				t.Errorf("direct call = (%v, %v), want secret result", result, err)
			}
			_, callee := Run(`(taint/secret/mark ` + test.callee + `)`)
			_, arguments := Read(`(` + test.arguments + `)`)
			_, forms := expressions(arguments)
			evaluation, env := newEvaluator(nil), prelude()
			base, _ := unwrap(callee)
			builtin, special := base.(*builtin)
			if !special || !builtin.special {
				for index, form := range forms {
					var argument Expr
					err, argument = evaluation.eval(form, env)
					if err != nil {
						t.Fatal(err)
					}
					forms[index] = argument
				}
			}
			err, result = evaluation.call(callee, env, forms)
			if test.failure {
				if err == nil || !IsSecret(caughtError(err)) {
					t.Errorf("indirect call error = %v, want secret error", err)
				}
			} else if err != nil || !IsSecret(result) {
				t.Errorf("indirect call = (%v, %v), want secret result", result, err)
			}
		})
	}
}

func TestQuotedHelpPreservesResolvedTaint(t *testing.T) {
	for _, source := range []string{
		`(let ((value (taint/secret/mark (help/document 7 "Private selection.")))) (help 'value))`,
		`(let ((first 'second) (second (taint/secret/mark (help/document 7 "Private selection.")))) (help 'first))`,
		`(let ((first (taint/secret/mark 'second)) (second (help/document 7 "Private selection."))) (help 'first))`,
		`(let ((first 'second) (second (taint/secret/mark 'first))) (help 'first))`,
	} {
		err, result := Run(source)
		if err != nil || !IsSecret(result) {
			t.Errorf("Run(%s) = (%v, %v), want secret help", source, result, err)
		}
	}
}
