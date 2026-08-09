package httpservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/identity"
	"gatehouse/keychain"
)

func TestHandlerEnablesConfiguredRouteGroups(t *testing.T) {
	handler := Handler(config.HTTPService{Web: true, API: true})
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
	handler := Handler(config.HTTPService{})
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
	Handler(config.HTTPService{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET / status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestHandlerAuthenticatesVersionedAPIRoutes(t *testing.T) {
	tokens := testBearerTokens(t)
	handler := Handler(config.HTTPService{API: true}, tokens)
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
	handler := Handler(config.HTTPService{API: true}, testBearerTokens(t))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identity":"gatehouse:alice","password":"wrong password"}`)))
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("POST login = status %d authenticate %q", response.Code, response.Header().Get("WWW-Authenticate"))
	}
}

func testBearerTokens(t *testing.T) *auth.BearerTokens {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	err, migrations := database.BuildMigrations(config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, config.State{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, store, migrations); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_principals (id, enabled) VALUES ('alice', TRUE)`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEHOUSE_TEST_KEYCHAIN", "test passphrase")
	t.Setenv("GATEHOUSE_TEST_PASSWORD", "correct password")
	err, keyring := keychain.Prepare(ctx, store, []config.Keychain{{
		ID: "test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"},
	}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(keyring.Close)
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
	return tokens
}

func TestServiceStartsServesAndStops(t *testing.T) {
	err, service := Start(config.HTTPService{Listen: "127.0.0.1:0"})
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
