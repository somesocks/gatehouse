package httpservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestSessionEventTranscriptProject(t *testing.T) {
	entries := []database.SessionEventTreeEntry{
		{Event: model.SessionEvent{Kind: "message.text", Payload: map[string]interface{}{"text": "reply", "attachments": []string{"sfi_00000000000000000000000000"}, "agent": "assistant", "reasoning": "hidden"}}},
		{Event: model.SessionEvent{Kind: "tool.request", Payload: map[string]interface{}{"name": "lisp", "reason": "Inspect source.", "code": "(read-all)"}}},
		{Event: model.SessionEvent{Kind: "approval.request", Payload: map[string]interface{}{"description": "Allow access?", "scope": "hidden"}}},
		{Event: model.SessionEvent{Kind: "tool.success", Payload: map[string]interface{}{"output": "hidden"}}},
	}

	sessionEventTranscriptProject(entries)

	for index, want := range []map[string]interface{}{
		{"text": "reply", "attachments": []string{"sfi_00000000000000000000000000"}, "agent": "assistant"},
		{"name": "lisp", "reason": "Inspect source."},
		{"description": "Allow access?"},
		{},
	} {
		if got := entries[index].Event.Payload; !reflect.DeepEqual(got, want) {
			t.Errorf("entry %d payload = %#v, want %#v", index, got, want)
		}
	}
}

func TestHandlerEnablesConfiguredRouteGroups(t *testing.T) {
	handler := Handler(config.HTTPService{Web: true, API: true}, nil)
	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/healthz", body: "ok\n"},
		{path: "/readyz", body: "ok\n"},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, http.StatusOK)
			}
			if response.Body.String() != test.body {
				t.Fatalf("GET %s body = %q, want %q", test.path, response.Body.String(), test.body)
			}
		})
	}
	redirect := httptest.NewRecorder()
	handler.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/app", nil))
	if redirect.Code != http.StatusTemporaryRedirect || redirect.Header().Get("Location") != "/app/" {
		t.Fatalf("GET /app = status %d location %q", redirect.Code, redirect.Header().Get("Location"))
	}
	app := httptest.NewRecorder()
	handler.ServeHTTP(app, httptest.NewRequest(http.MethodGet, "/app/", nil))
	if app.Code != http.StatusOK || !strings.Contains(app.Header().Get("Content-Type"), "text/html") || !strings.Contains(app.Body.String(), `<div id="app"></div>`) {
		t.Fatalf("GET /app/ = status %d content type %q body %q", app.Code, app.Header().Get("Content-Type"), app.Body.String())
	}
	route := httptest.NewRecorder()
	handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/app/wsp/engineering/ses/ses_00000000000000000000000000", nil))
	if route.Code != http.StatusOK || route.Body.String() != app.Body.String() {
		t.Fatalf("GET client route = status %d body %q", route.Code, route.Body.String())
	}
	entries, err := fs.ReadDir(webFiles, "assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("embedded web assets = (%#v, %v)", entries, err)
	}
	embeddedAsset := httptest.NewRecorder()
	handler.ServeHTTP(embeddedAsset, httptest.NewRequest(http.MethodGet, "/app/assets/"+entries[0].Name(), nil))
	if embeddedAsset.Code != http.StatusOK || embeddedAsset.Body.Len() == 0 {
		t.Fatalf("GET embedded asset = status %d body length %d", embeddedAsset.Code, embeddedAsset.Body.Len())
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/app/assets/missing.js", nil))
	if asset.Code != http.StatusNotFound {
		t.Fatalf("GET missing asset = status %d", asset.Code)
	}
	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusTemporaryRedirect || root.Header().Get("Location") != "/app/" {
		t.Fatalf("GET / = status %d location %q", root.Code, root.Header().Get("Location"))
	}
	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("GET unknown route = status %d, want %d", unknown.Code, http.StatusNotFound)
	}
}

func TestHandlerServesHealthWithoutRouteGroups(t *testing.T) {
	handler := Handler(config.HTTPService{}, nil)
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
				t.Fatalf("GET %s = status %d body %q, want status %d body %q", path, response.Code, response.Body.String(), http.StatusOK, "ok\n")
			}
		})
	}
}

func TestHandlerDisablesWebRouteGroup(t *testing.T) {
	response := httptest.NewRecorder()
	Handler(config.HTTPService{}, nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET / status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestHandlerAuthenticatesVersionedAPIRoutes(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	principal, identityID := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if unauthenticated.Code != http.StatusUnauthorized || unauthenticated.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("GET me without credentials = status %d authenticate %q", unauthenticated.Code, unauthenticated.Header().Get("WWW-Authenticate"))
	}

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identity":"alice","password":"correct password"}`)))
	if login.Code != http.StatusOK || login.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST login = status %d cache %q", login.Code, login.Header().Get("Cache-Control"))
	}
	var body loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TokenType != "Bearer" || body.AccessToken == "" {
		t.Fatalf("POST login response = %#v", body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "gatehouse_auth" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/api" {
		t.Fatalf("POST login cookie = %#v", cookies)
	}

	me := httptest.NewRecorder()
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(cookies[0])
	handler.ServeHTTP(me, meRequest)
	var claims auth.Claims
	if err := json.Unmarshal(me.Body.Bytes(), &claims); err != nil || me.Code != http.StatusOK || me.Header().Get("Cache-Control") != "no-store" || claims.Principal.Ref != principal || claims.Principal.Name == nil || *claims.Principal.Name != "Alice" || claims.Identity != identityID {
		t.Fatalf("GET me = status %d cache %q claims %#v error %v", me.Code, me.Header().Get("Cache-Control"), claims, err)
	}

	qualified := httptest.NewRecorder()
	handler.ServeHTTP(qualified, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	if qualified.Code != http.StatusOK {
		t.Fatalf("POST login with qualified identity = status %d", qualified.Code)
	}

	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	logoutCookies := logoutResponse.Result().Cookies()
	if logoutResponse.Code != http.StatusNoContent || len(logoutCookies) != 1 || logoutCookies[0].Name != "gatehouse_auth" || logoutCookies[0].MaxAge >= 0 {
		t.Fatalf("POST logout = status %d cookies %#v", logoutResponse.Code, logoutCookies)
	}
}

func TestHandlerRejectsInvalidLogin(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identity":"gatehouse:alice","password":"wrong password"}`)))
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("POST login = status %d authenticate %q", response.Code, response.Header().Get("WWW-Authenticate"))
	}
}

func TestHandlerListsAuthorizedWorkspaceCatalog(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	operations := refs["operations"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	principal, identityID := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(context.Background(), auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	err, roles := store.WorkspaceRolesGet(context.Background(), engineering, principal)
	if err != nil || !reflect.DeepEqual(roles, []authz.Role{authz.Manager}) {
		t.Fatalf("WorkspaceRolesGet() = (%#v, %v), want (%#v, nil)", roles, err, []authz.Role{authz.Manager})
	}
	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodGet, path, nil)
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	workspaces := request("/api/v1/workspaces")
	if workspaces.Code != http.StatusOK || workspaces.Body.String() != "[{\"id\":\""+engineering.Id+"\",\"alias\":\"engineering\",\"name\":\"Engineering\"},{\"id\":\""+operations.Id+"\",\"alias\":\"operations\"}]\n" {
		t.Fatalf("GET workspaces = status %d body %q", workspaces.Code, workspaces.Body.String())
	}
	groups := request("/api/v1/workspaces/" + engineering.Id + "/groups")
	developersID := groupID(t, context.Background(), store, "engineering", "developers")
	if groups.Code != http.StatusOK || groups.Body.String() != "[{\"id\":\""+developersID+"\",\"name\":\"Developers\"}]\n" {
		t.Fatalf("GET groups = status %d body %q", groups.Code, groups.Body.String())
	}
	sessions := request("/api/v1/workspaces/" + engineering.Id + "/sessions")
	if sessions.Code != http.StatusOK || sessions.Body.String() != "{\"sessions\":[{\"id\":\"ses_00000000000000000000000001\",\"created_at\":\"2026-01-01 00:00:02\"},{\"id\":\"ses_00000000000000000000000000\",\"created_at\":\"2026-01-01 00:00:01\"}]}\n" {
		t.Fatalf("GET sessions = status %d body %q", sessions.Code, sessions.Body.String())
	}
	firstSessionPage := request("/api/v1/workspaces/" + engineering.Id + "/sessions?limit=1")
	if firstSessionPage.Code != http.StatusOK || firstSessionPage.Body.String() != "{\"sessions\":[{\"id\":\"ses_00000000000000000000000001\",\"created_at\":\"2026-01-01 00:00:02\"}],\"next_cursor\":\"ses_00000000000000000000000001\"}\n" {
		t.Fatalf("GET first session page = status %d body %q", firstSessionPage.Code, firstSessionPage.Body.String())
	}
	secondSessionPage := request("/api/v1/workspaces/" + engineering.Id + "/sessions?limit=1&cursor=ses_00000000000000000000000001")
	if secondSessionPage.Code != http.StatusOK || secondSessionPage.Body.String() != "{\"sessions\":[{\"id\":\"ses_00000000000000000000000000\",\"created_at\":\"2026-01-01 00:00:01\"}]}\n" {
		t.Fatalf("GET second session page = status %d body %q", secondSessionPage.Code, secondSessionPage.Body.String())
	}
	invalidSessionPage := request("/api/v1/workspaces/" + engineering.Id + "/sessions?limit=101")
	if invalidSessionPage.Code != http.StatusBadRequest {
		t.Fatalf("GET invalid session page = status %d", invalidSessionPage.Code)
	}
	for _, path := range []string{
		"/api/v1/workspaces/" + operations.Id + "/projects",
		"/api/v1/workspaces/" + operations.Id + "/sessions",
	} {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		if response.Code != http.StatusForbidden {
			t.Fatalf("POST %s as workspace member = status %d, want %d", path, response.Code, http.StatusForbidden)
		}
	}
}

func TestHandlerEnforcesProjectAndSessionRoles(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	principal, identityID := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(context.Background(), auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	createdProject := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Policy"}`)
	var project projectResponse
	if err := json.Unmarshal(createdProject.Body.Bytes(), &project); err != nil || createdProject.Code != http.StatusCreated {
		t.Fatalf("POST project = (%d, %#v, %v)", createdProject.Code, project, err)
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_project_grants
		SET role = 'member'
		WHERE workspace = ? AND project = ? AND principal = ?
	`, engineering.Id, project.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID+"/notes", `{"title":"Denied"}`); response.Code != http.StatusForbidden {
		t.Fatalf("POST project note as member = %d, want %d", response.Code, http.StatusForbidden)
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_project_grants
		SET role = 'contributor'
		WHERE workspace = ? AND project = ? AND principal = ?
	`, engineering.Id, project.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID+"/notes", `{"title":"Allowed"}`); response.Code != http.StatusCreated {
		t.Fatalf("POST project note as contributor = %d, want %d", response.Code, http.StatusCreated)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID, `{}`); response.Code != http.StatusForbidden {
		t.Fatalf("PATCH project as contributor = %d, want %d", response.Code, http.StatusForbidden)
	}

	createdSession := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", `{}`)
	var session sessionResponse
	if err := json.Unmarshal(createdSession.Body.Bytes(), &session); err != nil || createdSession.Code != http.StatusCreated {
		t.Fatalf("POST session = (%d, %#v, %v)", createdSession.Code, session, err)
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_session_grants
		SET role = 'member'
		WHERE workspace = ? AND session = ? AND principal = ?
	`, engineering.Id, session.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/notes", `{"title":"Denied"}`); response.Code != http.StatusForbidden {
		t.Fatalf("POST session note as member = %d, want %d", response.Code, http.StatusForbidden)
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_session_grants
		SET role = 'contributor'
		WHERE workspace = ? AND session = ? AND principal = ?
	`, engineering.Id, session.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/notes", `{"title":"Allowed"}`); response.Code != http.StatusCreated {
		t.Fatalf("POST session note as contributor = %d, want %d", response.Code, http.StatusCreated)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/project", `{"project":null}`); response.Code != http.StatusForbidden {
		t.Fatalf("PATCH session project as contributor = %d, want %d", response.Code, http.StatusForbidden)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID, `{"name":"Denied"}`); response.Code != http.StatusForbidden {
		t.Fatalf("PATCH session name as contributor = %d, want %d", response.Code, http.StatusForbidden)
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_session_grants
		SET role = 'manager'
		WHERE workspace = ? AND session = ? AND principal = ?
	`, engineering.Id, session.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	renamed := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID, `{"name":"  Incident  review  "}`)
	var renamedSession sessionResponse
	if err := json.Unmarshal(renamed.Body.Bytes(), &renamedSession); err != nil || renamed.Code != http.StatusOK || renamedSession.Name == nil || *renamedSession.Name != "Incident review" {
		t.Fatalf("PATCH session name = (%d, %#v, %v)", renamed.Code, renamedSession, err)
	}
	for _, body := range []string{`{}`, `{"name":"  "}`, `{"name":"Valid","unknown":true}`} {
		if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID, body); response.Code != http.StatusBadRequest {
			t.Fatalf("PATCH invalid session name %s = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_project_grants
		SET role = 'member'
		WHERE workspace = ? AND project = ? AND principal = ?
	`, engineering.Id, project.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/project", `{"project":"`+project.ID+`"}`); response.Code != http.StatusForbidden {
		t.Fatalf("PATCH session into project as project member = %d, want %d", response.Code, http.StatusForbidden)
	}
	if _, err := store.ExecContext(context.Background(), `
		UPDATE gatehouse_project_grants
		SET role = 'contributor'
		WHERE workspace = ? AND project = ? AND principal = ?
	`, engineering.Id, project.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/project", `{"project":"`+project.ID+`"}`); response.Code != http.StatusOK {
		t.Fatalf("PATCH session into project as project contributor = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestHandlerAllowsDirectProjectGrantWithoutWorkspaceGrant(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	principal, identityID := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(context.Background(), auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	created := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Direct grant"}`)
	var project projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil || created.Code != http.StatusCreated {
		t.Fatalf("POST project = (%d, %#v, %v)", created.Code, project, err)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_workspace_grants SET enabled = FALSE WHERE workspace = ?`, engineering.Id); err != nil {
		t.Fatal(err)
	}
	err, roles := store.WorkspaceRolesGet(context.Background(), engineering, principal)
	if err != nil || len(roles) != 0 {
		t.Fatalf("WorkspaceRolesGet() after revocation = (%#v, %v), want no roles", roles, err)
	}

	projects := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/projects", "")
	if projects.Code != http.StatusOK || !strings.Contains(projects.Body.String(), project.ID) {
		t.Fatalf("GET projects with direct grant = (%d, %q)", projects.Code, projects.Body.String())
	}
	if response := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID, ""); response.Code != http.StatusOK {
		t.Fatalf("GET project with direct grant = %d, want %d", response.Code, http.StatusOK)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID, `{"description":"Available directly"}`); response.Code != http.StatusOK {
		t.Fatalf("PATCH project with direct grant = %d, want %d", response.Code, http.StatusOK)
	}
	createdSession := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", `{"project":"`+project.ID+`"}`)
	var session sessionResponse
	if err := json.Unmarshal(createdSession.Body.Bytes(), &session); err != nil || createdSession.Code != http.StatusCreated {
		t.Fatalf("POST linked session with direct project grant = (%d, %#v, %v)", createdSession.Code, session, err)
	}
	err, sessionRoles := store.SessionRolesGet(context.Background(), model.SessionRef{Workspace: engineering, Id: session.ID}, principal)
	if err != nil || !reflect.DeepEqual(sessionRoles, []authz.Role{authz.Manager}) {
		t.Fatalf("SessionRolesGet() after linked creation = (%#v, %v), want manager", sessionRoles, err)
	}
	if response := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/project", `{"project":null}`); response.Code != http.StatusNotFound {
		t.Fatalf("PATCH linked session without workspace grant = %d, want %d", response.Code, http.StatusNotFound)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_projects SET enabled = FALSE WHERE workspace = ? AND id = ?`, engineering.Id, project.ID); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID, ""); response.Code != http.StatusNotFound {
		t.Fatalf("GET session with disabled project = %d, want %d", response.Code, http.StatusNotFound)
	}
	if response := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/notes", `{"title":"Unavailable"}`); response.Code != http.StatusNotFound {
		t.Fatalf("POST session note with disabled project = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestHandlerBootstrapsLogsInCreatesSessionAndSubmitsMessage(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	principal, _ := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("POST login = status %d body %q", login.Code, login.Body.String())
	}
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	if credentials.AccessToken == "" {
		t.Fatalf("POST login response = %#v", credentials)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	created := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", "{}")
	if created.Code != http.StatusCreated || created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST session = status %d cache %q", created.Code, created.Header().Get("Cache-Control"))
	}
	var session sessionResponse
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.ID == "" {
		t.Fatalf("POST session response = %#v", session)
	}

	message := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/messages", `{"text":"hello"}`)
	if message.Code != http.StatusAccepted || message.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST message = status %d cache %q", message.Code, message.Header().Get("Cache-Control"))
	}
	var event model.SessionEvent
	if err := json.Unmarshal(message.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Ref.Id == "" || event.Ref.Session.Id != session.ID || event.Kind != "message.text" || event.AuthorPrincipal == nil || event.AuthorPrincipal.Ref != principal || event.AuthorPrincipal.Name == nil || *event.AuthorPrincipal.Name != "Alice" || event.AuthorAgent != nil || event.AuthorGateway != nil || event.Payload["text"] != "hello" || event.CreatedAt == "" {
		t.Fatalf("POST message response = %#v", event)
	}
	poll := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?limit=1", "")
	if poll.Code != http.StatusOK || poll.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET events = status %d cache %q", poll.Code, poll.Header().Get("Cache-Control"))
	}
	var polled []sessionEventTreeResponse
	if err := json.Unmarshal(poll.Body.Bytes(), &polled); err != nil {
		t.Fatal(err)
	}
	if len(polled) != 1 || polled[0].Event.Ref != event.Ref || len(polled[0].Children) != 0 {
		t.Fatalf("GET events response = %#v", polled)
	}
	search := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events/search", `{"expression":"\"HEL\""}`)
	if search.Code != http.StatusOK || search.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST event search = status %d cache %q", search.Code, search.Header().Get("Cache-Control"))
	}
	var searched sessionEventSearchResponse
	if err := json.Unmarshal(search.Body.Bytes(), &searched); err != nil {
		t.Fatal(err)
	}
	if len(searched.Events) != 1 || searched.Events[0].ID != event.Ref.Id || searched.Events[0].Kind != "message.text" || searched.Events[0].Size != 5 || len(searched.Events[0].Matches) != 1 || searched.Events[0].Matches[0].Offset != 0 || searched.Events[0].Matches[0].Length != 3 {
		t.Fatalf("POST event search response = %#v", searched)
	}
	invalidSearch := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events/search", `{"expression":"(not \"hello\")"}`)
	if invalidSearch.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid event search = status %d body %q", invalidSearch.Code, invalidSearch.Body.String())
	}
	parentTimestamp, err := typed_id.Timestamp(typed_id.SessionEvent, event.Ref.Id)
	if err != nil {
		t.Fatal(err)
	}
	childID, err := typed_id.NewAt(typed_id.SessionEvent, parentTimestamp.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	child := model.SessionEvent{
		Ref: model.SessionEventRef{Session: event.Ref.Session, Id: childID}, Parent: &event.Ref, Kind: "message.reasoning", AuthorPrincipal: event.AuthorPrincipal, Payload: map[string]interface{}{"text": "working"},
	}
	if err, _ := store.SessionEventsCreate(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	poll = request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?limit=1", "")
	if err := json.Unmarshal(poll.Body.Bytes(), &polled); err != nil || len(polled) != 1 || len(polled[0].Children) != 1 || polled[0].Children[0].Event.Ref != child.Ref {
		t.Fatalf("GET nested events response = (%#v, %v)", polled, err)
	}
	toolID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: event.Ref.Session, Id: toolID}, Parent: &event.Ref, Kind: "tool.request", AuthorPrincipal: event.AuthorPrincipal, Payload: map[string]interface{}{"name": "lisp", "reason": "Inspect source.", "code": "(read-all)"}}
	if err, _ := store.SessionEventsCreate(context.Background(), tool); err != nil {
		t.Fatal(err)
	}
	resultID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	result := model.SessionEvent{Ref: model.SessionEventRef{Session: event.Ref.Session, Id: resultID}, Parent: &tool.Ref, Kind: "tool.success", AuthorPrincipal: event.AuthorPrincipal, Payload: map[string]interface{}{"output": strings.Repeat("large result", 1024)}}
	if err, _ := store.SessionEventsCreate(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	transcript := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?view=transcript", "")
	if transcript.Code != http.StatusOK {
		t.Fatalf("GET transcript events = status %d body %q", transcript.Code, transcript.Body.String())
	}
	if err := json.Unmarshal(transcript.Body.Bytes(), &polled); err != nil {
		t.Fatal(err)
	}
	var projectedTool, projectedResult *sessionEventTreeResponse
	for _, child := range polled[0].Children {
		if child.Event.Ref == tool.Ref {
			projectedTool = child
			if len(child.Children) == 1 {
				projectedResult = child.Children[0]
			}
		}
	}
	if projectedTool == nil || projectedTool.Event.Payload["name"] != "lisp" || projectedTool.Event.Payload["reason"] != "Inspect source." || projectedTool.Event.Payload["code"] != nil {
		t.Fatalf("GET transcript tool request = %#v", projectedTool)
	}
	if projectedResult == nil || len(projectedResult.Event.Payload) != 0 {
		t.Fatalf("GET transcript tool result = %#v", projectedResult)
	}
	after := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?after_id="+event.Ref.Id, "")
	if after.Code != http.StatusOK || after.Body.String() != "[]\n" {
		t.Fatalf("GET events after cursor = status %d body %q", after.Code, after.Body.String())
	}
	invalidView := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?view=invalid", "")
	if invalidView.Code != http.StatusBadRequest {
		t.Fatalf("GET events with invalid view = status %d", invalidView.Code)
	}
	invalidCursor := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?after_id=invalid", "")
	if invalidCursor.Code != http.StatusBadRequest {
		t.Fatalf("GET events with invalid cursor ID = status %d", invalidCursor.Code)
	}

	err, events := store.SessionEventsGet(context.Background(), model.SessionRef{Workspace: engineering, Id: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[0].Ref != event.Ref || events[0].AuthorPrincipal == nil || events[0].AuthorPrincipal.Ref != principal || events[0].AuthorPrincipal.Name == nil || *events[0].AuthorPrincipal.Name != "Alice" {
		t.Fatalf("stored session events = %#v", events)
	}
	err, tasks := store.SessionEventReplyTasksGet(context.Background(), 10)
	if err != nil || len(tasks) != 1 || tasks[0].Event != event.Ref {
		t.Fatalf("stored session reply tasks = (%#v, %v)", tasks, err)
	}
	cancel := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/messages/"+event.Ref.Id+"/cancel", "")
	if cancel.Code != http.StatusAccepted || cancel.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST cancel = status %d cache %q", cancel.Code, cancel.Header().Get("Cache-Control"))
	}
	var cancellation model.SessionEvent
	if err := json.Unmarshal(cancel.Body.Bytes(), &cancellation); err != nil {
		t.Fatal(err)
	}
	if cancellation.Ref.Id == "" || cancellation.Parent == nil || *cancellation.Parent != event.Ref || cancellation.Kind != "cancel.request" || cancellation.AuthorPrincipal == nil || cancellation.AuthorPrincipal.Ref != principal || cancellation.AuthorPrincipal.Name == nil || *cancellation.AuthorPrincipal.Name != "Alice" || cancellation.AuthorAgent != nil || cancellation.AuthorGateway != nil || len(cancellation.Payload) != 0 {
		t.Fatalf("POST cancel response = %#v", cancellation)
	}
	err, tasks = store.SessionEventReplyTasksGet(context.Background(), 10)
	if err != nil || len(tasks) != 1 || tasks[0].Event != event.Ref {
		t.Fatalf("cancelled session reply tasks = (%#v, %v)", tasks, err)
	}
}

func TestSessionApprovalResponse(t *testing.T) {
	ctx := context.Background()
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	alice, _ := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("POST login = %d", login.Code)
	}
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: engineering, Id: "ses_00000000000000000000000000"}
	alicePrincipal := model.Principal{Ref: alice, Enabled: true}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: engineering, Id: agentID}
	root := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{"text": "hello"}}
	if err, _ := store.SessionEventsCreate(ctx, root); err != nil {
		t.Fatal(err)
	}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &root.Ref, Kind: "tool.request", AuthorAgent: &agent, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, tool); err != nil {
		t.Fatal(err)
	}
	approval := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &tool.Ref, Kind: "approval.request", AuthorAgent: &agent, Payload: map[string]interface{}{}}
	if err, _ := store.SessionEventsCreate(ctx, approval); err != nil {
		t.Fatal(err)
	}
	request := func(decision string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.Id+"/approvals/"+approval.Ref.Id, strings.NewReader(`{"decision":"`+decision+`"}`))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	response := request("approved")
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST approval = %d body %q", response.Code, response.Body.String())
	}
	var decision model.SessionEvent
	if err := json.Unmarshal(response.Body.Bytes(), &decision); err != nil {
		t.Fatal(err)
	}
	if decision.Kind != "approval.approved" || decision.Parent == nil || *decision.Parent != approval.Ref || decision.AuthorPrincipal == nil || decision.AuthorPrincipal.Ref != alice {
		t.Fatalf("approval response = %#v", decision)
	}
	if duplicate := request("rejected"); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate approval = %d body %q", duplicate.Code, duplicate.Body.String())
	}
	err, tasks := store.SessionApprovalDecisionTasksGet(ctx, 10)
	if err != nil || len(tasks) != 1 || tasks[0].Approval != approval.Ref || tasks[0].Response != decision.Ref {
		t.Fatalf("SessionApprovalDecisionTasksGet() = (%#v, %v)", tasks, err)
	}
}

func TestSessionFileUploadFinishAndDownload(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	createdSession := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", "{}")
	var session sessionResponse
	if err := json.Unmarshal(createdSession.Body.Bytes(), &session); err != nil || session.ID == "" {
		t.Fatalf("POST session = (%d, %#v, %v)", createdSession.Code, session, err)
	}
	createdFile := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files", `{"name":"report.txt","media_type":"text/plain"}`)
	if createdFile.Code != http.StatusCreated {
		t.Fatalf("POST session file = status %d body %q", createdFile.Code, createdFile.Body.String())
	}
	var uploaded sessionFileCreateResponse
	if err := json.Unmarshal(createdFile.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.SessionFile, uploaded.File.Ref.Id) || !typed_id.Valid(typed_id.StorageObject, uploaded.File.StorageObject.Id) || uploaded.File.Name != "report.txt" || uploaded.File.MediaType == nil || *uploaded.File.MediaType != "text/plain" || uploaded.UploadURL == "" {
		t.Fatalf("POST session file response = %#v", uploaded)
	}
	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(http.MethodPut, uploaded.UploadURL, strings.NewReader("hello storage")))
	if put.Code != http.StatusNoContent {
		t.Fatalf("PUT storage = status %d body %q", put.Code, put.Body.String())
	}
	finished := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id+"/finish", "")
	if finished.Code != http.StatusOK {
		t.Fatalf("POST finish = status %d body %q", finished.Code, finished.Body.String())
	}
	updatedFile := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id, `{"name":"  final-report.txt  "}`)
	var updated model.SessionFile
	if err := json.Unmarshal(updatedFile.Body.Bytes(), &updated); err != nil || updatedFile.Code != http.StatusOK || updated.Ref != uploaded.File.Ref || updated.Name != "final-report.txt" || updated.StorageObject != uploaded.File.StorageObject || updated.MediaType == nil || *updated.MediaType != "text/plain" {
		t.Fatalf("PATCH session file = (%d, %#v, %v)", updatedFile.Code, updated, err)
	}
	if invalid := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id, `{"media_type":"text/plain"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH session file unknown field = %d", invalid.Code)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_session_grants SET role = 'member' WHERE workspace = ? AND session = ?`, engineering.Id, session.ID); err != nil {
		t.Fatal(err)
	}
	if denied := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id, `{"name":"denied.txt"}`); denied.Code != http.StatusForbidden {
		t.Fatalf("PATCH session file as member = %d", denied.Code)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_session_grants SET role = 'manager' WHERE workspace = ? AND session = ?`, engineering.Id, session.ID); err != nil {
		t.Fatal(err)
	}
	listed := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files", "")
	var files []database.SessionFileSummary
	if err := json.Unmarshal(listed.Body.Bytes(), &files); err != nil || listed.Code != http.StatusOK || len(files) != 1 || files[0].ID != uploaded.File.Ref.Id || files[0].Name != "final-report.txt" || files[0].MediaType == nil || *files[0].MediaType != "text/plain" || files[0].Size != int64(len("hello storage")) || files[0].Fingerprint == "" {
		t.Fatalf("GET session files = (%d, %#v, %v)", listed.Code, files, err)
	}
	message := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/messages", `{"attachments":["`+uploaded.File.Ref.Id+`"]}`)
	if message.Code != http.StatusAccepted {
		t.Fatalf("POST attachment message = status %d body %q", message.Code, message.Body.String())
	}
	var attachmentMessage model.SessionEvent
	if err := json.Unmarshal(message.Body.Bytes(), &attachmentMessage); err != nil {
		t.Fatal(err)
	}
	attachments, ok := attachmentMessage.Payload["attachments"].([]interface{})
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachment message attachments = %#v", attachmentMessage.Payload["attachments"])
	}
	reference, ok := attachments[0].(map[string]interface{})
	if !ok || reference["id"] != uploaded.File.Ref.Id || reference["name"] != "final-report.txt" || reference["media_type"] != "text/plain" || reference["size"] != float64(len("hello storage")) || reference["fingerprint"] == "" {
		t.Fatalf("attachment message reference = %#v", attachments[0])
	}
	err, persisted := store.SessionEventGet(context.Background(), attachmentMessage.Ref)
	if err != nil || persisted == nil {
		t.Fatalf("SessionEventGet() = (%#v, %v)", persisted, err)
	}
	ids, ok := persisted.Payload["attachments"].([]interface{})
	if !ok || len(ids) != 1 || ids[0] != uploaded.File.Ref.Id {
		t.Fatalf("stored attachment IDs = %#v", persisted.Payload["attachments"])
	}
	loadedEvents := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events", "")
	if loadedEvents.Code != http.StatusOK {
		t.Fatalf("GET session events = status %d body %q", loadedEvents.Code, loadedEvents.Body.String())
	}
	var trees []*sessionEventTreeResponse
	if err := json.Unmarshal(loadedEvents.Body.Bytes(), &trees); err != nil || len(trees) != 1 || trees[0].Event.Ref != attachmentMessage.Ref {
		t.Fatalf("GET session events = (%#v, %v)", trees, err)
	}
	attachments, ok = trees[0].Event.Payload["attachments"].([]interface{})
	if !ok || len(attachments) != 1 {
		t.Fatalf("loaded attachments = %#v", trees[0].Event.Payload["attachments"])
	}
	reference, ok = attachments[0].(map[string]interface{})
	if !ok || reference["id"] != uploaded.File.Ref.Id || reference["name"] != "final-report.txt" || reference["media_type"] != "text/plain" || reference["size"] != float64(len("hello storage")) || reference["fingerprint"] == "" {
		t.Fatalf("loaded attachment = %#v", attachments[0])
	}
	download := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id+"/download", "")
	if download.Code != http.StatusTemporaryRedirect || download.Header().Get("Cache-Control") != "no-store" || download.Header().Get("Location") == "" {
		t.Fatalf("GET download = status %d cache %q location %q", download.Code, download.Header().Get("Cache-Control"), download.Header().Get("Location"))
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, download.Header().Get("Location"), nil))
	if get.Code != http.StatusOK || get.Body.String() != "hello storage" {
		t.Fatalf("GET storage = status %d body %q", get.Code, get.Body.String())
	}
	secondPut := httptest.NewRecorder()
	handler.ServeHTTP(secondPut, httptest.NewRequest(http.MethodPut, uploaded.UploadURL, strings.NewReader("replacement")))
	if secondPut.Code != http.StatusConflict {
		t.Fatalf("second PUT storage = status %d", secondPut.Code)
	}
	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet, uploaded.UploadURL, nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET upload URL = status %d", wrongMethod.Code)
	}
	removed := request(http.MethodDelete, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE session file = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if duplicate := request(http.MethodDelete, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id, ""); duplicate.Code != http.StatusNotFound {
		t.Fatalf("DELETE removed session file = status %d body %q", duplicate.Code, duplicate.Body.String())
	}
	if unavailable := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/files/"+uploaded.File.Ref.Id+"/download", ""); unavailable.Code != http.StatusNotFound {
		t.Fatalf("GET removed session file = status %d body %q", unavailable.Code, unavailable.Body.String())
	}
}

func TestProjectFileUploadFinishListDownloadAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	createdProject := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Files"}`)
	var project projectResponse
	if err := json.Unmarshal(createdProject.Body.Bytes(), &project); err != nil || createdProject.Code != http.StatusCreated || !typed_id.Valid(typed_id.Project, project.ID) {
		t.Fatalf("POST project = (%d, %#v, %v)", createdProject.Code, project, err)
	}
	updatedProject := request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID, `{"name":"Design files","description":"Shared files for the design team."}`)
	if err := json.Unmarshal(updatedProject.Body.Bytes(), &project); err != nil || updatedProject.Code != http.StatusOK || project.Name == nil || *project.Name != "Design files" || project.Description == nil || *project.Description != "Shared files for the design team." {
		t.Fatalf("PATCH project = (%d, %#v, %v)", updatedProject.Code, project, err)
	}
	updatedProject = request(http.MethodPatch, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID, `{"name":"Updated design files"}`)
	if err := json.Unmarshal(updatedProject.Body.Bytes(), &project); err != nil || updatedProject.Code != http.StatusOK || project.Name == nil || *project.Name != "Updated design files" || project.Description == nil || *project.Description != "Shared files for the design team." {
		t.Fatalf("PATCH project name only = (%d, %#v, %v)", updatedProject.Code, project, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/projects/" + project.ID + "/files"
	started := request(http.MethodPost, base+"/start", `{"name":"design.html","media_type":"text/html"}`)
	var upload projectFileCreateResponse
	if err := json.Unmarshal(started.Body.Bytes(), &upload); err != nil || started.Code != http.StatusCreated {
		t.Fatalf("POST project file start = (%d, %#v, %v)", started.Code, upload, err)
	}
	if !typed_id.Valid(typed_id.ProjectFile, upload.File.ID) || upload.File.Name != "design.html" || upload.File.MediaType == nil || *upload.File.MediaType != "text/html" || upload.File.CreatedAt == "" || upload.File.Size != nil || upload.File.Fingerprint != nil || upload.UploadURL == "" {
		t.Fatalf("POST project file start response = %#v", upload)
	}
	pending := request(http.MethodGet, base, "")
	var pendingFiles []projectFileResponse
	if err := json.Unmarshal(pending.Body.Bytes(), &pendingFiles); err != nil || pending.Code != http.StatusOK || len(pendingFiles) != 0 {
		t.Fatalf("GET pending project files = (%d, %#v, %v)", pending.Code, pendingFiles, err)
	}
	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(http.MethodPut, upload.UploadURL, strings.NewReader("project storage")))
	if put.Code != http.StatusNoContent {
		t.Fatalf("PUT storage = status %d body %q", put.Code, put.Body.String())
	}
	finished := request(http.MethodPost, base+"/"+upload.File.ID+"/finish", "")
	var completed projectFileResponse
	if err := json.Unmarshal(finished.Body.Bytes(), &completed); err != nil || finished.Code != http.StatusOK || completed.ID != upload.File.ID || completed.Size == nil || *completed.Size != int64(len("project storage")) || completed.Fingerprint == nil || *completed.Fingerprint == "" {
		t.Fatalf("POST project file finish = (%d, %#v, %v)", finished.Code, completed, err)
	}
	updatedFile := request(http.MethodPatch, base+"/"+upload.File.ID, `{"name":"  final-design.html  "}`)
	if err := json.Unmarshal(updatedFile.Body.Bytes(), &completed); err != nil || updatedFile.Code != http.StatusOK || completed.ID != upload.File.ID || completed.Name != "final-design.html" || completed.MediaType == nil || *completed.MediaType != "text/html" || completed.Size == nil || *completed.Size != int64(len("project storage")) || completed.Fingerprint == nil || *completed.Fingerprint == "" {
		t.Fatalf("PATCH project file = (%d, %#v, %v)", updatedFile.Code, completed, err)
	}
	if invalid := request(http.MethodPatch, base+"/"+upload.File.ID, `{"name":""}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH project file blank name = %d", invalid.Code)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_project_grants SET role = 'member' WHERE workspace = ? AND project = ?`, engineering.Id, project.ID); err != nil {
		t.Fatal(err)
	}
	if denied := request(http.MethodPatch, base+"/"+upload.File.ID, `{"name":"denied.html"}`); denied.Code != http.StatusForbidden {
		t.Fatalf("PATCH project file as member = %d", denied.Code)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_project_grants SET role = 'manager' WHERE workspace = ? AND project = ?`, engineering.Id, project.ID); err != nil {
		t.Fatal(err)
	}
	listed := request(http.MethodGet, base, "")
	var files []projectFileResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &files); err != nil || listed.Code != http.StatusOK || len(files) != 1 || files[0].ID != completed.ID || files[0].Name != completed.Name || files[0].MediaType == nil || completed.MediaType == nil || *files[0].MediaType != *completed.MediaType || files[0].Size == nil || completed.Size == nil || *files[0].Size != *completed.Size || files[0].Fingerprint == nil || completed.Fingerprint == nil || *files[0].Fingerprint != *completed.Fingerprint || files[0].CreatedAt != completed.CreatedAt {
		t.Fatalf("GET project files = (%d, %#v, %v)", listed.Code, files, err)
	}
	download := request(http.MethodGet, base+"/"+upload.File.ID+"/download", "")
	if download.Code != http.StatusOK || download.Header().Get("Cache-Control") != "no-store" || download.Header().Get("Content-Disposition") != "attachment; filename=final-design.html" || download.Header().Get("Content-Type") != "application/octet-stream" || download.Header().Get("X-Content-Type-Options") != "nosniff" || download.Body.String() != "project storage" {
		t.Fatalf("GET project file download = status %d cache %q disposition %q content type %q nosniff %q body %q", download.Code, download.Header().Get("Cache-Control"), download.Header().Get("Content-Disposition"), download.Header().Get("Content-Type"), download.Header().Get("X-Content-Type-Options"), download.Body.String())
	}
	removed := request(http.MethodDelete, base+"/"+upload.File.ID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE project file = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	listed = request(http.MethodGet, base, "")
	if err := json.Unmarshal(listed.Body.Bytes(), &files); err != nil || listed.Code != http.StatusOK || len(files) != 0 {
		t.Fatalf("GET removed project files = (%d, %#v, %v)", listed.Code, files, err)
	}
}

func TestProjectNoteCreateUpdateListGetAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	createdProject := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Notes"}`)
	var project projectResponse
	if err := json.Unmarshal(createdProject.Body.Bytes(), &project); err != nil || createdProject.Code != http.StatusCreated || !typed_id.Valid(typed_id.Project, project.ID) {
		t.Fatalf("POST project = (%d, %#v, %v)", createdProject.Code, project, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/projects/" + project.ID + "/notes"
	created := request(http.MethodPost, base, `{"title":"Guide","description":"How to work on this project.","body":"# Guide\n\nFollow the checklist."}`)
	var note projectNoteResponse
	if err := json.Unmarshal(created.Body.Bytes(), &note); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectNote, note.ID) || note.Title != "Guide" || note.Description != "How to work on this project." || note.Body == nil || *note.Body != "# Guide\n\nFollow the checklist." || note.Sensitive || note.Author.Principal == nil || note.Author.Principal.ID == "" || note.CreatedAt == "" {
		t.Fatalf("POST project note = (%d, %#v, %v)", created.Code, note, err)
	}
	listed := request(http.MethodGet, base, "")
	var notes []projectNoteResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &notes); err != nil || listed.Code != http.StatusOK || len(notes) != 1 || notes[0].ID != note.ID || notes[0].Body != nil {
		t.Fatalf("GET project notes = (%d, %#v, %v)", listed.Code, notes, err)
	}
	detail := request(http.MethodGet, base+"/"+note.ID, "")
	if err := json.Unmarshal(detail.Body.Bytes(), &note); err != nil || detail.Code != http.StatusOK || note.Body == nil || *note.Body != "# Guide\n\nFollow the checklist." {
		t.Fatalf("GET project note = (%d, %#v, %v)", detail.Code, note, err)
	}
	updated := request(http.MethodPatch, base+"/"+note.ID, `{"description":"","body":""}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &note); err != nil || updated.Code != http.StatusOK || note.Revision != 2 || note.Title != "Guide" || note.Description != "" || note.Body == nil || *note.Body != "" {
		t.Fatalf("PATCH project note = (%d, %#v, %v)", updated.Code, note, err)
	}
	revisions := request(http.MethodGet, base+"/"+note.ID+"/revisions", "")
	var projectRevisions []noteRevisionResponse
	if err := json.Unmarshal(revisions.Body.Bytes(), &projectRevisions); err != nil || revisions.Code != http.StatusOK || len(projectRevisions) != 2 || projectRevisions[0].Revision != 2 || projectRevisions[0].Body != nil || projectRevisions[1].Revision != 1 {
		t.Fatalf("GET project note revisions = (%d, %#v, %v)", revisions.Code, projectRevisions, err)
	}
	firstRevision := request(http.MethodGet, base+"/"+note.ID+"/revisions/1", "")
	var projectRevision noteRevisionResponse
	if err := json.Unmarshal(firstRevision.Body.Bytes(), &projectRevision); err != nil || firstRevision.Code != http.StatusOK || projectRevision.Body == nil || *projectRevision.Body != "# Guide\n\nFollow the checklist." {
		t.Fatalf("GET project note revision = (%d, %#v, %v)", firstRevision.Code, projectRevision, err)
	}
	empty := request(http.MethodPost, base, `{"title":"Empty"}`)
	if err := json.Unmarshal(empty.Body.Bytes(), &note); err != nil || empty.Code != http.StatusCreated || note.Title != "Empty" || note.Description != "" || note.Body == nil || *note.Body != "" {
		t.Fatalf("POST empty project note = (%d, %#v, %v)", empty.Code, note, err)
	}
	sensitive := request(http.MethodPost, base, `{"title":"Credentials","body":"secret","sensitive":true}`)
	if err := json.Unmarshal(sensitive.Body.Bytes(), &note); err != nil || sensitive.Code != http.StatusCreated || !note.Sensitive {
		t.Fatalf("POST sensitive project note = (%d, %#v, %v)", sensitive.Code, note, err)
	}
	updated = request(http.MethodPatch, base+"/"+note.ID, `{"sensitive":false}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &note); err != nil || updated.Code != http.StatusOK || note.Sensitive || note.Revision != 2 {
		t.Fatalf("PATCH sensitive project note = (%d, %#v, %v)", updated.Code, note, err)
	}
	firstRevision = request(http.MethodGet, base+"/"+note.ID+"/revisions/1", "")
	if err := json.Unmarshal(firstRevision.Body.Bytes(), &projectRevision); err != nil || firstRevision.Code != http.StatusOK || !projectRevision.Sensitive {
		t.Fatalf("GET sensitive project note revision = (%d, %#v, %v)", firstRevision.Code, projectRevision, err)
	}
	invalid := request(http.MethodPost, base, `{"title":"Guide","description":"Too large","body":"`+strings.Repeat("x", 1024*1024+1)+`"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid project note = status %d", invalid.Code)
	}
	removed := request(http.MethodDelete, base+"/"+note.ID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE project note = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if get := request(http.MethodGet, base+"/"+note.ID, ""); get.Code != http.StatusNotFound {
		t.Fatalf("GET removed project note = status %d", get.Code)
	}
}

func TestProjectTaskCreateUpdateListGetAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	createdProject := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Tasks"}`)
	var project projectResponse
	if err := json.Unmarshal(createdProject.Body.Bytes(), &project); err != nil || createdProject.Code != http.StatusCreated || !typed_id.Valid(typed_id.Project, project.ID) {
		t.Fatalf("POST project = (%d, %#v, %v)", createdProject.Code, project, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/projects/" + project.ID + "/tasks"
	created := request(http.MethodPost, base, `{"title":"Guide","description":"# Guide\n\nFollow the checklist.","sensitive":true}`)
	var task taskResponse
	if err := json.Unmarshal(created.Body.Bytes(), &task); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectTask, task.ID) || task.Title != "Guide" || task.Description == nil || *task.Description != "# Guide\n\nFollow the checklist." || !task.Sensitive || task.Status != "draft" || task.Creator.Principal == nil || task.Creator.Principal.ID == "" || task.Updater.Principal == nil || task.CreatedAt == "" || task.UpdatedAt == "" {
		t.Fatalf("POST project task = (%d, %#v, %v)", created.Code, task, err)
	}
	createdID := task.ID
	second := request(http.MethodPost, base, `{"title":"Release","status":"ready"}`)
	var newer taskResponse
	if err := json.Unmarshal(second.Body.Bytes(), &newer); err != nil || second.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectTask, newer.ID) || newer.Status != "ready" {
		t.Fatalf("POST ready project task = (%d, %#v, %v)", second.Code, newer, err)
	}
	listed := request(http.MethodGet, base, "")
	var tasks []taskResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &tasks); err != nil || listed.Code != http.StatusOK || len(tasks) != 2 || tasks[0].ID != newer.ID || tasks[1].ID != createdID || tasks[1].Description != nil || strings.Contains(listed.Body.String(), "Follow the checklist.") {
		t.Fatalf("GET project tasks = (%d, %#v, %v)", listed.Code, tasks, err)
	}
	detail := request(http.MethodGet, base+"/"+createdID, "")
	if err := json.Unmarshal(detail.Body.Bytes(), &task); err != nil || detail.Code != http.StatusOK || task.Description == nil || *task.Description != "# Guide\n\nFollow the checklist." {
		t.Fatalf("GET project task = (%d, %#v, %v)", detail.Code, task, err)
	}
	updated := request(http.MethodPatch, base+"/"+createdID, `{"description":"Updated description","sensitive":false,"status":"in_progress"}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &task); err != nil || updated.Code != http.StatusOK || task.Title != "Guide" || task.Description == nil || *task.Description != "Updated description" || task.Sensitive || task.Status != "in_progress" || task.Updater.Principal == nil {
		t.Fatalf("PATCH project task = (%d, %#v, %v)", updated.Code, task, err)
	}
	cancelled := request(http.MethodPost, base, `{"title":"Cancelled","status":"cancelled"}`)
	if err := json.Unmarshal(cancelled.Body.Bytes(), &task); err != nil || cancelled.Code != http.StatusCreated || task.Status != "cancelled" {
		t.Fatalf("POST cancelled project task = (%d, %#v, %v)", cancelled.Code, task, err)
	}
	if invalid := request(http.MethodPost, base, `{"title":"Guide","status":"invalid"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid project task status = %d", invalid.Code)
	}
	if invalid := request(http.MethodPatch, base+"/"+createdID, `{"title":""}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH invalid project task title = %d", invalid.Code)
	}
	if invalid := request(http.MethodPatch, base+"/"+createdID, `{"body":"unsupported"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH project task body = %d", invalid.Code)
	}
	removed := request(http.MethodDelete, base+"/"+createdID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE project task = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if get := request(http.MethodGet, base+"/"+createdID, ""); get.Code != http.StatusNotFound {
		t.Fatalf("GET removed project task = status %d", get.Code)
	}
}

func TestProjectSecretCreateUpdateListGetAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	createdProject := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Secrets"}`)
	var project projectResponse
	if err := json.Unmarshal(createdProject.Body.Bytes(), &project); err != nil || createdProject.Code != http.StatusCreated || !typed_id.Valid(typed_id.Project, project.ID) {
		t.Fatalf("POST project = (%d, %#v, %v)", createdProject.Code, project, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/projects/" + project.ID + "/secrets"
	created := request(http.MethodPost, base, `{"description":"Deploy token","value":"secret value"}`)
	var secret projectSecretResponse
	if err := json.Unmarshal(created.Body.Bytes(), &secret); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectSecret, secret.ID) || secret.Description != "Deploy token" || secret.Author.ID == "" || secret.CreatedAt == "" || secret.UpdatedAt == "" || strings.Contains(created.Body.String(), "secret value") {
		t.Fatalf("POST project secret = (%d, %#v, %v)", created.Code, secret, err)
	}
	listed := request(http.MethodGet, base, "")
	var secrets []projectSecretResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &secrets); err != nil || listed.Code != http.StatusOK || len(secrets) != 1 || secrets[0].ID != secret.ID || strings.Contains(listed.Body.String(), "secret value") {
		t.Fatalf("GET project secrets = (%d, %#v, %v)", listed.Code, secrets, err)
	}
	detail := request(http.MethodGet, base+"/"+secret.ID, "")
	if err := json.Unmarshal(detail.Body.Bytes(), &secret); err != nil || detail.Code != http.StatusOK || secret.Description != "Deploy token" || strings.Contains(detail.Body.String(), "secret value") {
		t.Fatalf("GET project secret = (%d, %#v, %v)", detail.Code, secret, err)
	}
	updated := request(http.MethodPatch, base+"/"+secret.ID, `{"description":"Rotated deploy token","value":"replacement value"}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &secret); err != nil || updated.Code != http.StatusOK || secret.Description != "Rotated deploy token" || strings.Contains(updated.Body.String(), "replacement value") {
		t.Fatalf("PATCH project secret = (%d, %#v, %v)", updated.Code, secret, err)
	}
	principal, _ := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, stored := store.ProjectSecretGet(context.Background(), model.ProjectSecretRef{Project: model.ProjectRef{Workspace: engineering, Id: project.ID}, Id: secret.ID}, principal)
	if err != nil || stored == nil || !strings.HasPrefix(stored.Secret.Ciphertext, "gh-enc:") || strings.Contains(stored.Secret.Ciphertext, "replacement value") {
		t.Fatalf("stored project secret = (%#v, %v)", stored, err)
	}
	invalid := request(http.MethodPost, base, `{"description":""}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid project secret = status %d", invalid.Code)
	}
	removed := request(http.MethodDelete, base+"/"+secret.ID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE project secret = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if get := request(http.MethodGet, base+"/"+secret.ID, ""); get.Code != http.StatusNotFound {
		t.Fatalf("GET removed project secret = status %d", get.Code)
	}
}

func TestProjectRecordSchemaAttributeRecordAndValueHTTPAPI(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	createdProject := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/projects", `{"name":"Records"}`)
	var project projectResponse
	if err := json.Unmarshal(createdProject.Body.Bytes(), &project); err != nil || createdProject.Code != http.StatusCreated {
		t.Fatalf("POST project = (%d, %#v, %v)", createdProject.Code, project, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/projects/" + project.ID + "/record-schemas"
	createdSchema := request(http.MethodPost, base, `{"name":"contacts","label":"Contacts","description":"Customer contacts"}`)
	var schema projectRecordSchemaResponse
	if err := json.Unmarshal(createdSchema.Body.Bytes(), &schema); err != nil || createdSchema.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectRecordSchema, schema.ID) || schema.Name != "contacts" || schema.Author.Principal == nil {
		t.Fatalf("POST project record schema = (%d, %#v, %v)", createdSchema.Code, schema, err)
	}
	listedSchemas := request(http.MethodGet, base, "")
	var schemas []projectRecordSchemaResponse
	if err := json.Unmarshal(listedSchemas.Body.Bytes(), &schemas); err != nil || listedSchemas.Code != http.StatusOK || len(schemas) != 1 || schemas[0].ID != schema.ID {
		t.Fatalf("GET project record schemas = (%d, %#v, %v)", listedSchemas.Code, schemas, err)
	}
	if detail := request(http.MethodGet, base+"/"+schema.ID, ""); detail.Code != http.StatusOK {
		t.Fatalf("GET project record schema = %d", detail.Code)
	}
	attributesPath := base + "/" + schema.ID + "/attributes"
	createdAttribute := request(http.MethodPost, attributesPath, `{"name":"email","label":"Email","type":"text","cardinality":"one","uniqueness":"none","display":"primary"}`)
	var attribute projectRecordAttributeResponse
	if err := json.Unmarshal(createdAttribute.Body.Bytes(), &attribute); err != nil || createdAttribute.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectRecordAttribute, attribute.ID) || attribute.Name != "email" || attribute.DisplayOrder != 0 {
		t.Fatalf("POST project record attribute = (%d, %#v, %v)", createdAttribute.Code, attribute, err)
	}
	if updated := request(http.MethodPatch, attributesPath+"/"+attribute.ID, `{"description":"Primary email"}`); updated.Code != http.StatusOK {
		t.Fatalf("PATCH project record attribute = %d", updated.Code)
	}
	listedAttributes := request(http.MethodGet, attributesPath, "")
	var attributes []projectRecordAttributeResponse
	if err := json.Unmarshal(listedAttributes.Body.Bytes(), &attributes); err != nil || listedAttributes.Code != http.StatusOK || len(attributes) != 1 || attributes[0].ID != attribute.ID || attributes[0].Display != "primary" || attributes[0].DisplayOrder != 0 || !bytes.Contains(listedAttributes.Body.Bytes(), []byte(`"display_order":0`)) {
		t.Fatalf("GET project record attributes = (%d, %#v, %v)", listedAttributes.Code, attributes, err)
	}
	detail := request(http.MethodGet, attributesPath+"/"+attribute.ID, "")
	var detailedAttribute projectRecordAttributeResponse
	if err := json.Unmarshal(detail.Body.Bytes(), &detailedAttribute); err != nil || detail.Code != http.StatusOK || detailedAttribute.ID != attribute.ID || detailedAttribute.DisplayOrder != 0 || !bytes.Contains(detail.Body.Bytes(), []byte(`"display_order":0`)) {
		t.Fatalf("GET project record attribute = (%d, %#v, %v)", detail.Code, detailedAttribute, err)
	}

	principal, _ := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_project_grants SET role = 'contributor' WHERE workspace = ? AND project = ? AND principal = ?`, engineering.Id, project.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	if denied := request(http.MethodPost, base, `{"name":"denied","label":"Denied"}`); denied.Code != http.StatusForbidden {
		t.Fatalf("POST project record schema as contributor = %d, want %d", denied.Code, http.StatusForbidden)
	}
	recordsPath := base + "/" + schema.ID + "/records"
	createdRecord := request(http.MethodPost, recordsPath, `{"values":[{"attribute":"email","value":"ada@example.test","sensitive":true}]}`)
	var recordResult struct {
		Record projectRecordResponse        `json:"record"`
		Values []projectRecordValueResponse `json:"values"`
	}
	if err := json.Unmarshal(createdRecord.Body.Bytes(), &recordResult); err != nil || createdRecord.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectRecord, recordResult.Record.ID) || len(recordResult.Values) != 1 || !typed_id.Valid(typed_id.ProjectRecordValue, recordResult.Values[0].ID) || recordResult.Values[0].Attribute != attribute.ID || !recordResult.Values[0].Sensitive {
		t.Fatalf("POST project record = (%d, %#v, %v)", createdRecord.Code, recordResult, err)
	}
	if detail := request(http.MethodGet, recordsPath+"/"+recordResult.Record.ID, ""); detail.Code != http.StatusOK {
		t.Fatalf("GET project record = %d", detail.Code)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_project_grants SET role = 'manager' WHERE workspace = ? AND project = ? AND principal = ?`, engineering.Id, project.ID, principal.Id); err != nil {
		t.Fatal(err)
	}
	fileID, err := typed_id.New(typed_id.ProjectFile)
	if err != nil {
		t.Fatal(err)
	}
	objectID, err := typed_id.New(typed_id.StorageObject)
	if err != nil {
		t.Fatal(err)
	}
	file := model.ProjectFile{Ref: model.ProjectFileRef{Project: model.ProjectRef{Workspace: engineering, Id: project.ID}, Id: fileID}, Name: "contact.pdf", Enabled: true}
	if err, _, _ := store.ProjectFileCreate(context.Background(), file, objectID, principal); err != nil {
		t.Fatal(err)
	}
	if err := store.StorageObjectStoreIntegrity(context.Background(), objectID, make([]byte, 32), 0); err != nil {
		t.Fatal(err)
	}
	if err := store.StorageObjectMarkSuccess(context.Background(), objectID); err != nil {
		t.Fatal(err)
	}
	relatedAttribute := request(http.MethodPost, attributesPath, `{"name":"related","label":"Related","type":"record","target_schema":"`+schema.ID+`","cardinality":"one","uniqueness":"none","display":"primary","display_order":2}`)
	var related projectRecordAttributeResponse
	if err := json.Unmarshal(relatedAttribute.Body.Bytes(), &related); err != nil || relatedAttribute.Code != http.StatusCreated || related.DisplayOrder != 2 {
		t.Fatalf("POST related project record attribute = (%d, %#v, %v)", relatedAttribute.Code, related, err)
	}
	fileAttribute := request(http.MethodPost, attributesPath, `{"name":"attachment","label":"Attachment","type":"file","cardinality":"one","uniqueness":"none","display":"none"}`)
	var attachment projectRecordAttributeResponse
	if err := json.Unmarshal(fileAttribute.Body.Bytes(), &attachment); err != nil || fileAttribute.Code != http.StatusCreated || attachment.Type != "file" || attachment.TargetSchema != nil {
		t.Fatalf("POST project file record attribute = (%d, %#v, %v)", fileAttribute.Code, attachment, err)
	}
	listedAttributes = request(http.MethodGet, attributesPath, "")
	if err := json.Unmarshal(listedAttributes.Body.Bytes(), &attributes); err != nil || listedAttributes.Code != http.StatusOK || len(attributes) != 3 || attributes[0].ID != attachment.ID || attributes[0].DisplayOrder != 0 || attributes[1].ID != attribute.ID || attributes[1].DisplayOrder != 0 || attributes[2].ID != related.ID || attributes[2].DisplayOrder != 2 || !bytes.Contains(listedAttributes.Body.Bytes(), []byte(`"display_order":0`)) || !bytes.Contains(listedAttributes.Body.Bytes(), []byte(`"display_order":2`)) {
		t.Fatalf("GET ordered project record attributes = (%d, %#v, %v)", listedAttributes.Code, attributes, err)
	}
	updatedAttribute := request(http.MethodPatch, attributesPath+"/"+attribute.ID, `{"display_order":3}`)
	if err := json.Unmarshal(updatedAttribute.Body.Bytes(), &attribute); err != nil || updatedAttribute.Code != http.StatusOK || attribute.DisplayOrder != 3 {
		t.Fatalf("PATCH project record attribute display order = (%d, %#v, %v)", updatedAttribute.Code, attribute, err)
	}
	listedAttributes = request(http.MethodGet, attributesPath, "")
	if err := json.Unmarshal(listedAttributes.Body.Bytes(), &attributes); err != nil || listedAttributes.Code != http.StatusOK || len(attributes) != 3 || attributes[0].ID != attachment.ID || attributes[0].DisplayOrder != 0 || attributes[1].ID != related.ID || attributes[1].DisplayOrder != 2 || attributes[2].ID != attribute.ID || attributes[2].DisplayOrder != 3 || !bytes.Contains(listedAttributes.Body.Bytes(), []byte(`"display_order":3`)) {
		t.Fatalf("GET reordered project record attributes = (%d, %#v, %v)", listedAttributes.Code, attributes, err)
	}
	if invalid := request(http.MethodPost, attributesPath, `{"name":"invalid","label":"Invalid","type":"text","cardinality":"one","uniqueness":"none","display":"secondary","display_order":-1}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST project record attribute with negative display order = %d, want %d", invalid.Code, http.StatusBadRequest)
	}
	if invalid := request(http.MethodPatch, attributesPath+"/"+attribute.ID, `{"display_order":-1}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH project record attribute with negative display order = %d, want %d", invalid.Code, http.StatusBadRequest)
	}
	secondRecord := request(http.MethodPost, recordsPath, `{"values":[{"attribute":"related","value":"`+recordResult.Record.ID+`","sensitive":false},{"attribute":"attachment","value":"`+fileID+`","sensitive":false}]}`)
	if secondRecord.Code != http.StatusCreated {
		t.Fatalf("POST second project record = %d", secondRecord.Code)
	}
	var second struct {
		Record projectRecordResponse `json:"record"`
	}
	if err := json.Unmarshal(secondRecord.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	listedRecords := request(http.MethodGet, recordsPath+"?limit=1", "")
	var records projectRecordSearchResponse
	if err := json.Unmarshal(listedRecords.Body.Bytes(), &records); err != nil || listedRecords.Code != http.StatusOK || len(records.Records) != 1 || !typed_id.Valid(typed_id.ProjectRecord, records.NextCursor) || len(records.Records[0].Values) != 1 || records.Records[0].Values[0].Reference == nil || records.Records[0].Values[0].Reference.SchemaLabel != "Contacts" || len(records.Records[0].Values[0].Reference.PrimaryValues) != 1 || records.Records[0].Values[0].Reference.PrimaryValues[0].Value != "ada@example.test" {
		t.Fatalf("GET project records = (%d, %#v, %v)", listedRecords.Code, records, err)
	}
	secondValues := request(http.MethodGet, recordsPath+"/"+second.Record.ID+"/values", "")
	var relatedValues projectRecordValuesResponse
	if err := json.Unmarshal(secondValues.Body.Bytes(), &relatedValues); err != nil || secondValues.Code != http.StatusOK || len(relatedValues.Values) != 2 {
		t.Fatalf("GET project record values with reference display = (%d, %#v, %v)", secondValues.Code, relatedValues, err)
	}
	var relatedValue, fileValue *projectRecordValueResponse
	for index := range relatedValues.Values {
		value := &relatedValues.Values[index]
		if value.Attribute == related.ID {
			relatedValue = value
		}
		if value.Attribute == attachment.ID {
			fileValue = value
		}
	}
	if relatedValue == nil || relatedValue.Reference == nil || relatedValue.Reference.SchemaLabel != "Contacts" || len(relatedValue.Reference.PrimaryValues) != 1 || relatedValue.Reference.PrimaryValues[0].Value != "ada@example.test" || fileValue == nil || fileValue.File == nil || fileValue.File.Name != "contact.pdf" {
		t.Fatalf("GET project record values displays = (%#v, %#v)", relatedValue, fileValue)
	}
	if blocked := request(http.MethodDelete, "/api/v1/workspaces/"+engineering.Id+"/projects/"+project.ID+"/files/"+fileID, ""); blocked.Code != http.StatusConflict {
		t.Fatalf("DELETE referenced project file = %d, want %d", blocked.Code, http.StatusConflict)
	}
	incomingReferences := request(http.MethodGet, recordsPath+"/"+recordResult.Record.ID+"/references", "")
	var incoming projectRecordIncomingReferencesResponse
	if err := json.Unmarshal(incomingReferences.Body.Bytes(), &incoming); err != nil || incomingReferences.Code != http.StatusOK || len(incoming.Groups) != 1 || incoming.Groups[0].SourceSchema.ID != schema.ID || incoming.Groups[0].SourceAttribute.ID != related.ID || len(incoming.Groups[0].References) != 1 || incoming.Groups[0].References[0].ID != second.Record.ID || len(incoming.Groups[0].References[0].PrimaryValues) != 0 {
		t.Fatalf("GET project record incoming references = (%d, %#v, %v)", incomingReferences.Code, incoming, err)
	}

	valuesPath := recordsPath + "/" + recordResult.Record.ID + "/values"
	listedValues := request(http.MethodGet, valuesPath+"?limit=1", "")
	var values projectRecordValuesResponse
	if err := json.Unmarshal(listedValues.Body.Bytes(), &values); err != nil || listedValues.Code != http.StatusOK || len(values.Values) != 1 || !values.Values[0].Sensitive || values.Values[0].Value != "ada@example.test" || values.NextCursor != "" {
		t.Fatalf("GET project record values = (%d, %#v, %v)", listedValues.Code, values, err)
	}
	mutated := request(http.MethodPost, valuesPath+"/mutate", `{"update":[{"id":"`+recordResult.Values[0].ID+`","value":"ada.lovelace@example.test"}]}`)
	var mutation projectRecordValuesMutationResponse
	if err := json.Unmarshal(mutated.Body.Bytes(), &mutation); err != nil || mutated.Code != http.StatusOK || len(mutation.Removed) != 1 || mutation.Removed[0] != recordResult.Values[0].ID || len(mutation.Created) != 1 || mutation.Created[0].ID == recordResult.Values[0].ID || !mutation.Created[0].Sensitive || mutation.Created[0].Value != "ada.lovelace@example.test" {
		t.Fatalf("POST project record value mutation = (%d, %#v, %v)", mutated.Code, mutation, err)
	}
	if invalid := request(http.MethodPost, recordsPath, `{"values":[{"attribute":"missing","value":"x"}]}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST project record with missing attribute = %d, want %d", invalid.Code, http.StatusBadRequest)
	}
	if removed := request(http.MethodDelete, recordsPath+"/"+second.Record.ID, ""); removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE second project record = %d", removed.Code)
	}
	if removed := request(http.MethodDelete, recordsPath+"/"+recordResult.Record.ID, ""); removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE project record = (%d, %q)", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if removed := request(http.MethodDelete, attributesPath+"/"+attribute.ID, ""); removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE project record attribute = %d", removed.Code)
	}
	if removed := request(http.MethodDelete, attributesPath+"/"+related.ID, ""); removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE related project record attribute = %d", removed.Code)
	}
	if removed := request(http.MethodDelete, attributesPath+"/"+attachment.ID, ""); removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE project file record attribute = %d", removed.Code)
	}
	if updated := request(http.MethodPatch, base+"/"+schema.ID, `{"label":"People"}`); updated.Code != http.StatusOK {
		t.Fatalf("PATCH project record schema = %d", updated.Code)
	}
	if removed := request(http.MethodDelete, base+"/"+schema.ID, ""); removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE project record schema = %d", removed.Code)
	}
}

func TestSessionNoteCreateUpdateListGetAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	createdSession := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", "{}")
	var session sessionResponse
	if err := json.Unmarshal(createdSession.Body.Bytes(), &session); err != nil || createdSession.Code != http.StatusCreated || !typed_id.Valid(typed_id.Session, session.ID) {
		t.Fatalf("POST session = (%d, %#v, %v)", createdSession.Code, session, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/sessions/" + session.ID + "/notes"
	created := request(http.MethodPost, base, `{"title":"Guide","description":"How to work in this session.","body":"# Guide\n\nFollow the checklist."}`)
	var note sessionNoteResponse
	if err := json.Unmarshal(created.Body.Bytes(), &note); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.SessionNote, note.ID) || note.Title != "Guide" || note.Description != "How to work in this session." || note.Body == nil || *note.Body != "# Guide\n\nFollow the checklist." || note.Sensitive || note.Author.Principal == nil || note.Author.Principal.ID == "" || note.CreatedAt == "" {
		t.Fatalf("POST session note = (%d, %#v, %v)", created.Code, note, err)
	}
	listed := request(http.MethodGet, base, "")
	var notes []sessionNoteResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &notes); err != nil || listed.Code != http.StatusOK || len(notes) != 1 || notes[0].ID != note.ID || notes[0].Body != nil {
		t.Fatalf("GET session notes = (%d, %#v, %v)", listed.Code, notes, err)
	}
	detail := request(http.MethodGet, base+"/"+note.ID, "")
	if err := json.Unmarshal(detail.Body.Bytes(), &note); err != nil || detail.Code != http.StatusOK || note.Body == nil || *note.Body != "# Guide\n\nFollow the checklist." {
		t.Fatalf("GET session note = (%d, %#v, %v)", detail.Code, note, err)
	}
	updated := request(http.MethodPatch, base+"/"+note.ID, `{"description":"","body":""}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &note); err != nil || updated.Code != http.StatusOK || note.Revision != 2 || note.Title != "Guide" || note.Description != "" || note.Body == nil || *note.Body != "" {
		t.Fatalf("PATCH session note = (%d, %#v, %v)", updated.Code, note, err)
	}
	revisions := request(http.MethodGet, base+"/"+note.ID+"/revisions", "")
	var sessionRevisions []noteRevisionResponse
	if err := json.Unmarshal(revisions.Body.Bytes(), &sessionRevisions); err != nil || revisions.Code != http.StatusOK || len(sessionRevisions) != 2 || sessionRevisions[0].Revision != 2 || sessionRevisions[0].Body != nil || sessionRevisions[1].Revision != 1 {
		t.Fatalf("GET session note revisions = (%d, %#v, %v)", revisions.Code, sessionRevisions, err)
	}
	firstRevision := request(http.MethodGet, base+"/"+note.ID+"/revisions/1", "")
	var sessionRevision noteRevisionResponse
	if err := json.Unmarshal(firstRevision.Body.Bytes(), &sessionRevision); err != nil || firstRevision.Code != http.StatusOK || sessionRevision.Body == nil || *sessionRevision.Body != "# Guide\n\nFollow the checklist." {
		t.Fatalf("GET session note revision = (%d, %#v, %v)", firstRevision.Code, sessionRevision, err)
	}
	empty := request(http.MethodPost, base, `{"title":"Empty"}`)
	if err := json.Unmarshal(empty.Body.Bytes(), &note); err != nil || empty.Code != http.StatusCreated || note.Title != "Empty" || note.Description != "" || note.Body == nil || *note.Body != "" {
		t.Fatalf("POST empty session note = (%d, %#v, %v)", empty.Code, note, err)
	}
	sensitive := request(http.MethodPost, base, `{"title":"Credentials","body":"secret","sensitive":true}`)
	if err := json.Unmarshal(sensitive.Body.Bytes(), &note); err != nil || sensitive.Code != http.StatusCreated || !note.Sensitive {
		t.Fatalf("POST sensitive session note = (%d, %#v, %v)", sensitive.Code, note, err)
	}
	updated = request(http.MethodPatch, base+"/"+note.ID, `{"sensitive":false}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &note); err != nil || updated.Code != http.StatusOK || note.Sensitive || note.Revision != 2 {
		t.Fatalf("PATCH sensitive session note = (%d, %#v, %v)", updated.Code, note, err)
	}
	firstRevision = request(http.MethodGet, base+"/"+note.ID+"/revisions/1", "")
	if err := json.Unmarshal(firstRevision.Body.Bytes(), &sessionRevision); err != nil || firstRevision.Code != http.StatusOK || !sessionRevision.Sensitive {
		t.Fatalf("GET sensitive session note revision = (%d, %#v, %v)", firstRevision.Code, sessionRevision, err)
	}
	invalid := request(http.MethodPost, base, `{"title":"Guide","description":"Too large","body":"`+strings.Repeat("x", 1024*1024+1)+`"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid session note = status %d", invalid.Code)
	}
	removed := request(http.MethodDelete, base+"/"+note.ID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE session note = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if get := request(http.MethodGet, base+"/"+note.ID, ""); get.Code != http.StatusNotFound {
		t.Fatalf("GET removed session note = status %d", get.Code)
	}
}

func TestSessionTaskCreateUpdateListGetAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	createdSession := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", "{}")
	var session sessionResponse
	if err := json.Unmarshal(createdSession.Body.Bytes(), &session); err != nil || createdSession.Code != http.StatusCreated || !typed_id.Valid(typed_id.Session, session.ID) {
		t.Fatalf("POST session = (%d, %#v, %v)", createdSession.Code, session, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/sessions/" + session.ID + "/tasks"
	created := request(http.MethodPost, base, `{"title":"Guide","description":"# Guide\n\nFollow the checklist.","sensitive":true}`)
	var task taskResponse
	if err := json.Unmarshal(created.Body.Bytes(), &task); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.SessionTask, task.ID) || task.Title != "Guide" || task.Description == nil || *task.Description != "# Guide\n\nFollow the checklist." || !task.Sensitive || task.Status != "draft" || task.Creator.Principal == nil || task.Creator.Principal.ID == "" || task.Updater.Principal == nil || task.CreatedAt == "" || task.UpdatedAt == "" {
		t.Fatalf("POST session task = (%d, %#v, %v)", created.Code, task, err)
	}
	createdID := task.ID
	second := request(http.MethodPost, base, `{"title":"Release","status":"ready"}`)
	var newer taskResponse
	if err := json.Unmarshal(second.Body.Bytes(), &newer); err != nil || second.Code != http.StatusCreated || !typed_id.Valid(typed_id.SessionTask, newer.ID) || newer.Status != "ready" {
		t.Fatalf("POST ready session task = (%d, %#v, %v)", second.Code, newer, err)
	}
	listed := request(http.MethodGet, base, "")
	var tasks []taskResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &tasks); err != nil || listed.Code != http.StatusOK || len(tasks) != 2 || tasks[0].ID != newer.ID || tasks[1].ID != createdID || tasks[1].Description != nil || strings.Contains(listed.Body.String(), "Follow the checklist.") {
		t.Fatalf("GET session tasks = (%d, %#v, %v)", listed.Code, tasks, err)
	}
	detail := request(http.MethodGet, base+"/"+createdID, "")
	if err := json.Unmarshal(detail.Body.Bytes(), &task); err != nil || detail.Code != http.StatusOK || task.Description == nil || *task.Description != "# Guide\n\nFollow the checklist." {
		t.Fatalf("GET session task = (%d, %#v, %v)", detail.Code, task, err)
	}
	updated := request(http.MethodPatch, base+"/"+createdID, `{"description":"Updated description","sensitive":false,"status":"done"}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &task); err != nil || updated.Code != http.StatusOK || task.Title != "Guide" || task.Description == nil || *task.Description != "Updated description" || task.Sensitive || task.Status != "done" || task.Updater.Principal == nil {
		t.Fatalf("PATCH session task = (%d, %#v, %v)", updated.Code, task, err)
	}
	if invalid := request(http.MethodPost, base, `{"title":"Guide","status":"invalid"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid session task status = %d", invalid.Code)
	}
	if invalid := request(http.MethodPatch, base+"/"+createdID, `{"title":""}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH invalid session task title = %d", invalid.Code)
	}
	if invalid := request(http.MethodPatch, base+"/"+createdID, `{"body":"unsupported"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH session task body = %d", invalid.Code)
	}
	removed := request(http.MethodDelete, base+"/"+createdID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE session task = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if get := request(http.MethodGet, base+"/"+createdID, ""); get.Code != http.StatusNotFound {
		t.Fatalf("GET removed session task = status %d", get.Code)
	}
}

func TestSessionSecretCreateUpdateListGetAndRemove(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	engineering := refs["engineering"]
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("POST login = (%d, %#v, %v)", login.Code, credentials, err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	createdSession := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", "{}")
	var session sessionResponse
	if err := json.Unmarshal(createdSession.Body.Bytes(), &session); err != nil || createdSession.Code != http.StatusCreated {
		t.Fatalf("POST session = (%d, %#v, %v)", createdSession.Code, session, err)
	}
	base := "/api/v1/workspaces/" + engineering.Id + "/sessions/" + session.ID + "/secrets"
	created := request(http.MethodPost, base, `{"description":"Deploy token","value":"secret value"}`)
	var secret sessionSecretResponse
	if err := json.Unmarshal(created.Body.Bytes(), &secret); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.SessionSecret, secret.ID) || secret.Description != "Deploy token" || secret.Author.ID == "" || secret.CreatedAt == "" || secret.UpdatedAt == "" || strings.Contains(created.Body.String(), "secret value") {
		t.Fatalf("POST session secret = (%d, %#v, %v)", created.Code, secret, err)
	}
	listed := request(http.MethodGet, base, "")
	var secrets []sessionSecretResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &secrets); err != nil || listed.Code != http.StatusOK || len(secrets) != 1 || secrets[0].ID != secret.ID || strings.Contains(listed.Body.String(), "secret value") {
		t.Fatalf("GET session secrets = (%d, %#v, %v)", listed.Code, secrets, err)
	}
	detail := request(http.MethodGet, base+"/"+secret.ID, "")
	if err := json.Unmarshal(detail.Body.Bytes(), &secret); err != nil || detail.Code != http.StatusOK || secret.Description != "Deploy token" || strings.Contains(detail.Body.String(), "secret value") {
		t.Fatalf("GET session secret = (%d, %#v, %v)", detail.Code, secret, err)
	}
	updated := request(http.MethodPatch, base+"/"+secret.ID, `{"description":"Rotated deploy token","value":"replacement value"}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &secret); err != nil || updated.Code != http.StatusOK || secret.Description != "Rotated deploy token" || strings.Contains(updated.Body.String(), "replacement value") {
		t.Fatalf("PATCH session secret = (%d, %#v, %v)", updated.Code, secret, err)
	}
	principal, _ := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, stored := store.SessionSecretGet(context.Background(), model.SessionSecretRef{Session: model.SessionRef{Workspace: engineering, Id: session.ID}, Id: secret.ID}, principal)
	if err != nil || stored == nil || !strings.HasPrefix(stored.Secret.Ciphertext, "gh-enc:") || strings.Contains(stored.Secret.Ciphertext, "replacement value") {
		t.Fatalf("stored session secret = (%#v, %v)", stored, err)
	}
	invalid := request(http.MethodPost, base, `{"description":""}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid session secret = status %d", invalid.Code)
	}
	removed := request(http.MethodDelete, base+"/"+secret.ID, "")
	if removed.Code != http.StatusNoContent || removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE session secret = status %d cache %q", removed.Code, removed.Header().Get("Cache-Control"))
	}
	if get := request(http.MethodGet, base+"/"+secret.ID, ""); get.Code != http.StatusNotFound {
		t.Fatalf("GET removed session secret = status %d", get.Code)
	}
}

func TestWorkspaceAgentsAndMessageAgentPreference(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	label := "Assistant"
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 2, Label: &label, Default: true, Enabled: true}},
	}
	keyringErr, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if keyringErr != nil {
		t.Fatal(keyringErr)
	}
	defer keyring.Close()
	buildErr, set := migrations.Build(config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, state, keyring)
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if err := migrations.Run(context.Background(), store, set); err != nil {
		t.Fatal(err)
	}
	engineering := refs["engineering"]
	var assistantID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = ? AND alias = 'assistant'`, engineering.Id).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	agents := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/agents", "")
	if agents.Code != http.StatusOK || agents.Body.String() != `[{"id":"`+assistantID+`","alias":"assistant","label":"Assistant"}]`+"\n" {
		t.Fatalf("GET agents = status %d body %q", agents.Code, agents.Body.String())
	}
	session := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions", "{}")
	var created sessionResponse
	if err := json.Unmarshal(session.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	message := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+created.ID+"/messages", `{"text":"hello","agent":"`+assistantID+`"}`)
	var event model.SessionEvent
	if err := json.Unmarshal(message.Body.Bytes(), &event); err != nil || message.Code != http.StatusAccepted || event.Payload["agent"] != assistantID {
		t.Fatalf("POST message = (%d, %#v, %v)", message.Code, event, err)
	}
	alias := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+created.ID+"/messages", `{"text":"hello","agent":"assistant"}`)
	if alias.Code != http.StatusBadRequest {
		t.Fatalf("POST message with agent alias = %d", alias.Code)
	}
	invalid := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+created.ID+"/messages", `{"text":"hello","agent":"missing"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST message with missing agent = %d", invalid.Code)
	}
}

func TestActivityAPI(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	ctx := context.Background()
	workspace := refs["engineering"]
	alice, _ := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	aliceName := "Alice"
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000002"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionEventsCreate(ctx, model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{},
	}); err != nil {
		t.Fatal(err)
	}
	privateWorkspace := refs["private"]
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:               model.ActivityEventRef{},
		Event:             "workspace.update",
		ResourceKind:      database.ActivityResourceKindWorkspace,
		ResourceWorkspace: &privateWorkspace.Id,
	}, []string{privateWorkspace.Id}); err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	principal, identityID := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(value any) *httptest.ResponseRecorder {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/activity", bytes.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	if response := request(model.ActivityTopicCheckpoints{}); response.Code != http.StatusBadRequest {
		t.Fatalf("POST activity without topics = %d", response.Code)
	}
	empty := request(map[string]any{"topics": []map[string]any{{"topic": workspace.Id + "/ses_00000000000000000000000001", "events": []string{"session.*"}, "cursor": nil}}})
	var emptyCheckpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyCheckpoints); err != nil || empty.Code != http.StatusOK || len(emptyCheckpoints.Topics) != 1 || emptyCheckpoints.Topics[0].Cursor != nil {
		t.Fatalf("POST activity with null cursor = (%d, %#v, %v)", empty.Code, emptyCheckpoints, err)
	}
	denied := request(model.ActivityTopicCheckpoints{Topics: []model.ActivityTopicCheckpoint{
		{Topic: privateWorkspace.Id, Events: []string{"workspace.*"}},
	}})
	var deniedCheckpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(denied.Body.Bytes(), &deniedCheckpoints); err != nil || denied.Code != http.StatusOK || len(deniedCheckpoints.Topics) != 1 || deniedCheckpoints.Topics[0].Cursor != nil {
		t.Fatalf("POST activity for unavailable workspace = (%d, %#v, %v)", denied.Code, deniedCheckpoints, err)
	}
	name := "active-session"
	response := request(model.ActivityTopicCheckpoints{Topics: []model.ActivityTopicCheckpoint{
		{Name: &name, Topic: workspace.Id + "/" + session.Id, Events: []string{"session.*"}},
	}})
	if response.Code != http.StatusOK {
		t.Fatalf("POST activity = %d body %q", response.Code, response.Body.String())
	}
	var checkpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(response.Body.Bytes(), &checkpoints); err != nil {
		t.Fatal(err)
	}
	if len(checkpoints.Topics) != 1 || checkpoints.Topics[0].Name == nil || *checkpoints.Topics[0].Name != name || checkpoints.Topics[0].Cursor == nil || strings.Contains(strings.Join(checkpoints.Topics[0].Events, ","), "*") {
		t.Fatalf("POST activity = %#v, want an advanced topic checkpoint", checkpoints)
	}
	repeated := request(model.ActivityTopicCheckpoints{Topics: []model.ActivityTopicCheckpoint{
		{Name: checkpoints.Topics[0].Name, Topic: checkpoints.Topics[0].Topic, Events: checkpoints.Topics[0].Events, Cursor: checkpoints.Topics[0].Cursor},
	}})
	var repeatedCheckpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(repeated.Body.Bytes(), &repeatedCheckpoints); err != nil || repeated.Code != http.StatusOK || !reflect.DeepEqual(repeatedCheckpoints, checkpoints) {
		t.Fatalf("POST activity with current checkpoints = (%d, %#v, %v), want %#v", repeated.Code, repeatedCheckpoints, err, checkpoints)
	}
}

func TestSystemGrantAPI(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	alice, identityID := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	initial := request(http.MethodGet, "/api/v1/system/grants", "")
	var grants []model.SystemGrant
	if err := json.Unmarshal(initial.Body.Bytes(), &grants); err != nil || initial.Code != http.StatusOK || len(grants) != 1 || grants[0].Principal.Id != alice.Id || !grants[0].Enabled || grants[0].Revision != 1 {
		t.Fatalf("GET system grants = (%d, %#v, %v)", initial.Code, grants, err)
	}
	aliceGrant := grants[0]
	var bob string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	created := request(http.MethodPost, "/api/v1/system/grants", `{"principal":"`+bob+`"}`)
	var grant model.SystemGrant
	if err := json.Unmarshal(created.Body.Bytes(), &grant); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.SystemGrant, grant.Ref.Id) || grant.Principal.Id != bob || !grant.Enabled || grant.Revision != 1 {
		t.Fatalf("POST system grant = (%d, %#v, %v)", created.Code, grant, err)
	}
	if duplicate := request(http.MethodPost, "/api/v1/system/grants", `{"principal":"`+bob+`"}`); duplicate.Code != http.StatusConflict {
		t.Fatalf("POST duplicate system grant = %d", duplicate.Code)
	}
	missing, err := typed_id.New(typed_id.Principal)
	if err != nil {
		t.Fatal(err)
	}
	if unavailable := request(http.MethodPost, "/api/v1/system/grants", `{"principal":"`+missing+`"}`); unavailable.Code != http.StatusNotFound {
		t.Fatalf("POST unavailable system grant principal = %d", unavailable.Code)
	}
	updated := request(http.MethodPatch, "/api/v1/system/grants/"+grant.Ref.Id, `{"enabled":false}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &grant); err != nil || updated.Code != http.StatusOK || grant.Enabled || grant.Revision != 2 {
		t.Fatalf("PATCH system grant = (%d, %#v, %v)", updated.Code, grant, err)
	}
	activity := request(http.MethodPost, "/api/v1/activity", `{"topics":[{"name":"system-grant","topic":"sys/`+aliceGrant.Ref.Id+`","events":["system_grant.*"]}]}`)
	var checkpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(activity.Body.Bytes(), &checkpoints); err != nil || activity.Code != http.StatusOK || len(checkpoints.Topics) != 1 || checkpoints.Topics[0].Cursor == nil {
		t.Fatalf("POST system activity = (%d, %#v, %v)", activity.Code, checkpoints, err)
	}
	if denied := request(http.MethodPost, "/api/v1/system/grants", `{"principal":"invalid"}`); denied.Code != http.StatusBadRequest {
		t.Fatalf("POST invalid system grant = %d", denied.Code)
	}
	noManagers := request(http.MethodPatch, "/api/v1/system/grants/"+aliceGrant.Ref.Id, `{"enabled":false}`)
	if noManagers.Code != http.StatusOK {
		t.Fatalf("PATCH final system manager = %d", noManagers.Code)
	}
	if denied := request(http.MethodGet, "/api/v1/system/grants", ""); denied.Code != http.StatusForbidden {
		t.Fatalf("GET system grants without manager = %d", denied.Code)
	}
}

func TestSystemPrincipalAPI(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	alice, identityID := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}

	initial := request(http.MethodGet, "/api/v1/system/principals", "")
	var principals []database.SystemPrincipal
	if err := json.Unmarshal(initial.Body.Bytes(), &principals); err != nil || initial.Code != http.StatusOK {
		t.Fatalf("GET system principals = (%d, %#v, %v)", initial.Code, principals, err)
	}
	var alicePrincipal *database.SystemPrincipal
	for index := range principals {
		if principals[index].ID == alice.Id {
			alicePrincipal = &principals[index]
			break
		}
	}
	if alicePrincipal == nil || alicePrincipal.Name == nil || *alicePrincipal.Name != "Alice" || len(alicePrincipal.Identities) != 1 || alicePrincipal.Identities[0].Key != "gatehouse:alice" || !alicePrincipal.Identities[0].Enabled {
		t.Fatalf("GET system principals alice = %#v", alicePrincipal)
	}
	if strings.Contains(initial.Body.String(), "verifier") || strings.Contains(initial.Body.String(), "correct password") {
		t.Fatalf("GET system principals exposed credentials: %s", initial.Body.String())
	}

	var bob string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	item := request(http.MethodGet, "/api/v1/system/principals/"+bob, "")
	var principal database.SystemPrincipal
	if err := json.Unmarshal(item.Body.Bytes(), &principal); err != nil || item.Code != http.StatusOK || principal.ID != bob || len(principal.Identities) != 0 {
		t.Fatalf("GET system principal = (%d, %#v, %v)", item.Code, principal, err)
	}
	updated := request(http.MethodPatch, "/api/v1/system/principals/"+bob, `{"enabled":false}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &principal); err != nil || updated.Code != http.StatusOK || principal.Enabled || principal.Revision != 2 {
		t.Fatalf("PATCH system principal = (%d, %#v, %v)", updated.Code, principal, err)
	}
	unchanged := request(http.MethodPatch, "/api/v1/system/principals/"+bob, `{"enabled":false}`)
	if err := json.Unmarshal(unchanged.Body.Bytes(), &principal); err != nil || unchanged.Code != http.StatusOK || principal.Revision != 2 {
		t.Fatalf("PATCH unchanged system principal = (%d, %#v, %v)", unchanged.Code, principal, err)
	}
	var activities int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events AS events JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id WHERE events.event = 'principal.update' AND topics.topic = ?`, bob).Scan(&activities); err != nil || activities != 1 {
		t.Fatalf("principal update activity = (%d, %v)", activities, err)
	}
	if invalid := request(http.MethodPatch, "/api/v1/system/principals/invalid", `{"enabled":false}`); invalid.Code != http.StatusNotFound {
		t.Fatalf("PATCH invalid system principal = %d", invalid.Code)
	}
	if invalid := request(http.MethodPatch, "/api/v1/system/principals/"+bob, `{}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("PATCH invalid system principal update = %d", invalid.Code)
	}
	if missing, err := typed_id.New(typed_id.Principal); err != nil {
		t.Fatal(err)
	} else if response := request(http.MethodPatch, "/api/v1/system/principals/"+missing, `{"enabled":false}`); response.Code != http.StatusNotFound {
		t.Fatalf("PATCH missing system principal = %d", response.Code)
	}
	selfDisabled := request(http.MethodPatch, "/api/v1/system/principals/"+alice.Id, `{"enabled":false}`)
	if selfDisabled.Code != http.StatusOK {
		t.Fatalf("PATCH current system principal = %d", selfDisabled.Code)
	}
	if unauthenticated := request(http.MethodGet, "/api/v1/system/principals", ""); unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("GET system principals for disabled principal = %d", unauthenticated.Code)
	}
}

func TestSystemAdministrationAPI(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	configured := []config.Keychain{{ID: "test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"}}}
	err, ring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	handler := HandlerWithKeyring(config.HTTPService{API: true}, store, ring, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(method, path, strings.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	keychains := request(http.MethodGet, "/api/v1/system/keychains", "")
	if keychains.Code != http.StatusOK || strings.Contains(keychains.Body.String(), "passphrase") || !strings.Contains(keychains.Body.String(), `"id":"test"`) {
		t.Fatalf("GET system keychains = status %d body %q", keychains.Code, keychains.Body.String())
	}
	created := request(http.MethodPost, "/api/v1/system/agent-providers", `{"alias":"admin-openai","protocol":"openai-responses","base_url":"https://api.example.test/v1","keychain":"test","api_key":"secret-value","enabled":true}`)
	var provider database.SystemAgentProvider
	if err := json.Unmarshal(created.Body.Bytes(), &provider); err != nil || created.Code != http.StatusOK || provider.ID == "" || !provider.CredentialConfigured || strings.Contains(created.Body.String(), "secret-value") {
		t.Fatalf("POST system agent provider = (%d, %#v, %v, %q)", created.Code, provider, err, created.Body.String())
	}
	listed := request(http.MethodGet, "/api/v1/system/agent-providers", "")
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "secret-value") {
		t.Fatalf("GET system agent providers = status %d body %q", listed.Code, listed.Body.String())
	}
	stale := request(http.MethodPatch, "/api/v1/system/agent-providers/"+provider.ID, `{"alias":"admin-openai","protocol":"openai-responses","base_url":"https://api.example.test/v1","keychain":"test","enabled":false,"expected_revision":2}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("PATCH stale system agent provider = %d", stale.Code)
	}
	updated := request(http.MethodPatch, "/api/v1/system/agent-providers/"+provider.ID, `{"alias":"admin-openai","protocol":"openai-responses","base_url":"https://api.example.test/v1","keychain":"test","enabled":false,"expected_revision":1}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &provider); err != nil || updated.Code != http.StatusOK || provider.Revision != 2 || provider.Enabled || !provider.CredentialConfigured {
		t.Fatalf("PATCH system agent provider = (%d, %#v, %v)", updated.Code, provider, err)
	}
	model := request(http.MethodPost, "/api/v1/system/agent-models", `{"alias":"admin-assistant","provider":"`+provider.ID+`","model":"gpt-test","parameters":"{}","compaction":"{\"algorithm\":\"mcmtr\",\"history_bytes\":96,\"buffer_bytes\":16}","max_turns":3,"max_output_tokens":1000,"enabled":true}`)
	var agentModel database.SystemAgentModel
	if err := json.Unmarshal(model.Body.Bytes(), &agentModel); err != nil || model.Code != http.StatusOK || agentModel.ID == "" {
		t.Fatalf("POST system agent model = (%d, %#v, %v)", model.Code, agentModel, err)
	}
	binding := request(http.MethodPost, "/api/v1/system/workspace-agents/"+refs["engineering"].Id, `{"alias":"admin-assistant","model":"`+agentModel.ID+`","default":true,"enabled":true}`)
	var agent database.SystemWorkspaceAgent
	if err := json.Unmarshal(binding.Body.Bytes(), &agent); err != nil || binding.Code != http.StatusOK || agent.Alias != "admin-assistant" {
		t.Fatalf("POST system workspace agent = (%d, %#v, %v)", binding.Code, agent, err)
	}
	if renamed := request(http.MethodPatch, "/api/v1/system/workspace-agents/"+refs["engineering"].Id+"/"+agent.ID, `{"alias":"renamed","model":"`+agentModel.ID+`","default":false,"enabled":false,"expected_revision":1}`); renamed.Code != http.StatusBadRequest {
		t.Fatalf("PATCH system workspace agent alias = %d", renamed.Code)
	}
	if patched := request(http.MethodPatch, "/api/v1/system/workspace-agents/"+refs["engineering"].Id+"/"+agent.ID, `{"model":"`+agentModel.ID+`","default":false,"enabled":false,"expected_revision":1}`); patched.Code != http.StatusOK {
		t.Fatalf("PATCH system workspace agent = %d", patched.Code)
	}
	if unauthenticated := httptest.NewRecorder(); func() bool {
		handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/system/agent-providers", nil))
		return unauthenticated.Code == http.StatusUnauthorized
	}() == false {
		t.Fatalf("GET system agent providers unauthenticated = %d", unauthenticated.Code)
	}
}

func testBearerTokens(t *testing.T) (*auth.BearerTokens, *database.Store, map[string]model.WorkspaceRef) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	t.Setenv("GATEHOUSE_TEST_KEYCHAIN", "test passphrase")
	t.Setenv("GATEHOUSE_TEST_PASSWORD", "correct password")
	configured := []config.Keychain{{
		ID: "test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"},
	}}
	algorithm := "pbkdf2-hmac-sha256-v1"
	alice := "Alice"
	engineering := "Engineering"
	private := "Private"
	developers := "Developers"
	developersGroup := "developers"
	operatorsGroup := "operators"
	state := config.State{
		Keychains: configured,
		Workspaces: []config.Workspace{
			{Alias: "engineering", Name: &engineering, Enabled: true},
			{Alias: "operations", Enabled: true},
			{Alias: "private", Name: &private, Enabled: true},
		},
		Principals: []config.Principal{{
			Alias: "alice", Name: &alice, Enabled: true, Identities: []config.Identity{{
				Alias: "alice-gatehouse", Key: "gatehouse:alice", Revision: 1, Enabled: true,
				Verifiers: []config.Verifier{{Algorithm: &algorithm, Sources: []config.PasswordSource{"env:GATEHOUSE_TEST_PASSWORD"}}},
			}},
		}, {Alias: "bob", Enabled: true}},
		Groups: []config.Group{
			{
				WorkspaceID: "engineering", Alias: developersGroup, Name: &developers, Enabled: true,
				Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
			},
			{
				WorkspaceID: "operations", Alias: operatorsGroup, Enabled: true,
				Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
			},
			{WorkspaceID: "private", Alias: "owners", Enabled: true},
		},
		WorkspaceGrants: []config.WorkspaceGrant{
			{WorkspaceID: "engineering", Role: string(authz.Manager), GroupID: &developersGroup, Revision: 1, Enabled: true},
			{WorkspaceID: "operations", Role: string(authz.Member), GroupID: &operatorsGroup, Revision: 1, Enabled: true},
		},
		SystemGrants:     []config.SystemGrant{{PrincipalID: "alice", Revision: 1, Enabled: true}},
		AgentProviders:   []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:      []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents:  []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
		StorageProviders: []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{
			{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true},
			{WorkspaceID: "operations", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true},
			{WorkspaceID: "private", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true},
		},
	}
	err, keyring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(keyring.Close)
	err, set := migrations.Build(configuration, state, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}
	workspaces := map[string]model.WorkspaceRef{}
	for _, alias := range []string{"engineering", "operations", "private"} {
		err, workspace := store.WorkspaceRefGetByAlias(ctx, alias)
		if err != nil {
			t.Fatal(err)
		}
		if workspace == nil {
			t.Fatalf("workspace alias %q was not found", alias)
		}
		workspaces[alias] = *workspace
	}
	principal, _ := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	developersID := groupID(t, ctx, store, "engineering", "developers")
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES
			(?, 'ses_00000000000000000000000000', ?, TRUE, '2026-01-01 00:00:01'),
			(?, 'ses_00000000000000000000000001', ?, TRUE, '2026-01-01 00:00:02')
	`, workspaces["engineering"].Id, principal.Id, workspaces["engineering"].Id, principal.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled)
		VALUES ('sgr_00000000000000000000000000', ?, 'ses_00000000000000000000000000', 'manager', ?, NULL, TRUE)
	`, workspaces["engineering"].Id, principal.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled)
		VALUES ('sgr_00000000000000000000000001', ?, 'ses_00000000000000000000000001', 'manager', NULL, ?, TRUE)
	`, workspaces["engineering"].Id, developersID); err != nil {
		t.Fatal(err)
	}
	err, tokens := auth.Prepare(ctx, store, keyring, "test")
	if err != nil {
		t.Fatal(err)
	}
	return tokens, store, workspaces
}

func principalIdentityRefs(t *testing.T, ctx context.Context, store *database.Store, principalAlias, identityKey string) (model.PrincipalRef, string) {
	t.Helper()
	var principalID, identityID string
	if err := store.QueryRowContext(ctx, `
		SELECT principals.id, identities.id
		FROM gatehouse_principals AS principals
		JOIN gatehouse_identities AS identities ON identities.principal_id = principals.id
		WHERE principals.alias = ? AND identities.key = ?
	`, principalAlias, identityKey).Scan(&principalID, &identityID); err != nil {
		t.Fatal(err)
	}
	return model.PrincipalRef{Id: principalID}, identityID
}

func groupID(t *testing.T, ctx context.Context, store *database.Store, workspaceAlias, alias string) string {
	t.Helper()
	var id string
	if err := store.QueryRowContext(ctx, `
		SELECT groups.id
		FROM gatehouse_groups AS groups
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id
		WHERE workspaces.alias = ? AND groups.alias = ?
	`, workspaceAlias, alias).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestServiceStartsServesAndStops(t *testing.T) {
	err, service := Start(config.HTTPService{Listen: "127.0.0.1:0"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.Get("http://" + service.Address() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("GET health = status %d body %q", response.StatusCode, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-service.Done(); err != nil {
		t.Fatal(err)
	}
}
