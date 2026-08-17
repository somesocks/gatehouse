package lisp

import (
	"strings"
	"testing"
)

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

func TestRunTurnExposesLinkedProjectFilesSeparately(t *testing.T) {
	attachmentMediaType := "text/plain"
	mediaType := "text/markdown"
	err, result := RunTurnWithFilesAndProjectFiles(`(list
  (file/list)
  (project/files/list)
  (bytes/utf8/decode (project/files/read "guide" 0 5)))`, nil, nil, []TurnFile{{
		ID: "attachment", Name: "attachment.txt", MediaType: &attachmentMediaType, Size: 1, Fingerprint: "sha256:attachment",
	}}, func(string, int64, int64) (error, []byte) {
		return nil, []byte("a")
	}, &TurnProjectFiles{Files: []TurnFile{{
		ID: "guide", Name: "guide.md", MediaType: &mediaType, Size: 5, Fingerprint: "sha256:guide",
	}}, Read: func(id string, offset, length int64) (error, []byte) {
		if id != "guide" || offset != 0 || length != 5 {
			t.Fatalf("project file reader = (%q, %d, %d)", id, offset, length)
		}
		return nil, []byte("hello")
	}})
	if err != nil || result.String() != `((((id . "attachment") (name . "attachment.txt") (media_type . "text/plain") (size . 1) (fingerprint . "sha256:attachment"))) (((id . "guide") (name . "guide.md") (media_type . "text/markdown") (size . 5) (fingerprint . "sha256:guide"))) "hello")` {
		t.Fatalf("RunTurnWithFilesAndProjectFiles() = (%s, %v)", result, err)
	}

	err, _ = RunTurnWithFiles(`(project/files/list)`, nil, nil, nil, func(string, int64, int64) (error, []byte) { return nil, nil })
	if err == nil {
		t.Fatal("RunTurnWithFiles() exposed project files without a linked project")
	}
	err, _ = RunTurnWithFilesAndProjectFiles(`(project/files/read "guide" 0 65537)`, nil, nil, nil, nil, &TurnProjectFiles{Read: func(string, int64, int64) (error, []byte) { return nil, nil }})
	if err == nil {
		t.Fatal("RunTurnWithFilesAndProjectFiles() accepted an oversized project file read")
	}
}

func TestRunTurnExposesLinkedProjectNotesSeparately(t *testing.T) {
	authorName := "Ada"
	err, result := RunTurnWithFilesAndProjectFilesAndNotes(`(list
  (project/notes/list)
  (bytes/utf8/decode (project/notes/read "guide" 0 7)))`, nil, nil, nil, nil, nil, nil, &TurnProjectNotes{Notes: []TurnProjectNote{{
		ID: "guide", Title: "Guide", AuthorID: "author", AuthorName: &authorName, CreatedAt: "2026-01-01T00:00:00.000Z",
	}}, Read: func(id string, offset, length int64) (error, []byte) {
		if id != "guide" || offset != 0 || length != 7 {
			t.Fatalf("project note reader = (%q, %d, %d)", id, offset, length)
		}
		return nil, []byte("# Guide")
	}})
	if err != nil || !strings.Contains(result.String(), `(title . "Guide")`) || !strings.Contains(result.String(), `(description . "")`) || !strings.Contains(result.String(), `(author_name . "Ada")`) || !strings.HasSuffix(result.String(), `"# Guide")`) {
		t.Fatalf("RunTurnWithFilesAndProjectFilesAndNotes() = (%s, %v)", result, err)
	}

	err, _ = RunTurnWithFiles(`(project/notes/list)`, nil, nil, nil, func(string, int64, int64) (error, []byte) { return nil, nil })
	if err == nil {
		t.Fatal("RunTurnWithFiles() exposed project notes without a linked project")
	}
	err, _ = RunTurnWithFilesAndProjectFilesAndNotes(`(project/notes/read "guide" 0 65537)`, nil, nil, nil, nil, nil, nil, &TurnProjectNotes{Read: func(string, int64, int64) (error, []byte) { return nil, nil }})
	if err == nil {
		t.Fatal("RunTurnWithFilesAndProjectFilesAndNotes() accepted an oversized project note read")
	}
}

func TestRunTurnExposesLinkedProjectInfo(t *testing.T) {
	name := "Roadmap"
	err, result := RunTurnWithFilesAndProjectFilesAndNotes(`(project/info)`, nil, nil, nil, nil, &TurnProjectInfo{Name: &name, CreatedAt: "2026-01-01T00:00:00.000Z"}, nil, nil)
	if err != nil || result.String() != `((name . "Roadmap") (description) (created_at . "2026-01-01T00:00:00.000Z"))` {
		t.Fatalf("RunTurnWithFilesAndProjectFilesAndNotes() = (%s, %v)", result, err)
	}
	err, _ = RunTurnWithFiles(`(project/info)`, nil, nil, nil, func(string, int64, int64) (error, []byte) { return nil, nil })
	if err == nil {
		t.Fatal("RunTurnWithFiles() exposed project info without a linked project")
	}
}
