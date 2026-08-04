package lisp

import (
	_ "embed"
	"testing"
)

//go:embed native_crypto_hmac_sha256_test.lisp
var nativeCryptoHMACSHA256Tests string

func TestNativeCryptoHMACSHA256Lisp(t *testing.T) {
	err, result := Run(nativeCryptoHMACSHA256Tests)
	if err != nil {
		t.Fatal(err)
	}
	if result.kind != exprNull {
		t.Fatalf("Native HMAC-SHA256 test result = %s, want null", result)
	}
}
