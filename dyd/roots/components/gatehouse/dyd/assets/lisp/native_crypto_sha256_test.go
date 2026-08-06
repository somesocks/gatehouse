package lisp

import (
	_ "embed"
	"testing"
)

//go:embed native_crypto_sha256_test.lisp
var nativeCryptoSHA256Tests string

func TestNativeCryptoSHA256Lisp(t *testing.T) {
	err, result := Run(nativeCryptoSHA256Tests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Native SHA-256 test result = %s, want null", result)
	}
}
