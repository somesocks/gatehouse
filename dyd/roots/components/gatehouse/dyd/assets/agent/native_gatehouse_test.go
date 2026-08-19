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
  (project/info)
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
			}, nil),
			NewProjectModule(&ProjectInfo{CreatedAt: "2026-01-01T00:00:00.000Z"}, nil, &ProjectNotes{Notes: []ProjectNote{{ID: "guide", Title: "Guide", AuthorID: "author", AuthorName: &authorName, CreatedAt: "2026-01-01T00:00:00.000Z"}}, Read: func(id string, offset, length int64) (error, []byte) {
				if id != "guide" || offset != 0 || length != 7 {
					t.Fatalf("project note read = (%q, %d, %d)", id, offset, length)
				}
				return nil, []byte("# Guide")
			}}),
			NewPolicyModule(nil),
		},
	})
	if err != nil || !strings.Contains(result.String(), `(name . "report.txt")`) || !strings.Contains(result.String(), `(author_name . "Ada")`) || !strings.HasSuffix(result.String(), `"# Guide")`) {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseCapabilityReadsValidateAndHideFailures(t *testing.T) {
	err, _ := lisp.Evaluate(`(session/files/read "file" 0 65537)`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return nil, nil }, nil), NewPolicyModule(nil)},
	})
	if err == nil || !strings.Contains(err.Error(), "length from 1 through 65536") {
		t.Fatalf("Evaluate() oversized read error = %v", err)
	}

	err, result := lisp.Evaluate(`(error/value (error/catch (session/files/read "file" 0 1)))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return errors.New("storage unavailable"), nil }, nil),
			NewPolicyModule(nil),
		},
	})
	if err != nil || result.String() != `"session/files/read failed"` {
		t.Fatalf("Evaluate() failed read = (%s, %v)", result, err)
	}
}
