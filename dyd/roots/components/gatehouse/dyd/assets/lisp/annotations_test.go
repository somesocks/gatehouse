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
