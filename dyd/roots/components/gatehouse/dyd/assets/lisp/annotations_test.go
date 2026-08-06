package lisp

import "testing"

func TestEvalPreservesSecretAnnotation(t *testing.T) {
	err, result := Eval(withSecret(integer(1)))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSecret(result) {
		t.Fatalf("Eval(withSecret(1)) = %s, want a secret value", result)
	}
}
