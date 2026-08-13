package httpservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
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
	handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/app/w/engineering/s/session-one", nil))
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
	tokens, store := testBearerTokens(t)
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
	if me.Code != http.StatusOK || me.Header().Get("Cache-Control") != "no-store" || me.Body.String() != "{\"principal\":\"alice\",\"identity\":\"gatehouse:alice\"}\n" {
		t.Fatalf("GET me = status %d cache %q body %q", me.Code, me.Header().Get("Cache-Control"), me.Body.String())
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
	tokens, store := testBearerTokens(t)
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identity":"gatehouse:alice","password":"wrong password"}`)))
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("POST login = status %d authenticate %q", response.Code, response.Header().Get("WWW-Authenticate"))
	}
}

func TestHandlerListsAuthorizedWorkspaceCatalog(t *testing.T) {
	tokens, store := testBearerTokens(t)
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	err, token := tokens.Mint(context.Background(), auth.Claims{Principal: "alice", Identity: "gatehouse:alice"})
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
	if workspaces.Code != http.StatusOK || workspaces.Body.String() != "[{\"id\":\"engineering\",\"name\":\"Engineering\"},{\"id\":\"operations\"}]\n" {
		t.Fatalf("GET workspaces = status %d body %q", workspaces.Code, workspaces.Body.String())
	}
	tools := request("/api/v1/workspaces/engineering/tools")
	if tools.Code != http.StatusOK || tools.Body.String() != "[{\"id\":\"git\"}]\n" {
		t.Fatalf("GET tools = status %d body %q", tools.Code, tools.Body.String())
	}
	groups := request("/api/v1/workspaces/engineering/groups")
	if groups.Code != http.StatusOK || groups.Body.String() != "[{\"id\":\"developers\",\"name\":\"Developers\"}]\n" {
		t.Fatalf("GET groups = status %d body %q", groups.Code, groups.Body.String())
	}
	sessions := request("/api/v1/workspaces/engineering/sessions")
	if sessions.Code != http.StatusOK || sessions.Body.String() != "[{\"id\":\"shared\"},{\"id\":\"private\"}]\n" {
		t.Fatalf("GET sessions = status %d body %q", sessions.Code, sessions.Body.String())
	}
	resources := request("/api/v1/workspaces/engineering/resources")
	if resources.Code != http.StatusOK || resources.Body.String() != "[{\"id\":\"docs\",\"secret\":false},{\"id\":\"token\",\"secret\":true}]\n" {
		t.Fatalf("GET resources = status %d body %q", resources.Code, resources.Body.String())
	}
	if strings.Contains(resources.Body.String(), "file:") || strings.Contains(resources.Body.String(), "env:") || strings.Contains(resources.Body.String(), "TOP_SECRET") {
		t.Fatalf("GET resources disclosed a resource source: %q", resources.Body.String())
	}
	operations := request("/api/v1/workspaces/operations/resources")
	if operations.Code != http.StatusOK || operations.Body.String() != "[]\n" {
		t.Fatalf("GET ungranted workspace resources = status %d body %q", operations.Code, operations.Body.String())
	}
	private := request("/api/v1/workspaces/private/tools")
	if private.Code != http.StatusNotFound {
		t.Fatalf("GET inaccessible workspace tools = status %d, want %d", private.Code, http.StatusNotFound)
	}
}

func TestHandlerBootstrapsLogsInCreatesSessionAndSubmitsMessage(t *testing.T) {
	tokens, store := testBearerTokens(t)
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

	created := request(http.MethodPost, "/api/v1/workspaces/engineering/sessions", "{}")
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

	message := request(http.MethodPost, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/messages", `{"text":"hello"}`)
	if message.Code != http.StatusAccepted || message.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST message = status %d cache %q", message.Code, message.Header().Get("Cache-Control"))
	}
	var event model.SessionEvent
	if err := json.Unmarshal(message.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Ref.Id == "" || event.Ref.Session.Id != session.ID || event.Kind != "message.text" || event.AuthorPrincipal == nil || event.AuthorPrincipal.Id != "alice" || event.AuthorAgent != nil || event.AuthorGateway != nil || event.Payload["text"] != "hello" || event.CreatedAt == "" {
		t.Fatalf("POST message response = %#v", event)
	}
	poll := request(http.MethodGet, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/events?limit=1", "")
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
	child := model.SessionEvent{
		Ref: model.SessionEventRef{Session: event.Ref.Session, Id: "1c76ece6-ef42-46e9-927f-b05084e5a997"}, Parent: &event.Ref, Kind: "message.reasoning", AuthorPrincipal: event.AuthorPrincipal, Payload: map[string]interface{}{"text": "working"},
	}
	if err, _ := store.SessionEventsCreate(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	poll = request(http.MethodGet, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/events?limit=1", "")
	if err := json.Unmarshal(poll.Body.Bytes(), &polled); err != nil || len(polled) != 1 || len(polled[0].Children) != 1 || polled[0].Children[0].Event.Ref != child.Ref {
		t.Fatalf("GET nested events response = (%#v, %v)", polled, err)
	}
	after := request(http.MethodGet, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/events?after_created_at="+event.CreatedAt+"&after_id="+event.Ref.Id, "")
	if after.Code != http.StatusOK || after.Body.String() != "[]\n" {
		t.Fatalf("GET events after cursor = status %d body %q", after.Code, after.Body.String())
	}
	invalidCursor := request(http.MethodGet, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/events?after_id="+event.Ref.Id, "")
	if invalidCursor.Code != http.StatusBadRequest {
		t.Fatalf("GET events with incomplete cursor = status %d", invalidCursor.Code)
	}
	invalidID := request(http.MethodGet, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/events?after_created_at="+event.CreatedAt+"&after_id=invalid", "")
	if invalidID.Code != http.StatusBadRequest {
		t.Fatalf("GET events with invalid cursor ID = status %d", invalidID.Code)
	}

	err, events := store.SessionEventsGet(context.Background(), model.SessionRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ref != event.Ref || events[0].AuthorPrincipal == nil || events[0].AuthorPrincipal.Id != "alice" {
		t.Fatalf("stored session events = %#v", events)
	}
	err, tasks := store.SessionEventReplyTasksGet(context.Background(), 10)
	if err != nil || len(tasks) != 1 || tasks[0].Event != event.Ref {
		t.Fatalf("stored session reply tasks = (%#v, %v)", tasks, err)
	}
	cancel := request(http.MethodPost, "/api/v1/workspaces/engineering/sessions/"+session.ID+"/messages/"+event.Ref.Id+"/cancel", "")
	if cancel.Code != http.StatusAccepted || cancel.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("POST cancel = status %d cache %q", cancel.Code, cancel.Header().Get("Cache-Control"))
	}
	var cancellation model.SessionEvent
	if err := json.Unmarshal(cancel.Body.Bytes(), &cancellation); err != nil {
		t.Fatal(err)
	}
	if cancellation.Ref.Id == "" || cancellation.Parent == nil || *cancellation.Parent != event.Ref || cancellation.Kind != "cancel.request" || cancellation.AuthorPrincipal == nil || cancellation.AuthorPrincipal.Id != "alice" || cancellation.AuthorAgent != nil || cancellation.AuthorGateway != nil || len(cancellation.Payload) != 0 {
		t.Fatalf("POST cancel response = %#v", cancellation)
	}
	err, tasks = store.SessionEventReplyTasksGet(context.Background(), 10)
	if err != nil || len(tasks) != 1 || tasks[0].Event != event.Ref {
		t.Fatalf("cancelled session reply tasks = (%#v, %v)", tasks, err)
	}
}

func TestWorkspaceAgentsAndMessageAgentPreference(t *testing.T) {
	tokens, store := testBearerTokens(t)
	label := "Assistant"
	state := config.State{
		Workspaces:     []config.Workspace{{ID: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{ID: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{ID: "assistant", Revision: 1, Provider: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello"}`, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Model: "assistant", Label: &label, Priority: 1, MaxTurns: 1, Enabled: true}},
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
	agents := request(http.MethodGet, "/api/v1/workspaces/engineering/agents", "")
	if agents.Code != http.StatusOK || agents.Body.String() != `[{"id":"assistant","label":"Assistant"}]`+"\n" {
		t.Fatalf("GET agents = status %d body %q", agents.Code, agents.Body.String())
	}
	session := request(http.MethodPost, "/api/v1/workspaces/engineering/sessions", "{}")
	var created sessionResponse
	if err := json.Unmarshal(session.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	message := request(http.MethodPost, "/api/v1/workspaces/engineering/sessions/"+created.ID+"/messages", `{"text":"hello","agent":"assistant"}`)
	var event model.SessionEvent
	if err := json.Unmarshal(message.Body.Bytes(), &event); err != nil || message.Code != http.StatusAccepted || event.Payload["agent"] != "assistant" {
		t.Fatalf("POST message = (%d, %#v, %v)", message.Code, event, err)
	}
	invalid := request(http.MethodPost, "/api/v1/workspaces/engineering/sessions/"+created.ID+"/messages", `{"text":"hello","agent":"missing"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("POST message with missing agent = %d", invalid.Code)
	}
}

func testBearerTokens(t *testing.T) (*auth.BearerTokens, *database.Store) {
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
	engineering := "Engineering"
	private := "Private"
	developers := "Developers"
	state := config.State{
		Keychains: configured,
		Workspaces: []config.Workspace{
			{ID: "engineering", Name: &engineering, Enabled: true},
			{ID: "operations", Enabled: true},
			{ID: "private", Name: &private, Enabled: true},
		},
		Principals: []config.Principal{{
			ID: "alice", Enabled: true, Identities: []config.Identity{{
				ID: "gatehouse:alice", Revision: 1, Enabled: true,
				Verifiers: []config.Verifier{{Algorithm: &algorithm, Sources: []config.PasswordSource{"env:GATEHOUSE_TEST_PASSWORD"}}},
			}},
		}},
		Groups: []config.Group{
			{
				WorkspaceID: "engineering", ID: "developers", Name: &developers, Enabled: true,
				Members:        []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
				ToolGrants:     []config.GroupToolGrant{{ToolID: "git", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{{ResourceID: "docs", Enabled: true}, {ResourceID: "token", Enabled: true}},
			},
			{
				WorkspaceID: "operations", ID: "operators", Enabled: true,
				Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
			},
			{WorkspaceID: "private", ID: "owners", Enabled: true},
		},
		Tools: []config.Tool{{WorkspaceID: "engineering", ID: "git", Source: "file:./git.lisp", Enabled: true}},
		Resources: []config.Resource{
			{WorkspaceID: "engineering", ID: "docs", Source: "file:./docs", Secret: false, Enabled: true},
			{WorkspaceID: "engineering", ID: "token", Source: "env:TOP_SECRET", Secret: true, Enabled: true},
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
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES
			('engineering', 'private', 'alice', TRUE, '2026-01-01 00:00:01'),
			('engineering', 'shared', 'alice', TRUE, '2026-01-01 00:00:02');
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled)
			VALUES ('engineering', 'private', 'alice', TRUE);
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
			VALUES ('engineering', 'shared', 'developers', TRUE);
	`); err != nil {
		t.Fatal(err)
	}
	err, tokens := auth.Prepare(ctx, store, keyring, "test")
	if err != nil {
		t.Fatal(err)
	}
	return tokens, store
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
