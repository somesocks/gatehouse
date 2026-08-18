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
			NewPolicyModule(),
		},
	})
	if err != nil || !strings.Contains(result.String(), `(name . "report.txt")`) || !strings.Contains(result.String(), `(author_name . "Ada")`) || !strings.HasSuffix(result.String(), `"# Guide")`) {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectModuleIsAvailableWithoutProject(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (project/info)
  (project/files/list)
  (project/notes/list)
  (error/value (error/catch (project/files/read "guide" 0 1)))
  (error/value (error/catch (project/notes/read "guide" 0 1))))`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, nil, nil), NewPolicyModule()},
	})
	if err != nil || result.String() != `(null null null "project/files/read is unavailable" "project/notes/read is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseCapabilityReadsValidateAndHideFailures(t *testing.T) {
	err, _ := lisp.Evaluate(`(session/files/read "file" 0 65537)`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return nil, nil }, nil), NewPolicyModule()},
	})
	if err == nil || !strings.Contains(err.Error(), "length from 1 through 65536") {
		t.Fatalf("Evaluate() oversized read error = %v", err)
	}

	err, result := lisp.Evaluate(`(error/value (error/catch (session/files/read "file" 0 1)))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return errors.New("storage unavailable"), nil }, nil),
			NewPolicyModule(),
		},
	})
	if err != nil || result.String() != `"session/files/read failed"` {
		t.Fatalf("Evaluate() failed read = (%s, %v)", result, err)
	}
}

func TestGatehouseSessionNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create "Decision" "" "# Decision"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, nil, &SessionNotes{Create: func(title, description, body string) (error, ProjectNote) {
				if title != "Decision" || description != "" || body != "# Decision" {
					t.Fatalf("session note create = (%q, %q, %q)", title, description, body)
				}
				return nil, ProjectNote{ID: "note", Title: title, Description: description, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z"))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create "Decision" ""))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, &SessionNotes{Create: func(string, string, string) (error, ProjectNote) { return nil, ProjectNote{} }})},
	})
	if err == nil || !strings.Contains(err.Error(), "requires title, description, and body") {
		t.Fatalf("Evaluate() invalid session note create error = %v", err)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (error/value (error/catch (session/notes/create "Decision" "" "# Decision"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil)},
	})
	if err != nil || result.String() != `"session/notes/create is unavailable"` {
		t.Fatalf("Evaluate() unavailable session note create = (%s, %v)", result, err)
	}
}

func TestAgentPreludeRequiresApprovalForSessionNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (error/value (error/catch (policy/await-approval)))
  (error/value (error/catch ((policy/require-approval +) 1 2)))
  (error/value (error/catch (session/notes/create "Decision" "" "# Decision"))))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, nil, &SessionNotes{Create: func(string, string, string) (error, ProjectNote) {
				t.Fatal("session note create was called without approval")
				return nil, ProjectNote{}
			}}),
			NewPolicyModule(),
		},
	})
	if err != nil || result.String() != `("policy/await-approval is unavailable" "policy/await-approval is unavailable" "policy/await-approval is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehousePolicyModuleAwaitApprovalIsUnavailable(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (policy @native:gatehouse/policy/v1)
  (list
    (error/value (error/catch (policy/await-approval)))
    (error/value (error/catch ((policy/require-approval +) 1 2)))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewPolicyModule()},
	})
	if err != nil || result.String() != `("policy/await-approval is unavailable" "policy/await-approval is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}
