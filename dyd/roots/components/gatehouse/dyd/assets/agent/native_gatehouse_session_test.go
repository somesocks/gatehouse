package agent

import (
	"io"
	"strings"
	"testing"

	"gatehouse/lisp"
	"gatehouse/model"
	"gatehouse/sessionsearch"
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
				return nil, SessionNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 1}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #f) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z") (revision . 1))` {
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
		Bindings: []lisp.Binding{{Name: "body", Value: lisp.MarkSensitive(lisp.String("# Decision"))}},
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Create: func(title, description, body string, sensitive bool) (error, SessionNote) {
			if title != "Decision" || description != "" || body != "# Decision" || !sensitive {
				t.Fatalf("sensitive session note create = (%q, %q, %q, %t)", title, description, body, sensitive)
			}
			return nil, SessionNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 1}
		}})},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #t) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z") (revision . 1))` {
		t.Fatalf("Evaluate() sensitive note = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/create title "" "# Decision"))`, lisp.EvalOptions{
		Bindings:    []lisp.Binding{{Name: "title", Value: lisp.MarkSensitive(lisp.String("Decision"))}},
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Create: func(string, string, string, bool) (error, SessionNote) { return nil, SessionNote{} }})},
	})
	if err == nil || !strings.Contains(err.Error(), "title and description must not be sensitive") {
		t.Fatalf("Evaluate() sensitive note metadata error = %v", err)
	}
}

func TestGatehouseSessionNoteUpdate(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/update "note" "Decision" "Updated" body))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "body", Value: lisp.MarkSensitive(lisp.String("# Decision"))}},
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, nil, nil, &SessionNotes{Update: func(id, title, description, body string, sensitive bool) (error, SessionNote) {
				if id != "note" || title != "Decision" || description != "Updated" || body != "# Decision" || !sensitive {
					t.Fatalf("session note update = (%q, %q, %q, %q, %t)", id, title, description, body, sensitive)
				}
				return nil, SessionNote{ID: id, Title: title, Description: description, Sensitive: true, AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 2}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "Updated") (sensitive . #t) (author_id . "agent") (author_name) (created_at . "2026-01-01T00:00:00.000Z") (revision . 2))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseSessionEventSearch(t *testing.T) {
	called := false
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/events/search "(and \"Conroe\" \"ordinance\")"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecretsTasksAndEventSearch(nil, nil, nil, nil, nil, nil, nil, nil, func(expression, cursor string) (error, SessionEventSearchResult) {
			called = true
			if expression != `(and "Conroe" "ordinance")` || cursor != "" {
				t.Fatalf("session event search = (%q, %q)", expression, cursor)
			}
			return nil, SessionEventSearchResult{Events: []sessionsearch.EventResult{{ID: "sev_00000000000000000000000000", Kind: "tool.success", Size: 16, Preview: "Conroe ordinance", Matches: []sessionsearch.Range{{Offset: 0, Length: 7}, {Offset: 8, Length: 9}}}}}
		})},
	})
	if err != nil || !called || !strings.Contains(result.String(), `"sev_00000000000000000000000000"`) || !strings.Contains(result.String(), `(offset . 8)`) {
		t.Fatalf("Evaluate() = (%s, %v), called = %t", result, err, called)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (error/value (error/catch (session/events/search "(not \"Conroe\")"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecretsTasksAndEventSearch(nil, nil, nil, nil, nil, nil, nil, nil, func(expression, cursor string) (error, SessionEventSearchResult) {
			return lisp.Errorf("unsupported expression"), SessionEventSearchResult{}
		})},
	})
	if err != nil || result.String() != `"session/events/search failed"` {
		t.Fatalf("Evaluate() invalid search = (%s, %v)", result, err)
	}
}

func TestGatehouseSessionFileSearch(t *testing.T) {
	fileID := "sfi_00000000000000000000000000"
	called := false
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/files/search-file "sfi_00000000000000000000000000" "(or \"Conroe\" \"ordinance\")"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecretsTasksAndEventSearch(nil, nil, nil, func(id, query, cursor string) (error, SessionFileSearchResult) {
			called = true
			if id != fileID || query != `(or "Conroe" "ordinance")` || cursor != "" {
				t.Fatalf("session file search = (%q, %q, %q)", id, query, cursor)
			}
			return nil, SessionFileSearchResult{Terms: []string{"Conroe", "ordinance"}, Matches: []SessionFileSearchMatch{{Offset: 0, Length: 7, TermIndex: 0, PreviewOffset: 0, Preview: "Conroe ordinance"}}, NextCursor: "cursor"}
		}, nil, nil, nil, nil, nil)},
	})
	if err != nil || !called || !strings.Contains(result.String(), `(term_index . 0)`) || !strings.Contains(result.String(), `(next_cursor . "cursor")`) {
		t.Fatalf("Evaluate() = (%s, %v), called = %t", result, err, called)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (error/value (error/catch (session/files/search-file "sfi_00000000000000000000000000" "Conroe"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil)},
	})
	if err != nil || result.String() != `"session/files/search-file is unavailable"` {
		t.Fatalf("Evaluate() unavailable search = (%s, %v)", result, err)
	}
}

func TestGatehouseSessionFileMutations(t *testing.T) {
	const fileID = "sfi_01m17ej89df8jnhnh7476ssnvg"
	mediaType := "text/plain"
	updated := false
	removed := []string{}
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (list
    (session/files/update "sfi_01m17ej89df8jnhnh7476ssnvg" "final.txt")
    (session/files/remove "sfi_01m17ej89df8jnhnh7476ssnvg")
    (session/files/remove "sfi_01m17ej89df8jnhnh7476ssnvh")))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecretsTasksAndEventSearch(nil, nil, nil, nil, nil, nil, nil, nil, nil, SessionFileActions{
			Update: func(id, name string) (error, File) {
				if id != fileID || name != "final.txt" {
					t.Fatalf("session file update = (%q, %q)", id, name)
				}
				updated = true
				return nil, File{ID: id, Name: name, MediaType: &mediaType, Size: 12, Fingerprint: "sha256:abc"}
			},
			Remove: func(id string) (error, bool) {
				removed = append(removed, id)
				return nil, id == fileID
			},
		})},
	})
	if err != nil || !updated || strings.Join(removed, ",") != fileID+",sfi_01m17ej89df8jnhnh7476ssnvh" || !strings.Contains(result.String(), `(name . "final.txt")`) || !strings.HasSuffix(result.String(), `#t #f)`) {
		t.Fatalf("Evaluate() = (%s, %v), updated = %t, removed = %#v", result, err, updated, removed)
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

func TestGatehouseSessionSecretRead(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/secrets/read "ssc_01m17ej89df8jnhnh7476ssnvg"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecrets(nil, nil, nil, nil, &SessionSecrets{Read: func(id string) (error, []byte) {
			if id != "ssc_01m17ej89df8jnhnh7476ssnvg" {
				t.Fatalf("session secret read id = %q", id)
			}
			return nil, []byte("secret")
		}})},
	})
	if err != nil || !lisp.IsSecret(result) {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
	bytesErr, value := lisp.RequireBytes(result)
	if bytesErr != nil || string(value) != "secret" {
		t.Fatalf("RequireBytes() = (%q, %v)", value, bytesErr)
	}
}

func TestGatehouseSessionSecretsList(t *testing.T) {
	authorName := "Ada"
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/secrets/list))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecrets(nil, nil, nil, nil, &SessionSecrets{Secrets: []SessionSecret{
			{ID: "ssc_01m17ej89df8jnhnh7476ssnvg", Description: "Deployment token", AuthorID: "prn_01m17ej89df8jnhnh7476ssnvg", AuthorName: &authorName, CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z"},
		}})},
	})
	if err != nil || lisp.TaintOf(result) != lisp.TaintNone || result.String() != `(((id . "ssc_01m17ej89df8jnhnh7476ssnvg") (description . "Deployment token") (author_id . "prn_01m17ej89df8jnhnh7476ssnvg") (author_name . "Ada") (created_at . "2026-01-01T00:00:00.000Z") (updated_at . "2026-01-02T00:00:00.000Z")))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (error/value (error/catch (session/secrets/list "unexpected"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModuleWithSecrets(nil, nil, nil, nil, &SessionSecrets{})},
	})
	if err != nil || result.String() != `"session/secrets/list requires no arguments"` {
		t.Fatalf("Evaluate() invalid session secrets list = (%s, %v)", result, err)
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
		{name: "tool request", event: model.SessionEvent{Kind: "tool.request", Payload: map[string]interface{}{"name": "lisp", "call_id": "call", "code": `say "Conroe"`, "reason": "Explain the quote."}}, offset: 0, length: 4096, output: "say \"Conroe\"\nExplain the quote."},
		{name: "approval request", event: model.SessionEvent{Kind: "approval.request", Payload: map[string]interface{}{"description": `Approve "Conroe"`}}, offset: 0, length: 4096, output: `Approve "Conroe"`},
		{name: "other kind", event: model.SessionEvent{Kind: "thinking.completed", Payload: map[string]interface{}{}}, offset: 0, length: 1, output: "{"},
		{name: "missing payload", event: model.SessionEvent{Kind: "message.text", Payload: map[string]interface{}{}}, offset: 0, length: 1, fails: true},
		{name: "invalid tool request", event: model.SessionEvent{Kind: "tool.request", Payload: map[string]interface{}{"name": "lisp", "call_id": "call", "code": "", "reason": "reason"}}, offset: 0, length: 1, fails: true},
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
	if err != nil || !lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() = (%s, %v), sensitive = %t", result, err, lisp.IsSensitive(result))
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
	if err != nil || lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() = (%s, %v), sensitive = %t", result, err, lisp.IsSensitive(result))
	}
	bytesErr, bytes = lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "guide" {
		t.Fatalf("RequireBytes() = (%q, %v)", bytes, bytesErr)
	}
}

func TestGatehouseSessionNoteRevisions(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/revisions/list "guide"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{Revisions: func(id string) (error, []NoteRevision) {
			if id != "guide" {
				t.Fatalf("session note revisions id = %q", id)
			}
			return nil, []NoteRevision{{Revision: 1, Title: "Guide", Description: "Original", Sensitive: false, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		}})},
	})
	if err != nil || result.String() != `(((revision . 1) (title . "Guide") (description . "Original") (sensitive . #f) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z")))` {
		t.Fatalf("Evaluate() revisions = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/notes/revisions/read "guide" 1 0 5))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, &SessionNotes{RevisionRead: func(id string, revision int, offset, length int64) (error, []byte, bool) {
			if id != "guide" || revision != 1 || offset != 0 || length != 5 {
				t.Fatalf("session note revision read = (%q, %d, %d, %d)", id, revision, offset, length)
			}
			return nil, []byte("Guide"), false
		}})},
	})
	if err != nil || lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() revision read = (%s, %v), sensitive = %t", result, err, lisp.IsSensitive(result))
	}
	bytesErr, bytes := lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "Guide" {
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
