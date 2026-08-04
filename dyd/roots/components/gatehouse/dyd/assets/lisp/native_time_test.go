package lisp

import (
	_ "embed"
	"testing"
)

//go:embed native_time_test.lisp
var nativeTimeTests string

func TestNativeTimeLisp(t *testing.T) {
	err, result := Run(nativeTimeTests)
	if err != nil {
		t.Fatal(err)
	}
	if result.kind != exprNull {
		t.Fatalf("Native time test result = %s, want null", result)
	}
}
