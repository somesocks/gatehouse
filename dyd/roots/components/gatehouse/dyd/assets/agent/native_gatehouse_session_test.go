package agent

import (
	"io"
	"strings"
	"testing"

	"gatehouse/lisp"
	"gatehouse/model"
)

func TestGatehouseSessionNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create "Decision" "" "# Decision"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, nil, nil, &SessionNotes{Create: func(title, description, body string, sensitive bool) (error, SessionNote) {
				if title != "Decision" || description != "" || body != "# Decision" || sensitive {
					t.Fatalf("session note create = (%q, %q, %q, %t)", title, description, body, sensitive)
				}
				return nil, SessionNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #f) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z"))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create "Decision" ""))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Create: func(string, string, string, bool) (error, SessionNote) { return nil, SessionNote{} }})},
	})
	if err == nil || !strings.Contains(err.Error(), "requires title, description, and body") {
		t.Fatalf("Evaluate() invalid session note create error = %v", err)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (error/value (error/catch (session/notes/create "Decision" "" "# Decision"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil)},
	})
	if err != nil || result.String() != `"session/notes/create is unavailable"` {
		t.Fatalf("Evaluate() unavailable session note create = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create "Decision" "" body))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "body", Value: lisp.MarkSecret(lisp.String("# Decision"))}},
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Create: func(title, description, body string, sensitive bool) (error, SessionNote) {
			if title != "Decision" || description != "" || body != "# Decision" || !sensitive {
				t.Fatalf("sensitive session note create = (%q, %q, %q, %t)", title, description, body, sensitive)
			}
			return nil, SessionNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}
		}})},
	})
	if err != nil || result.String() != "#<secret>" {
		t.Fatalf("Evaluate() sensitive note = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create title "" "# Decision"))`, lisp.EvalOptions{
		Bindings:    []lisp.Binding{{Name: "title", Value: lisp.MarkSecret(lisp.String("Decision"))}},
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Create: func(string, string, string, bool) (error, SessionNote) { return nil, SessionNote{} }})},
	})
	if err == nil || !strings.Contains(err.Error(), "title and description must not be sensitive") {
		t.Fatalf("Evaluate() sensitive note metadata error = %v", err)
	}
}

func TestGatehouseSessionEventRead(t *testing.T) {
	reads := []struct {
		id     string
		offset int64
		length int64
	}{
		{id: "message", offset: 1, length: 4},
		{id: "tool-success", offset: 0, length: 4},
		{id: "tool-failure", offset: 2, length: 8},
	}
	values := map[string][]byte{
		"message":      []byte("ello"),
		"tool-success": []byte("done"),
		"tool-failure": []byte("failure"),
	}
	index := 0
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (list
    (bytes/utf8/decode (session/events/read "message" 1 4))
    (bytes/utf8/decode (session/events/read "tool-success" 0 4))
    (bytes/utf8/decode (session/events/read "tool-failure" 2 8))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil, func(id string, offset, length int64) (error, []byte) {
			if index >= len(reads) || reads[index].id != id || reads[index].offset != offset || reads[index].length != length {
				t.Fatalf("session event read %d = (%q, %d, %d)", index, id, offset, length)
			}
			index++
			return nil, values[id]
		})},
	})
	if err != nil || index != len(reads) || result.String() != `("ello" "done" "failure")` {
		t.Fatalf("Evaluate() = (%s, %v), reads = %d", result, err, index)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (list
    (error/value (error/catch (session/events/read "event" 0 4097)))
    (error/value (error/catch (session/events/read "missing" 0 1)))
    (error/value (error/catch (session/events/read "other" 0 1)))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil, func(id string, offset, length int64) (error, []byte) {
			return lisp.Errorf("unavailable"), nil
		})},
	})
	if err != nil || result.String() != `("session/events/read requires a non-empty id, non-negative offset, and length from 1 through 4096" "session/events/read is unavailable" "session/events/read is unavailable")` {
		t.Fatalf("Evaluate() unavailable or invalid session event read = (%s, %v)", result, err)
	}
}

func TestSessionEventReadRange(t *testing.T) {
	for _, test := range []struct {
		name   string
		event  model.SessionEvent
		offset int64
		length int64
		output string
		fails  bool
	}{
		{name: "message text", event: model.SessionEvent{Kind: "message.text", Payload: map[string]interface{}{"text": "hello"}}, offset: 1, length: 3, output: "ell"},
		{name: "message UTF-8 bytes", event: model.SessionEvent{Kind: "message.text", Payload: map[string]interface{}{"text": "aéz"}}, offset: 1, length: 2, output: "é"},
		{name: "tool success", event: model.SessionEvent{Kind: "tool.success", Payload: map[string]interface{}{"output": "done"}}, offset: 0, length: 4096, output: "done"},
		{name: "tool failure", event: model.SessionEvent{Kind: "tool.failure", Payload: map[string]interface{}{"output": "failure"}}, offset: 2, length: 8, output: "ilure"},
		{name: "other kind", event: model.SessionEvent{Kind: "thinking.completed", Payload: map[string]interface{}{}}, offset: 0, length: 1, output: "{"},
		{name: "missing payload", event: model.SessionEvent{Kind: "message.text", Payload: map[string]interface{}{}}, offset: 0, length: 1, fails: true},
		{name: "unavailable offset", event: model.SessionEvent{Kind: "message.text", Payload: map[string]interface{}{"text": "hello"}}, offset: 6, length: 1, fails: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, value := sessionEventReadRange(test.event, test.offset, test.length)
			if test.fails {
				if err == nil || value != nil {
					t.Fatalf("sessionEventReadRange() = (%q, %v), want unavailable", value, err)
				}
				return
			}
			if err != nil || string(value) != test.output {
				t.Fatalf("sessionEventReadRange() = (%q, %v), want %q", value, err, test.output)
			}
		})
	}
}

func TestGatehouseSessionNoteReadMarksSensitiveBytes(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/read "credentials" 0 6))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Read: func(id string, offset, length int64) (error, []byte, bool) {
			if id != "credentials" || offset != 0 || length != 6 {
				t.Fatalf("session note read = (%q, %d, %d)", id, offset, length)
			}
			return nil, []byte("secret"), true
		}})},
	})
	if err != nil || !lisp.IsSecret(result) {
		t.Fatalf("Evaluate() = (%s, %v), sensitive = %t", result, err, lisp.IsSecret(result))
	}
	bytesErr, bytes := lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "secret" {
		t.Fatalf("RequireBytes() = (%q, %v)", bytes, bytesErr)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/read "guide" 0 5))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Read: func(string, int64, int64) (error, []byte, bool) {
			return nil, []byte("guide"), false
		}})},
	})
	if err != nil || lisp.IsSecret(result) {
		t.Fatalf("Evaluate() = (%s, %v), sensitive = %t", result, err, lisp.IsSecret(result))
	}
	bytesErr, bytes = lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "guide" {
		t.Fatalf("RequireBytes() = (%q, %v)", bytes, bytesErr)
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
			NewSessionModule(nil, nil, nil, &SessionNotes{Remove: func(id string) (error, bool) {
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
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil)},
	})
	if err != nil || result.String() != `"session/notes/remove is unavailable"` {
		t.Fatalf("Evaluate() unavailable session note remove = (%s, %v)", result, err)
	}
}

func TestGatehouseSessionFileCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(session/files/create "report.txt" "text/plain" (seq/from (bytes/utf8/encode "Generated ") (bytes/utf8/encode "report")))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, nil, func(name, mediaType string, source io.Reader) (error, string) {
				contents, err := io.ReadAll(source)
				if err != nil || name != "report.txt" || mediaType != "text/plain" || string(contents) != "Generated report" {
					t.Fatalf("session file create = (%q, %q, %q, %v)", name, mediaType, contents, err)
				}
				return nil, "file"
			}, nil),
			NewProjectModule(nil, nil, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != `"file"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(session/files/create "" "text/plain" (seq/from (bytes/utf8/encode "Generated report")))`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, func(string, string, io.Reader) (error, string) { return nil, "file" }, nil), NewProjectModule(nil, nil, nil), NewPolicyModule(nil), NewWebModule()},
	})
	if err == nil || !strings.Contains(err.Error(), "requires non-empty name and media_type") {
		t.Fatalf("Evaluate() invalid session file create error = %v", err)
	}

	err, result = lisp.Evaluate(`(error/value (error/catch (session/files/create "report.txt" "text/plain" (seq/from (bytes/utf8/encode "Generated report")))))`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil), NewProjectModule(nil, nil, nil), NewPolicyModule(nil), NewWebModule()},
	})
	if err != nil || result.String() != `"session/files/create is unavailable"` {
		t.Fatalf("Evaluate() unavailable session file create = (%s, %v)", result, err)
	}
}
