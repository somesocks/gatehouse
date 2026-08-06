package lisp

import (
	_ "embed"
	"testing"
)

//go:embed function_test.lisp
var functionTests string

func TestFunctionLisp(t *testing.T) {
	err, result := Run(functionTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Function test result = %s, want null", result)
	}
}
