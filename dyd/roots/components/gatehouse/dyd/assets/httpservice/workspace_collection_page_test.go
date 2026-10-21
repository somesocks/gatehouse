package httpservice

import (
	"encoding/json"
	"gatehouse/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWorkspaceCollectionPagesKeepNativeBodiesAndScope(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(w, r)
		return w
	}
	create := func(path, body string) string {
		w := request(http.MethodPost, path, body)
		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusCreated || result.ID == "" {
			t.Fatalf("create %s: %d %s (%v)", path, w.Code, w.Body, err)
		}
		return result.ID
	}
	workspace := "/api/v1/workspaces/" + refs["engineering"].Id
	project := create(workspace+"/projects", `{"name":"Collections"}`)
	other := create(workspace+"/projects", `{"name":"Other owner"}`)
	session := create(workspace+"/sessions", `{"project":"`+project+`"}`)
	for _, owner := range []string{workspace + "/projects/" + project, workspace + "/sessions/" + session} {
		for _, resource := range []struct{ path, body, other string }{
			{"notes", `{"title":"needle-alpha","body":"body must not appear in a directory"}`, `{"title":"needle-beta"}`},
			{"tasks", `{"title":"needle-alpha","status":"ready"}`, `{"title":"needle-beta","status":"draft"}`},
			{"secrets", `{"description":"needle-alpha","value":"write-only-value"}`, `{"description":"needle-beta","value":"another-write-only-value"}`},
		} {
			if resource.path == "secrets" && owner == workspace+"/projects/"+project {
				continue
			}
			t.Run(owner+"/"+resource.path, func(t *testing.T) {
				base := owner + "/" + resource.path
				create(base, resource.body)
				create(base, resource.other)
				legacy := request(http.MethodGet, base, "")
				var all []map[string]any
				if err := json.Unmarshal(legacy.Body.Bytes(), &all); err != nil || len(all) != 2 || legacy.Code != 200 {
					t.Fatalf("legacy body: %d %s (%v)", legacy.Code, legacy.Body, err)
				}
				first := request(http.MethodGet, base+"?limit=1&name=needle", "")
				var page []map[string]any
				if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 || len(page) != 1 || first.Header().Get("X-Next-Cursor") == "" {
					t.Fatalf("first page: %d %s (%v)", first.Code, first.Body, err)
				}
				id := page[0]["id"]
				second := request(http.MethodGet, base+"?limit=1&name=needle&cursor="+url.QueryEscape(first.Header().Get("X-Next-Cursor")), "")
				if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil || second.Code != 200 || len(page) != 1 || page[0]["id"] == id || second.Header().Get("X-Next-Cursor") != "" {
					t.Fatalf("second page: %d %s (%v)", second.Code, second.Body, err)
				}
				filtered := request(http.MethodGet, base+"?limit=1&name=needle-alpha", "")
				if err := json.Unmarshal(filtered.Body.Bytes(), &page); err != nil || filtered.Code != 200 || len(page) != 1 || filtered.Header().Get("X-Next-Cursor") != "" {
					t.Fatalf("filtered page: %d %s (%v)", filtered.Code, filtered.Body, err)
				}
				if strings.Contains(filtered.Body.String(), "write-only-value") || strings.Contains(filtered.Body.String(), "body must not appear") {
					t.Fatal("directory included content that is not in its native summary")
				}
				for _, query := range []string{"limit=101", "limit=1&cursor=invalid"} {
					if w := request(http.MethodGet, base+"?"+query, ""); w.Code != 400 {
						t.Fatalf("invalid page: %d %s", w.Code, w.Body)
					}
				}
			})
		}
	}
	otherNotes := workspace + "/projects/" + other + "/notes"
	create(otherNotes, `{"title":"needle-other-owner"}`)
	for _, path := range []string{workspace + "/projects/" + project + "/notes?limit=50&name=other-owner", workspace + "/sessions/" + session + "/notes?limit=50&name=other-owner"} {
		w := request(http.MethodGet, path, "")
		var rows []projectNoteResponse
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 0 || w.Code != 200 {
			t.Fatalf("owner scope: %d %s (%v)", w.Code, w.Body, err)
		}
	}
	note := create(otherNotes, `{"title":"100%_complete"}`)
	w := request(http.MethodGet, otherNotes+"?limit=50&name="+url.QueryEscape("%_"), "")
	var notes []projectNoteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &notes); err != nil || len(notes) != 1 || notes[0].ID != note {
		t.Fatalf("literal search: %d %s (%v)", w.Code, w.Body, err)
	}
	request(http.MethodPatch, otherNotes+"/"+note, `{"title":"Revision two"}`)
	history := request(http.MethodGet, otherNotes+"/"+note+"/revisions?limit=1", "")
	var revisions []noteRevisionResponse
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || len(revisions) != 1 || revisions[0].Revision != 2 || history.Header().Get("X-Next-Cursor") != "2" {
		t.Fatalf("history: %d %s (%v)", history.Code, history.Body, err)
	}
	history = request(http.MethodGet, otherNotes+"/"+note+"/revisions?limit=1&cursor=2", "")
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || len(revisions) != 1 || revisions[0].Revision != 1 {
		t.Fatalf("older revision: %d %s (%v)", history.Code, history.Body, err)
	}
	history = request(http.MethodGet, otherNotes+"/"+note+"/revisions?limit=1&name=missing", "")
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || history.Code != 200 || len(revisions) != 0 {
		t.Fatalf("no matching revision: %d %s (%v)", history.Code, history.Body, err)
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, otherNotes+"?limit=1", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated collection: %d", unauthorized.Code)
	}
}
