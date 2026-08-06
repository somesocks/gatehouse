package lisp

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed native_crypto_padding_pkcs7_test.lisp
var nativeCryptoPaddingPKCS7Tests string

func TestNativeCryptoPaddingPKCS7Lisp(t *testing.T) {
	err, result := Run(nativeCryptoPaddingPKCS7Tests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("PKCS#7 padding test result = %s, want null", result)
	}
}

func TestNativeCryptoPaddingPKCS7DocumentationExamples(t *testing.T) {
	cache := newModuleCache()
	err, exports := (&moduleLoader{cache: cache}).load(moduleReference{scheme: moduleReferenceNative, uri: nativeCryptoPaddingPKCS7ID}, prelude())
	if err != nil {
		t.Fatal(err)
	}
	for _, export := range exports {
		t.Run(export.name, func(t *testing.T) {
			lines := strings.Split(helpOf(export.value), "\n")
			if len(lines) != 3 || !strings.HasPrefix(lines[2], "Example: ") {
				t.Fatalf("help = %q, want standard documentation", helpOf(export.value))
			}
			example, expected, found := strings.Cut(strings.TrimPrefix(lines[2], "Example: "), " => ")
			if !found || !strings.HasSuffix(expected, ".") {
				t.Fatalf("example = %q, want source and result", lines[2])
			}
			err, result := Run(example)
			if err != nil {
				t.Fatalf("example %s: %v", example, err)
			}
			if got, want := result.String(), strings.TrimSuffix(expected, "."); got != want {
				t.Fatalf("example %s = %s, want %s", example, got, want)
			}
		})
	}
}
