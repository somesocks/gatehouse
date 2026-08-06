package lisp

import (
	_ "embed"
	"testing"
)

//go:embed symbol_test.lisp
var symbolTests string

func TestSymbolLisp(t *testing.T) {
	err, result := Run(symbolTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Symbol test result = %s, want null", result)
	}
}
