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
		}{"sha256/digest", nativeCryptoSHA256Documentation, "(import (sha256 @native:crypto/sha256/v1) (help sha256/digest))"},
		struct {
			name          string
			documentation documentation
			helpSource    string
		}{"hmac/digest", nativeCryptoHMACSHA256Documentation, "(import (hmac @native:crypto/hmac/sha256/v1) (help hmac/digest))"},
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
