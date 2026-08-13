package lisp

import "testing"

func TestRunTurnPreloadsAuthorizedToolsAndResources(t *testing.T) {
	err, result := RunTurn(
		"(bytes/utf8/decode (example/read resource/endpoint))",
		[]TurnTool{{ID: "example", Source: "(list (pair 'read (fn (value) value)))"}},
		[]TurnResource{{ID: "endpoint", Value: []byte("https://example.test")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `"https://example.test"`; got != want {
		t.Fatalf("RunTurn() = %s, want %s", got, want)
	}
}

func TestRunTurnKeepsSecretsTainted(t *testing.T) {
	err, result := RunTurn("resource/token", nil, []TurnResource{{ID: "token", Value: []byte("secret"), Secret: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSecret(result) || result.String() != "#<secret>" {
		t.Fatalf("RunTurn() = %s, want secret result", result)
	}
}

func TestRunTurnDefaultPreludeImportsNativeModules(t *testing.T) {
	err, result := RunTurn(`(begin
  (help json/decode)
	  (help json/string)
	  (help json/object)
  (help seq/bytes/split)
  (help crypto/digest/sha256/digest)
  (help crypto/cipher/aes/128/encrypt)
  (help crypto/cipher/cbc/encrypt)
  (help crypto/cipher/ctr/crypt)
  (help crypto/mac/hmac/sha256/digest)
  (help crypto/padding/pkcs7/pad)
  (json/encode (json/string "ok")))`, nil, nil)
	if err != nil || result.String() != `"\"ok\""` {
		t.Fatalf("RunTurn() default prelude = (%s, %v)", result, err)
	}
}

func TestRunTurnPreludeEvaluatesParsedProgramLexically(t *testing.T) {
	err, result := RunTurnWithPrelude("(+ factor 1)", "(let ((factor 41)) (eval agent/program))", nil, nil)
	if err != nil || result.String() != "42" {
		t.Fatalf("RunTurnWithPrelude() = (%s, %v)", result, err)
	}
}

func TestRunTurnDefaultPreludeHidesImports(t *testing.T) {
	err, result := RunTurn("(list (= import null) (= import/restrict null) (= import/search null))", nil, nil)
	if err != nil || result.String() != "(#t #t #t)" {
		t.Fatalf("RunTurn() imports = (%s, %v)", result, err)
	}
}
