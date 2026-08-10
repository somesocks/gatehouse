package httpservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/identity"
	"gatehouse/keychain"
	"gatehouse/migrations"
)

func TestHandlerEnablesConfiguredRouteGroups(t *testing.T) {
	handler := Handler(config.HTTPService{Web: true, API: true}, nil)
	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/healthz", body: "ok\n"},
		{path: "/readyz", body: "ok\n"},
		{path: "/", body: "<!doctype html><title>Gatehouse</title><h1>Gatehouse</h1>\n"},
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
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identity":"gatehouse:alice","password":"correct password"}`)))
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
	configured := []config.Keychain{{
		ID: "test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"},
	}}
	err, keyring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(keyring.Close)
	err, set := migrations.Build(configuration, config.State{Keychains: configured}, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_principals (id, enabled) VALUES ('alice', TRUE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_workspaces (id, name, enabled) VALUES
			('engineering', 'Engineering', TRUE),
			('operations', NULL, TRUE),
			('private', 'Private', TRUE);
		INSERT INTO gatehouse_groups (workspace_id, id, name, enabled) VALUES
			('engineering', 'developers', 'Developers', TRUE),
			('operations', 'operators', 'Operators', TRUE),
			('private', 'owners', 'Owners', TRUE);
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled) VALUES
			('engineering', 'developers', 'alice', TRUE),
			('operations', 'operators', 'alice', TRUE);
		INSERT INTO gatehouse_tools (workspace_id, id, source, enabled)
			VALUES ('engineering', 'git', 'file:./git.lisp', TRUE);
		INSERT INTO gatehouse_resources (workspace_id, id, source, secret, enabled) VALUES
			('engineering', 'docs', 'file:./docs', FALSE, TRUE),
			('engineering', 'token', 'env:TOP_SECRET', TRUE, TRUE);
		INSERT INTO gatehouse_group_tool_grants (workspace_id, group_id, tool_id, enabled)
			VALUES ('engineering', 'developers', 'git', TRUE);
		INSERT INTO gatehouse_group_resource_grants (workspace_id, group_id, resource_id, enabled) VALUES
			('engineering', 'developers', 'docs', TRUE),
			('engineering', 'developers', 'token', TRUE);
		INSERT INTO gatehouse_sessions (workspace, id, created_by, enabled, created_at) VALUES
			('engineering', 'private', 'alice', TRUE, '2026-01-01 00:00:01'),
			('engineering', 'shared', 'alice', TRUE, '2026-01-01 00:00:02');
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled)
			VALUES ('engineering', 'private', 'alice', TRUE);
		INSERT INTO gatehouse_session_group_grants (workspace, session, "group", enabled)
			VALUES ('engineering', 'shared', 'developers', TRUE);
	`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEHOUSE_TEST_PASSWORD", "correct password")
	algorithm := "pbkdf2-hmac-sha256-v1"
	if err := identity.Prepare(ctx, store, []config.Principal{{
		ID: "alice", Enabled: true, Identities: []config.Identity{{
			ID: "gatehouse:alice", Revision: 1, Enabled: true,
			Verifiers: []config.Verifier{{Algorithm: &algorithm, Sources: []config.PasswordSource{"env:GATEHOUSE_TEST_PASSWORD"}}},
		}},
	}}, identity.NewPasswordSourceResolver()); err != nil {
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
