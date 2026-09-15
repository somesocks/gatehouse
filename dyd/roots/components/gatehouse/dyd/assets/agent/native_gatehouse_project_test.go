package agent

import (
	"io"
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseProjectFileCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(project/files/create "report.txt" "text/plain" (seq/from (bytes/utf8/encode "Generated ") (bytes/utf8/encode "report")))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: agentResourceModules([]lisp.HostModule{NewProjectModule(nil, &ProjectFiles{Create: func(name, mediaType string, source io.Reader) (error, string) {
			contents, err := io.ReadAll(source)
			if err != nil {
				return err, ""
			}
			if name != "report.txt" || mediaType != "text/plain" || string(contents) != "Generated report" {
				t.Fatalf("project file create = (%q, %q, %q)", name, mediaType, contents)
			}
			return nil, "pfi_01m17ej89df8jnhnh7476ssnvg"
		}}, nil), NewSessionModule(nil, nil, nil, nil), NewPolicyModule(nil), NewWebModule()}),
	})
	if err != nil || result.String() != `"pfi_01m17ej89df8jnhnh7476ssnvg"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(project/files/create "report.txt" "text/plain" chunks)`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		Bindings:    []lisp.Binding{{Name: "chunks", Value: lisp.List(lisp.MarkSensitive(lisp.Bytes([]byte("secret"))))}},
		HostModules: agentResourceModules([]lisp.HostModule{NewProjectModule(nil, &ProjectFiles{Create: func(_ string, _ string, source io.Reader) (error, string) {
			_, err := io.ReadAll(source)
			if err == nil {
				t.Fatalf("project file create sensitive contents error = %v", err)
			}
			return err, ""
		}}, nil), NewSessionModule(nil, nil, nil, nil), NewPolicyModule(nil), NewWebModule()}),
	})
	if err == nil {
		t.Fatalf("Evaluate() sensitive file contents error = %v", err)
	}
}

func TestGatehouseProjectFileMutationsAndSearch(t *testing.T) {
	const fileID = "pfi_01m17ej89df8jnhnh7476ssnvg"
	mediaType := "text/plain"
	updated := false
	removed := []string{}
	searched := false
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/files/update "pfi_01m17ej89df8jnhnh7476ssnvg" "final.txt")
    (project/files/remove "pfi_01m17ej89df8jnhnh7476ssnvg")
    (project/files/search-file "pfi_01m17ej89df8jnhnh7476ssnvg" "Conroe")))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, &ProjectFiles{
			Update: func(id, name string) (error, File) {
				if id != fileID || name != "final.txt" {
					t.Fatalf("project file update = (%q, %q)", id, name)
				}
				updated = true
				return nil, File{ID: id, Name: name, MediaType: &mediaType, Size: 12, Fingerprint: "sha256:abc"}
			},
			Remove: func(id string) (error, bool) {
				removed = append(removed, id)
				return nil, true
			},
			Search: func(id, query, cursor string) (error, SessionFileSearchResult) {
				if id != fileID || query != "Conroe" || cursor != "" {
					t.Fatalf("project file search = (%q, %q, %q)", id, query, cursor)
				}
				searched = true
				return nil, SessionFileSearchResult{Terms: []string{"Conroe"}, Matches: []SessionFileSearchMatch{{Offset: 0, Length: 7, TermIndex: 0, PreviewOffset: 0, Preview: "Conroe ordinance"}}}
			},
		}, nil)},
	})
	if err != nil || !updated || !searched || strings.Join(removed, ",") != fileID || !strings.Contains(result.String(), `(name . "final.txt")`) || !strings.Contains(result.String(), `(preview . "Conroe ordinance")`) {
		t.Fatalf("Evaluate() = (%s, %v), updated = %t, searched = %t, removed = %#v", result, err, updated, searched, removed)
	}
}

func TestGatehouseProjectModuleIsAvailableWithoutProject(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/info/get)
    (project/files/list)
    (project/files/info "pfi_01m17ej89df8jnhnh7476ssnvg")
    (project/notes/list)
	(project/secrets/list)
    (error/value (error/catch (project/files/read "guide" 0 1)))
    (error/value (error/catch (project/notes/read "guide" 0 1)))
	(error/value (error/catch (project/notes/revisions/list "guide")))
	(error/value (error/catch (project/notes/revisions/read "guide" 1 0 1)))
    (error/value (error/catch (project/notes/create "Decision" "" "# Decision")))
    (error/value (error/catch (project/notes/remove "example-note-id")))
	(error/value (error/catch (project/secrets/read "psc_01m17ej89df8jnhnh7476ssnvg")))
    (error/value (error/catch (project/info/set "Roadmap" "Current priorities")))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil)},
	})
	if err != nil || result.String() != `(null null null null null "project/files/read is unavailable" "project/notes/read is unavailable" "project/notes/revisions/list is unavailable" "project/notes/revisions/read is unavailable" "project/notes/create is unavailable" "project/notes/remove is unavailable" "project/secrets/read is unavailable" "project/info/set is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectSecretsListAndRead(t *testing.T) {
	authorName := "Ada"
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/secrets/list))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModuleWithSecrets(nil, nil, nil, &ProjectSecrets{Secrets: []ProjectSecret{
			{ID: "psc_01m17ej89df8jnhnh7476ssnvg", Description: "Deployment token", AuthorID: "prn_01m17ej89df8jnhnh7476ssnvg", AuthorName: &authorName, CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z"},
		}})},
	})
	if err != nil || lisp.TaintOf(result) != lisp.TaintNone || result.String() != `(((id . "psc_01m17ej89df8jnhnh7476ssnvg") (description . "Deployment token") (author_id . "prn_01m17ej89df8jnhnh7476ssnvg") (author_name . "Ada") (created_at . "2026-01-01T00:00:00.000Z") (updated_at . "2026-01-02T00:00:00.000Z")))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/secrets/read "psc_01m17ej89df8jnhnh7476ssnvg"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModuleWithSecrets(nil, nil, nil, &ProjectSecrets{Read: func(id string) (error, []byte) {
			if id != "psc_01m17ej89df8jnhnh7476ssnvg" {
				t.Fatalf("project secret read id = %q", id)
			}
			return nil, []byte("secret")
		}})},
	})
	if err != nil || !lisp.IsSecret(result) {
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
				return nil, ProjectNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 1}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #f) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z") (revision . 1))` {
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
			return nil, ProjectNote{ID: "note", Title: title, Description: description, Sensitive: sensitive, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 1}
	}})},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "") (sensitive . #t) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z") (revision . 1))` {
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

func TestGatehouseProjectNoteUpdate(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/update "note" "Decision" "Updated" body))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "body", Value: lisp.MarkSensitive(lisp.String("# Decision"))}},
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, &ProjectNotes{Update: func(id, title, description, body string, sensitive bool) (error, ProjectNote) {
			if id != "note" || title != "Decision" || description != "Updated" || body != "# Decision" || !sensitive {
				t.Fatalf("project note update = (%q, %q, %q, %q, %t)", id, title, description, body, sensitive)
			}
			return nil, ProjectNote{ID: id, Title: title, Description: description, Sensitive: true, AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 2}
			}}),
		},
	})
	if err != nil || result.String() != `((id . "note") (title . "Decision") (description . "Updated") (sensitive . #t) (author_id . "agent") (author_name) (created_at . "2026-01-01T00:00:00.000Z") (revision . 2))` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
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

func TestGatehouseProjectNoteRevisions(t *testing.T) {
	authorName := "Ada"
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/revisions/list "guide"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, &ProjectNotes{Revisions: func(id string) (error, []NoteRevision) {
			if id != "guide" {
				t.Fatalf("project note revisions id = %q", id)
			}
			return nil, []NoteRevision{{Revision: 2, Title: "Guide", Description: "Updated", Sensitive: false, AuthorID: "agent", AuthorName: &authorName, CreatedAt: "2026-01-02T00:00:00.000Z"}, {Revision: 1, Title: "Guide", Description: "Original", Sensitive: true, AuthorID: "author", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		}})},
	})
	if err != nil || result.String() != `(((revision . 2) (title . "Guide") (description . "Updated") (sensitive . #f) (author_id . "agent") (author_name . "Ada") (created_at . "2026-01-02T00:00:00.000Z")) ((revision . 1) (title . "Guide") (description . "Original") (sensitive . #t) (author_id . "author") (author_name) (created_at . "2026-01-01T00:00:00.000Z")))` {
		t.Fatalf("Evaluate() revisions = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/notes/revisions/read "guide" 1 2 7))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, &ProjectNotes{RevisionRead: func(id string, revision int, offset, length int64) (error, []byte, bool) {
			if id != "guide" || revision != 1 || offset != 2 || length != 7 {
				t.Fatalf("project note revision read = (%q, %d, %d, %d)", id, revision, offset, length)
			}
			return nil, []byte("Original"), true
		}})},
	})
	if err != nil || !lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() revision read = (%s, %v), sensitive = %t", result, err, lisp.IsSensitive(result))
	}
	bytesErr, bytes := lisp.RequireBytes(result)
	if bytesErr != nil || string(bytes) != "Original" {
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
