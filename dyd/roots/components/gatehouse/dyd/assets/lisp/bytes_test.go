package lisp

import (
	_ "embed"
	"testing"
)

//go:embed bytes_test.lisp
var bytesTests string

func TestBytesLisp(t *testing.T) {
	err, result := Run(bytesTests)
	if err != nil {
		t.Fatal(err)
	}
	if result.kind != exprNull {
		t.Fatalf("Bytes test result = %s, want null", result)
	}

	err, value := Run("(bytes/hex/decode \"ff00\")")
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != "#<bytes 2>" {
		t.Fatalf("Bytes rendering = %s, want #<bytes 2>", got)
	}
}
