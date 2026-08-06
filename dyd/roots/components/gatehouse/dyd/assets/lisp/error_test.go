package lisp

import (
	_ "embed"
	"testing"
)

//go:embed error_test.lisp
var errorTests string

func TestErrorLisp(t *testing.T) {
	err, result := Run(errorTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Error test result = %s, want null", result)
	}
}
