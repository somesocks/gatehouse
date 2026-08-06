package lisp

import (
	_ "embed"
	"testing"
)

//go:embed native_random_test.lisp
var nativeRandomTests string

func TestNativeRandomLisp(t *testing.T) {
	err, result := Run(nativeRandomTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Native random test result = %s, want null", result)
	}
}
