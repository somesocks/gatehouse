package lisp

import "testing"

func TestDocumentationExamples(t *testing.T) {
	tests := make([]struct {
		name          string
		documentation documentation
		helpSource    string
	}, 0, len(preludeBuiltins)+len(evaluatorForms)+6)
	for _, definition := range preludeBuiltins {
		tests = append(tests, struct {
			name          string
			documentation documentation
			helpSource    string
		}{
			name:          definition.name,
			documentation: definition.documentation,
			helpSource:    "(help '" + definition.name + ")",
		})
	}
	for _, form := range evaluatorForms {
		tests = append(tests, struct {
			name          string
			documentation documentation
			helpSource    string
		}{
			name:          form.name,
			documentation: form.documentation,
			helpSource:    "(help '" + form.name + ")",
		})
	}
	tests = append(tests,
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"aes/encrypt", nativeCryptoCipherAES128EncryptDocumentation, "(import (aes @native:crypto/cipher/aes/128/v1) (help aes/encrypt))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"aes/decrypt", nativeCryptoCipherAES128DecryptDocumentation, "(import (aes @native:crypto/cipher/aes/128/v1) (help aes/decrypt))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"import", importDocumentation, "(help 'import)"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"import/search", importSearchDocumentation, "(help 'import/search)"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"sha256/digest", nativeCryptoSHA256Documentation, "(import (sha256 @native:crypto/digest/sha256/v1) (help sha256/digest))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"hmac/digest", nativeCryptoHMACSHA256Documentation, "(import (hmac @native:crypto/mac/hmac/sha256/v1) (help hmac/digest))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"hmac/verify", nativeCryptoHMACSHA256VerifyDocumentation, "(import (hmac @native:crypto/mac/hmac/sha256/v1) (help hmac/verify))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"http/request", nativeNetHTTPRequestDocumentation, "(import (http @native:net/http/v1) (help http/request))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"http/status", nativeNetHTTPStatusDocumentation, "(import (http @native:net/http/v1) (help http/status))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"http/headers", nativeNetHTTPHeadersDocumentation, "(import (http @native:net/http/v1) (help http/headers))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"http/header", nativeNetHTTPHeaderDocumentation, "(import (http @native:net/http/v1) (help http/header))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"http/body", nativeNetHTTPBodyDocumentation, "(import (http @native:net/http/v1) (help http/body))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"json/decode", nativeJSONDecodeDocumentation, "(import (json @native:json/v1) (help json/decode))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"json/encode", nativeJSONEncodeDocumentation, "(import (json @native:json/v1) (help json/encode))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"random/bytes", nativeRandomBytesDocumentation, "(import (random @native:random/v1) (help random/bytes))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"random/int", nativeRandomIntegerDocumentation, "(import (random @native:random/v1) (help random/int))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"random/bool", nativeRandomBooleanDocumentation, "(import (random @native:random/v1) (help random/bool))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"time/now", nativeTimeDocumentation, "(import (time @native:time/v1) (help time/now))"},
	)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.documentation.signature == "" || test.documentation.description == "" || test.documentation.example == "" || test.documentation.result == "" {
				t.Fatal("documentation requires a signature, description, example, and result")
			}
			err, result := Run(test.documentation.example)
			if err != nil {
				t.Fatalf("example %s: %v", test.documentation.example, err)
			}
			if got := result.String(); got != test.documentation.result {
				t.Fatalf("example %s = %s, want %s", test.documentation.example, got, test.documentation.result)
			}

			err, help := Run(test.helpSource)
			if err != nil {
				t.Fatalf("help %s: %v", test.helpSource, err)
			}
			_, got := requireString(help)
			if got != test.documentation.text() {
				t.Fatalf("help = %q, want %q", got, test.documentation.text())
			}
		})
	}
}
