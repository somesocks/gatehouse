package lisp

import (
	_ "embed"
	"testing"
)

//go:embed boolean_test.lisp
var booleanTests string

func TestBooleanLisp(t *testing.T) {
	err, result := Run(booleanTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Boolean test result = %s, want null", result)
	}
}
