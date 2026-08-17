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
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

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
	tools := request("/api/v1/workspaces/" + engineering.Id + "/tools")
	var toolResponse []struct{ ID string }
	if err := json.Unmarshal(tools.Body.Bytes(), &toolResponse); err != nil || tools.Code != http.StatusOK || len(toolResponse) != 1 || !typed_id.Valid(typed_id.Tool, toolResponse[0].ID) {
		t.Fatalf("GET tools = status %d body %q", tools.Code, tools.Body.String())
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
	resources := request("/api/v1/workspaces/" + engineering.Id + "/resources")
	var resourceResponse []struct {
		ID     string
		Secret bool
	}
	if err := json.Unmarshal(resources.Body.Bytes(), &resourceResponse); err != nil || resources.Code != http.StatusOK || len(resourceResponse) != 2 || !typed_id.Valid(typed_id.Resource, resourceResponse[0].ID) || !typed_id.Valid(typed_id.Resource, resourceResponse[1].ID) || resourceResponse[0].Secret == resourceResponse[1].Secret {
		t.Fatalf("GET resources = status %d body %q", resources.Code, resources.Body.String())
	}
	if strings.Contains(resources.Body.String(), "file:") || strings.Contains(resources.Body.String(), "env:") || strings.Contains(resources.Body.String(), "TOP_SECRET") {
		t.Fatalf("GET resources disclosed a resource source: %q", resources.Body.String())
	}
	operationResources := request("/api/v1/workspaces/" + operations.Id + "/resources")
	if operationResources.Code != http.StatusOK || operationResources.Body.String() != "[]\n" {
		t.Fatalf("GET ungranted workspace resources = status %d body %q", operationResources.Code, operationResources.Body.String())
	}
	private := request("/api/v1/workspaces/private/tools")
	if private.Code != http.StatusNotFound {
		t.Fatalf("GET inaccessible workspace tools = status %d, want %d", private.Code, http.StatusNotFound)
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
	after := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?after_id="+event.Ref.Id, "")
	if after.Code != http.StatusOK || after.Body.String() != "[]\n" {
		t.Fatalf("GET events after cursor = status %d body %q", after.Code, after.Body.String())
	}
	invalidCursor := request(http.MethodGet, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/events?after_id=invalid", "")
	if invalidCursor.Code != http.StatusBadRequest {
		t.Fatalf("GET events with invalid cursor ID = status %d", invalidCursor.Code)
	}

	err, events := store.SessionEventsGet(context.Background(), model.SessionRef{Workspace: engineering, Id: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ref != event.Ref || events[0].AuthorPrincipal == nil || events[0].AuthorPrincipal.Ref != principal || events[0].AuthorPrincipal.Name == nil || *events[0].AuthorPrincipal.Name != "Alice" {
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
	message := request(http.MethodPost, "/api/v1/workspaces/"+engineering.Id+"/sessions/"+session.ID+"/messages", `{"files":["`+uploaded.File.Ref.Id+`"]}`)
	if message.Code != http.StatusAccepted {
		t.Fatalf("POST attachment message = status %d body %q", message.Code, message.Body.String())
	}
	var attachmentMessage model.SessionEvent
	if err := json.Unmarshal(message.Body.Bytes(), &attachmentMessage); err != nil {
		t.Fatal(err)
	}
	files, ok := attachmentMessage.Payload["files"].([]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("attachment message files = %#v", attachmentMessage.Payload["files"])
	}
	snapshot, ok := files[0].(map[string]interface{})
	if !ok || snapshot["id"] != uploaded.File.Ref.Id || snapshot["name"] != "report.txt" || snapshot["media_type"] != "text/plain" || snapshot["size"] != float64(len("hello storage")) || snapshot["fingerprint"] == "" {
		t.Fatalf("attachment message snapshot = %#v", files[0])
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
	listed := request(http.MethodGet, base, "")
	var files []projectFileResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &files); err != nil || listed.Code != http.StatusOK || len(files) != 1 || files[0].ID != completed.ID || files[0].Name != completed.Name || files[0].MediaType == nil || completed.MediaType == nil || *files[0].MediaType != *completed.MediaType || files[0].Size == nil || completed.Size == nil || *files[0].Size != *completed.Size || files[0].Fingerprint == nil || completed.Fingerprint == nil || *files[0].Fingerprint != *completed.Fingerprint || files[0].CreatedAt != completed.CreatedAt {
		t.Fatalf("GET project files = (%d, %#v, %v)", listed.Code, files, err)
	}
	download := request(http.MethodGet, base+"/"+upload.File.ID+"/download", "")
	if download.Code != http.StatusOK || download.Header().Get("Cache-Control") != "no-store" || download.Header().Get("Content-Disposition") != "attachment; filename=design.html" || download.Header().Get("Content-Type") != "application/octet-stream" || download.Header().Get("X-Content-Type-Options") != "nosniff" || download.Body.String() != "project storage" {
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
	if err := json.Unmarshal(created.Body.Bytes(), &note); err != nil || created.Code != http.StatusCreated || !typed_id.Valid(typed_id.ProjectNote, note.ID) || note.Title != "Guide" || note.Description != "How to work on this project." || note.Body == nil || *note.Body != "# Guide\n\nFollow the checklist." || note.Author.ID == "" || note.CreatedAt == "" {
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
	if err := json.Unmarshal(updated.Body.Bytes(), &note); err != nil || updated.Code != http.StatusOK || note.Title != "Guide" || note.Description != "" || note.Body == nil || *note.Body != "" {
		t.Fatalf("PATCH project note = (%d, %#v, %v)", updated.Code, note, err)
	}
	empty := request(http.MethodPost, base, `{"title":"Empty"}`)
	if err := json.Unmarshal(empty.Body.Bytes(), &note); err != nil || empty.Code != http.StatusCreated || note.Title != "Empty" || note.Description != "" || note.Body == nil || *note.Body != "" {
		t.Fatalf("POST empty project note = (%d, %#v, %v)", empty.Code, note, err)
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

func TestWorkspaceAgentsAndMessageAgentPreference(t *testing.T) {
	tokens, store, refs := testBearerTokens(t)
	label := "Assistant"
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello"}`, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "assistant", Label: &label, Priority: 1, MaxTurns: 1, Enabled: true}},
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
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'assistant'`).Scan(&assistantID); err != nil {
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
	if agents.Code != http.StatusOK || agents.Body.String() != `[{"id":"`+assistantID+`","label":"Assistant"}]`+"\n" {
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
		httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspace.Id+"/activity", bytes.NewReader(body))
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	if response := request(model.ActivityTopicCheckpoints{}); response.Code != http.StatusBadRequest {
		t.Fatalf("POST activity without topics = %d", response.Code)
	}
	empty := request(map[string]any{"topics": []map[string]any{{"topic": "session/ses_00000000000000000000000001", "cursor": nil}}})
	var emptyCheckpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyCheckpoints); err != nil || empty.Code != http.StatusOK || len(emptyCheckpoints.Topics) != 1 || emptyCheckpoints.Topics[0].Cursor != nil {
		t.Fatalf("POST activity with null cursor = (%d, %#v, %v)", empty.Code, emptyCheckpoints, err)
	}
	response := request(model.ActivityTopicCheckpoints{Topics: []model.ActivityTopicCheckpoint{
		{Topic: "session/" + session.Id},
	}})
	if response.Code != http.StatusOK {
		t.Fatalf("POST activity = %d body %q", response.Code, response.Body.String())
	}
	var checkpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(response.Body.Bytes(), &checkpoints); err != nil {
		t.Fatal(err)
	}
	if len(checkpoints.Topics) != 1 || checkpoints.Topics[0].Cursor == nil {
		t.Fatalf("POST activity = %#v, want an advanced topic checkpoint", checkpoints)
	}
	repeated := request(model.ActivityTopicCheckpoints{Topics: []model.ActivityTopicCheckpoint{
		{Topic: checkpoints.Topics[0].Topic, Cursor: checkpoints.Topics[0].Cursor},
	}})
	var repeatedCheckpoints model.ActivityTopicCheckpoints
	if err := json.Unmarshal(repeated.Body.Bytes(), &repeatedCheckpoints); err != nil || repeated.Code != http.StatusOK || !reflect.DeepEqual(repeatedCheckpoints, checkpoints) {
		t.Fatalf("POST activity with current checkpoints = (%d, %#v, %v), want %#v", repeated.Code, repeatedCheckpoints, err, checkpoints)
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
		}},
		Groups: []config.Group{
			{
				WorkspaceID: "engineering", Alias: "developers", Name: &developers, Enabled: true,
				Members:        []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
				ToolGrants:     []config.GroupToolGrant{{ToolAlias: "git", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{{ResourceAlias: "docs", Enabled: true}, {ResourceAlias: "token", Enabled: true}},
			},
			{
				WorkspaceID: "operations", Alias: "operators", Enabled: true,
				Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
			},
			{WorkspaceID: "private", Alias: "owners", Enabled: true},
		},
		Tools: []config.Tool{{WorkspaceID: "engineering", Alias: "git", Source: "file:./git.lisp", Enabled: true}},
		Resources: []config.Resource{
			{WorkspaceID: "engineering", Alias: "docs", Source: "file:./docs", Secret: false, Enabled: true},
			{WorkspaceID: "engineering", Alias: "token", Source: "env:TOP_SECRET", Secret: true, Enabled: true},
		},
		StorageProviders: []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{
			{WorkspaceID: "engineering", ProviderAlias: "embedded", Priority: 1, Enabled: true},
			{WorkspaceID: "operations", ProviderAlias: "embedded", Priority: 1, Enabled: true},
			{WorkspaceID: "private", ProviderAlias: "embedded", Priority: 1, Enabled: true},
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
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled)
		VALUES (?, 'ses_00000000000000000000000000', ?, TRUE)
	`, workspaces["engineering"].Id, principal.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
		VALUES (?, 'ses_00000000000000000000000001', ?, TRUE)
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
