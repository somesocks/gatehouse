package lisp

import (
	_ "embed"
	"testing"
)

//go:embed secret_test.lisp
var secretTests string

func TestSecretLisp(t *testing.T) {
	err, result := Run(secretTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Secret test result = %s, want null", result)
	}
}
