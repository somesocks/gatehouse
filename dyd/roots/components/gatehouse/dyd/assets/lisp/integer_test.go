package lisp

import (
	_ "embed"
	"testing"
)

//go:embed integer_test.lisp
var integerTests string

func TestIntegerLisp(t *testing.T) {
	err, result := Run(integerTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Integer test result = %s, want null", result)
	}
}
