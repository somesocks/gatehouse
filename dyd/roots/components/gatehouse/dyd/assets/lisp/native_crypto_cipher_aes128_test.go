package lisp

import (
	_ "embed"
	"testing"
)

//go:embed native_crypto_cipher_aes128_test.lisp
var nativeCryptoCipherAES128Tests string

func TestNativeCryptoCipherAES128Lisp(t *testing.T) {
	err, result := Run(nativeCryptoCipherAES128Tests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("AES-128 test result = %s, want null", result)
	}
}
