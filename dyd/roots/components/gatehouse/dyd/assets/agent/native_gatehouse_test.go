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
	sessionFileID := "sfi_00000000000000000000000000"
	err, result := lisp.Evaluate(`(list
  (project/info/get)
  (session/files/list)
  (bytes/utf8/decode (session/files/read "sfi_00000000000000000000000000" 6 5))
  (project/notes/list)
  (bytes/utf8/decode (project/notes/read "guide" 0 7)))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule([]File{{ID: sessionFileID, Name: "report.txt", MediaType: &mediaType, Size: 11, Fingerprint: "sha256:report"}}, func(id string, offset, length int64) (error, []byte) {
				if id != sessionFileID || offset != 6 || length != 5 {
					t.Fatalf("session file read = (%q, %d, %d)", id, offset, length)
				}
				return nil, []byte("world")
			}, nil, nil),
			NewProjectModule(&ProjectInfo{CreatedAt: "2026-01-01T00:00:00.000Z"}, nil, &ProjectNotes{Notes: []ProjectNote{{ID: "guide", Title: "Guide", AuthorID: "author", AuthorName: &authorName, CreatedAt: "2026-01-01T00:00:00.000Z", Revision: 1}}, Read: func(id string, offset, length int64) (error, []byte, bool) {
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
	sessionFileID := "sfi_00000000000000000000000000"
	projectFileID := "pfi_00000000000000000000000000"
	err, result := lisp.Evaluate(`(list
  (session/files/info "sfi_00000000000000000000000000")
  (session/files/info "sfi_00000000000000000000000001")
  (project/files/info "pfi_00000000000000000000000000")
  (project/files/info "pfi_00000000000000000000000001"))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule([]File{{ID: sessionFileID, Name: "session.txt", MediaType: &mediaType, Size: 12, Fingerprint: "sha256:session"}}, nil, nil, nil),
			NewProjectModule(nil, &ProjectFiles{Files: []File{{ID: projectFileID, Name: "project.txt", Size: 34, Fingerprint: "sha256:project"}}}, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != `(((id . "sfi_00000000000000000000000000") (name . "session.txt") (media_type . "text/plain") (size . 12) (fingerprint . "sha256:session")) null ((id . "pfi_00000000000000000000000000") (name . "project.txt") (media_type) (size . 34) (fingerprint . "sha256:project")) null)` {
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
	err, _ := lisp.Evaluate(`(session/files/read "sfi_00000000000000000000000000" 0 65537)`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, func(string, int64, int64) (error, []byte) { return nil, nil }, nil, nil), NewPolicyModule(nil), NewWebModule()},
	})
	if err == nil || !strings.Contains(err.Error(), "length from 1 through 65536") {
		t.Fatalf("Evaluate() oversized read error = %v", err)
	}

	err, result := lisp.Evaluate(`(error/value (error/catch (session/files/read "sfi_00000000000000000000000000" 0 1)))`, lisp.EvalOptions{
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

func TestGatehouseFileAPIsRejectWrongFileKinds(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (error/value (error/catch (session/files/info "pfi_00000000000000000000000000")))
  (error/value (error/catch (project/files/info "sfi_00000000000000000000000000")))
  (error/value (error/catch (session/files/read "not-a-file-id" 0 1)))
  (error/value (error/catch (project/files/read "not-a-file-id" 0 1))))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewSessionModule(nil, func(string, int64, int64) (error, []byte) {
				t.Fatal("session file read called with an invalid ID")
				return nil, nil
			}, nil, nil),
			NewProjectModule(nil, &ProjectFiles{Read: func(string, int64, int64) (error, []byte) {
				t.Fatal("project file read called with an invalid ID")
				return nil, nil
			}}, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || result.String() != `("not a session file" "not a project file" "not a session file" "not a project file")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
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
