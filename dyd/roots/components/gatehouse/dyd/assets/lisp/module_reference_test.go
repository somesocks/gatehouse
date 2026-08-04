package lisp

import (
	_ "embed"
	"testing"
)

//go:embed module_reference_test.lisp
var moduleReferenceTests string

func TestModuleReferenceLisp(t *testing.T) {
	err, result := Run(moduleReferenceTests)
	if err != nil {
		t.Fatal(err)
	}
	if result.kind != exprNull {
		t.Fatalf("Module reference test result = %s, want null", result)
	}
}
