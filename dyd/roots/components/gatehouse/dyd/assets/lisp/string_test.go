package lisp

import (
	_ "embed"
	"testing"
)

//go:embed string_test.lisp
var stringTests string

func TestStringLisp(t *testing.T) {
	err, result := Run(stringTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("String test result = %s, want null", result)
	}
}
