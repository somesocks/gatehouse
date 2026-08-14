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
	err, result := RunTurn(`(begin
  (list
    (null? (let ((value null)) value))
    (help/env "import")
    (help/search "imports limited")))`, nil, nil)
	if err != nil || result.String() != "(#t null null)" {
		t.Fatalf("RunTurn() import visibility = (%s, %v)", result, err)
	}
}

func TestRunTurnExposesSessionFiles(t *testing.T) {
	mediaType := "text/plain"
	err, result := RunTurnWithFiles(`(list
  (file/list)
  (bytes/utf8/decode (file/read "report" 6 5)))`, nil, nil, []TurnFile{{
		ID: "report", Name: "report.txt", MediaType: &mediaType, Size: 11, Fingerprint: "sha256:abc",
	}}, func(id string, offset, length int64) (error, []byte) {
		if id != "report" || offset != 6 || length != 5 {
			t.Fatalf("file reader = (%q, %d, %d)", id, offset, length)
		}
		return nil, []byte("world")
	})
	if err != nil || result.String() != `((((id . "report") (name . "report.txt") (media_type . "text/plain") (size . 11) (fingerprint . "sha256:abc"))) "world")` {
		t.Fatalf("RunTurnWithFiles() = (%s, %v)", result, err)
	}

	err, _ = RunTurnWithFiles(`(file/read "report" 0 65537)`, nil, nil, nil, func(string, int64, int64) (error, []byte) { return nil, nil })
	if err == nil {
		t.Fatal("RunTurnWithFiles() accepted an oversized file read")
	}
}
