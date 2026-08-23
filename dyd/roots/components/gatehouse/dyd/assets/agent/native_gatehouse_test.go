package agent

import (
	"errors"
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseCapabilityModulesExposeAuthorizedValues(t *testing.T) {
	mediaType := "text/plain"
	authorName := "Ada"
	err, result := lisp.Evaluate(`(list
  (project/info/get)
  (session/files/list)
  (bytes/utf8/decode (session/files/read "report" 6 5))
  (project/notes/list)
  (bytes/utf8/decode (project/notes/read "guide" 0 7)))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule([]File{{ID: "report", Name: "report.txt", MediaType: &mediaType, Size: 11, Fingerprint: "sha256:report"}}, func(id string, offset, length int64) (error, []byte) {
				if id != "report" || offset != 6 || length != 5 {
					t.Fatalf("session file read = (%q, %d, %d)", id, offset, length)
				}
				return nil, []byte("world")
			}, nil, nil),
			NewProjectModule(&ProjectInfo{CreatedAt: "2026-01-01T00:00:00.000Z"}, nil, &ProjectNotes{Notes: []ProjectNote{{ID: "guide", Title: "Guide", AuthorID: "author", AuthorName: &authorName, CreatedAt: "2026-01-01T00:00:00.000Z"}}, Read: func(id string, offset, length int64) (error, []byte, bool) {
				if id != "guide" || offset != 0 || length != 7 {
					t.Fatalf("project note read = (%q, %d, %d)", id, offset, length)
				}
				return nil, []byte("# Guide"), false
			}}),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || !strings.Contains(result.String(), `(name . "report.txt")`) || !strings.Contains(result.String(), `(author_name . "Ada")`) || !strings.HasSuffix(result.String(), `"# Guide")`) {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseFileInfoReturnsAuthorizedMetadata(t *testing.T) {
	mediaType := "text/plain"
	err, result := lisp.Evaluate(`(list
  (session/files/info "session-file")
  (session/files/info "missing")
  (project/files/info "project-file")
  (project/files/info "missing"))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule([]File{{ID: "session-file", Name: "session.txt", MediaType: &mediaType, Size: 12, Fingerprint: "sha256:session"}}, nil, nil, nil),
			NewProjectModule(nil, &ProjectFiles{Files: []File{{ID: "project-file", Name: "project.txt", Size: 34, Fingerprint: "sha256:project"}}}, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != `(((id . "session-file") (name . "session.txt") (media_type . "text/plain") (size . 12) (fingerprint . "sha256:session")) null ((id . "project-file") (name . "project.txt") (media_type) (size . 34) (fingerprint . "sha256:project")) null)` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseFileInfoValidatesID(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (error/value (error/catch (session/files/info "")))
  (error/value (error/catch (project/files/info "")))
  (error/value (error/catch (session/files/info))))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, nil, nil, nil),
			NewProjectModule(nil, nil, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != `("session/files/info requires a non-empty id" "project/files/info requires a non-empty id" "session/files/info requires an id")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseCapabilityReadsValidateAndHideFailures(t *testing.T) {
	err, _ := lisp.Evaluate(`(session/files/read "file" 0 65537)`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return nil, nil }, nil, nil), NewPolicyModule(nil), NewWebModule()},
	})
	if err == nil || !strings.Contains(err.Error(), "length from 1 through 65536") {
		t.Fatalf("Evaluate() oversized read error = %v", err)
	}

	err, result := lisp.Evaluate(`(error/value (error/catch (session/files/read "file" 0 1)))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return errors.New("storage unavailable"), nil }, nil, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != `"session/files/read failed"` {
		t.Fatalf("Evaluate() failed read = (%s, %v)", result, err)
	}
}

func TestAgentPreludeHidesImportBindings(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (error? (error/catch (import)))
  (error? (error/catch (import/restrict)))
  (error? (error/catch (import/search))))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, nil, nil, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != "(#t #t #t)" {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}
