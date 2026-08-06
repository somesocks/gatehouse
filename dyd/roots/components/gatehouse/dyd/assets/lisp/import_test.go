package lisp

import (
	_ "embed"
	"testing"
)

//go:embed import_test.lisp
var importTests string

func TestImportLisp(t *testing.T) {
	err, result := Run(importTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Import test result = %s, want null", result)
	}
}
