package lisp

import (
	_ "embed"
	"testing"
)

//go:embed collections_test.lisp
var collectionsTests string

func TestCollectionsLisp(t *testing.T) {
	err, result := Run(collectionsTests)
	if err != nil {
		t.Fatal(err)
	}
	if result.kind != exprNull {
		t.Fatalf("Collections test result = %s, want null", result)
	}

	err, value := Run("(pair 1 2)")
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != "(1 . 2)" {
		t.Fatalf("Pair rendering = %s, want (1 . 2)", got)
	}
}
