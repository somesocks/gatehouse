package agent

import (
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseProjectModuleIsAvailableWithoutProject(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (project/info/get)
  (project/files/list)
  (project/notes/list)
  (error/value (error/catch (project/files/read "guide" 0 1)))
  (error/value (error/catch (project/notes/read "guide" 0 1)))
  (error/value (error/catch (project/notes/create "Decision" "" "# Decision")))
  (error/value (error/catch (project/info/set "Roadmap" "Current priorities"))))`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, nil, nil), NewPolicyModule(nil)},
	})
	if err != nil || result.String() != `(null null null "project/files/read is unavailable" "project/notes/read is unavailable" "policy/await-approval is unavailable" "policy/await-approval is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectInfoSet(t *testing.T) {
	name, description := "Roadmap", "Current priorities"
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/info/get)
    (project/info/set "Roadmap" "Current priorities")))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewProjectModule(&ProjectInfo{CreatedAt: "2026-01-01T00:00:00.000Z", Set: func(gotName, gotDescription string) (error, ProjectInfo) {
				if gotName != name || gotDescription != description {
					t.Fatalf("project info set = (%q, %q)", gotName, gotDescription)
				}
				return nil, ProjectInfo{Name: &name, Description: &description, CreatedAt: "2026-01-01T00:00:00.000Z"}
			}}, nil, nil),
		},
	})
	if err != nil || result.String() != `(((name) (description) (created_at . "2026-01-01T00:00:00.000Z")) ((name . "Roadmap") (description . "Current priorities") (created_at . "2026-01-01T00:00:00.000Z")))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/create "Decision" "" "# Decision"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, &ProjectNotes{Create: func(title, description, body string) (error, ProjectNote) {
				if title != "Decision" || description != "" || body != "# Decision" {
					t.Fatalf("project note create = (%q, %q, %q)", title, description, body)
				}
				return nil, ProjectNote{ID: "note", Title: title, Description: description, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z"))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}
