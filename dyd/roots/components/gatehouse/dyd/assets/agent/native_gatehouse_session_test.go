package agent

import (
	"strings"
	"testing"

	"gatehouse/lisp"
)

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

func TestGatehouseSessionNoteRemove(t *testing.T) {
	removed := []string{}
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (list
    (session/notes/remove "note")
    (session/notes/remove "missing")))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, nil, &SessionNotes{Remove: func(id string) (error, bool) {
				removed = append(removed, id)
				return nil, id == "note"
			}}),
		},
	})
	if err != nil || result.String() != `(#t #f)` || strings.Join(removed, ",") != "note,missing" {
		t.Fatalf("Evaluate() = (%s, %v), removed = %#v", result, err, removed)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (error/value (error/catch (session/notes/remove "note"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil)},
	})
	if err != nil || result.String() != `"session/notes/remove is unavailable"` {
		t.Fatalf("Evaluate() unavailable session note remove = (%s, %v)", result, err)
	}
}
