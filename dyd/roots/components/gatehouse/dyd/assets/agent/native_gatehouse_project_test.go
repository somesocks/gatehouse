package agent

import (
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseProjectModuleIsAvailableWithoutProject(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/info/get)
    (project/files/list)
    (project/files/info "guide")
    (project/notes/list)
    (error/value (error/catch (project/files/read "guide" 0 1)))
    (error/value (error/catch (project/notes/read "guide" 0 1)))
    (error/value (error/catch (project/notes/create "Decision" "" "# Decision")))
    (error/value (error/catch (project/notes/remove "example-note-id")))
    (error/value (error/catch (project/info/set "Roadmap" "Current priorities")))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil)},
	})
	if err != nil || result.String() != `(null null null null "project/files/read is unavailable" "project/notes/read is unavailable" "project/notes/create is unavailable" "project/notes/remove is unavailable" "project/info/set is unavailable")` {
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
			NewProjectModule(nil, nil, &ProjectNotes{Create: func(title, description, body string, sensitive bool) (error, ProjectNote) {
				if title != "Decision" || description != "" || body != "# Decision" || sensitive {
					t.Fatalf("project note create = (%q, %q, %q, %t)", title, description, body, sensitive)
				}
				return nil, ProjectNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #f) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z"))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/create "Decision" "" body))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "body", Value: lisp.MarkSensitive(lisp.String("# Decision"))}},
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, &ProjectNotes{Create: func(title, description, body string, sensitive bool) (error, ProjectNote) {
			if title != "Decision" || description != "" || body != "# Decision" || !sensitive {
				t.Fatalf("sensitive project note create = (%q, %q, %q, %t)", title, description, body, sensitive)
			}
			return nil, ProjectNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}
	}})},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #t) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z"))` {
		t.Fatalf("Evaluate() sensitive note = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/create title "" "# Decision"))`, lisp.EvalOptions{
		Bindings:    []lisp.Binding{{Name: "title", Value: lisp.MarkSensitive(lisp.String("Decision"))}},
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, &ProjectNotes{Create: func(string, string, string, bool) (error, ProjectNote) { return nil, ProjectNote{} }})},
	})
	if err == nil || !strings.Contains(err.Error(), "title and description must not be sensitive") {
		t.Fatalf("Evaluate() sensitive note metadata error = %v", err)
	}
}

func TestGatehouseProjectNoteReadMarksSensitiveBytes(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/read "credentials" 0 6))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, &ProjectNotes{Read: func(id string, offset, length int64) (error, []byte, bool) {
			if id != "credentials" || offset != 0 || length != 6 {
				t.Fatalf("project note read = (%q, %d, %d)", id, offset, length)
			}
			return nil, []byte("secret"), true
		}})},
	})
	if err != nil || !lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() = (%s, %v), sensitive = %t", result, err, lisp.IsSensitive(result))
	}
	bytesErr, bytes := lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "secret" {
		t.Fatalf("RequireBytes() = (%q, %v)", bytes, bytesErr)
	}

	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/read "guide" 0 5))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, &ProjectNotes{Read: func(string, int64, int64) (error, []byte, bool) {
			return nil, []byte("guide"), false
		}})},
	})
	if err != nil || lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() = (%s, %v), sensitive = %t", result, err, lisp.IsSensitive(result))
	}
	bytesErr, bytes = lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "guide" {
		t.Fatalf("RequireBytes() = (%q, %v)", bytes, bytesErr)
	}
}

func TestGatehouseProjectNoteRemove(t *testing.T) {
	removed := []string{}
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/notes/remove "note")
    (project/notes/remove "missing")))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, &ProjectNotes{Remove: func(id string) (error, bool) {
				removed = append(removed, id)
				return nil, id == "note"
			}}),
		},
	})
	if err != nil || result.String() != `(#t #f)` || len(removed) != 2 || removed[0] != "note" || removed[1] != "missing" {
		t.Fatalf("Evaluate() = (%s, %v), removed = %#v", result, err, removed)
	}
}
