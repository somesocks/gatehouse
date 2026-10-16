package httpservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type passwordSetFixture struct {
	tokens                                     *auth.BearerTokens
	store                                      *database.Store
	handler                                    http.Handler
	admin, user, principal, identity, linkPath string
}

func newPasswordSetFixture(t *testing.T) passwordSetFixture {
	t.Helper()
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	alice, aliceIdentity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, admin := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: aliceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	err, principal := store.SystemPrincipalCreate(ctx, database.SystemPrincipalCreateInput{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	username, password := "link-user", "original password"
	err, principal = store.SystemPrincipalIdentityCreate(ctx, principal.ID, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, user := tokens.Login(ctx, "gatehouse:link-user", []byte(password))
	if err != nil {
		t.Fatal(err)
	}
	id := principal.Identities[0].ID
	return passwordSetFixture{tokens, store, Handler(config.HTTPService{API: true, Web: true, PublicBaseURL: "https://gatehouse.example.test"}, store, tokens), admin, user, principal.ID, id,
		"/api/v1/system/principals/" + principal.ID + "/identities/" + id + "/password-link"}
}

func (f passwordSetFixture) request(method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func (f passwordSetFixture) link(t *testing.T) string {
	t.Helper()
	before := time.Now()
	w := f.request("POST", f.linkPath, f.admin, `{"expected_revision":1}`)
	if w.Code != 200 {
		t.Fatalf("mint = %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("link response was cacheable")
	}
	var payload struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	expires, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil || expires.Before(before.Add(24*time.Hour-time.Second)) || expires.After(time.Now().Add(24*time.Hour)) {
		t.Fatalf("expiry = %q", payload.ExpiresAt)
	}
	u, err := url.Parse(payload.URL)
	if err != nil || u.Scheme != "https" || u.Host != "gatehouse.example.test" || u.Path != "/app/set-password/" || u.RawQuery != "" {
		t.Fatalf("link = %q: %v", payload.URL, err)
	}
	// URL.Fragment is already percent-decoded; parse the encoded fragment just
	// as URLSearchParams(window.location.hash) does in the standalone page.
	fragment, err := url.ParseQuery(u.EscapedFragment())
	if err != nil || fragment.Get("capability") == "" {
		t.Fatal("missing fragment capability")
	}
	return fragment.Get("capability")
}

func TestPasswordSetLinks(t *testing.T) {
	f := newPasswordSetFixture(t)
	const path = "/api/v1/auth/set-password"
	for _, test := range []struct {
		token, body string
		status      int
	}{
		{"", `{"expected_revision":1}`, 401},
		{f.user, `{"expected_revision":1}`, 403},
		{f.admin, `{}`, 400},
		{f.admin, `{"expected_revision":0}`, 409},
		{f.admin, `{"expected_revision":1,"password":"forbidden"}`, 400},
	} {
		if w := f.request("POST", f.linkPath, test.token, test.body); w.Code != test.status {
			t.Fatalf("mint = %d, want %d: %s", w.Code, test.status, w.Body.String())
		}
	}
	other, _ := principalIdentityRefs(t, context.Background(), f.store, "alice", "gatehouse:alice")
	wrongOwner := strings.Replace(f.linkPath, f.principal, other.Id, 1)
	if w := f.request("POST", wrongOwner, f.admin, `{"expected_revision":1}`); w.Code != 404 {
		t.Fatalf("wrong owner = %d", w.Code)
	}
	unconfigured := Handler(config.HTTPService{API: true}, f.store, f.tokens)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", f.linkPath, strings.NewReader(`{"expected_revision":1}`))
	r.Header.Set("Authorization", "Bearer "+f.admin)
	unconfigured.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("unconfigured origin = %d", w.Code)
	}

	capability, sibling := f.link(t), f.link(t)
	for _, token := range []string{"", f.user, f.admin, capability + "tampered"} {
		if w := f.request("GET", path, token, ""); w.Code != 401 {
			t.Fatalf("wrong credential = %d", w.Code)
		}
	}
	if w := f.request("GET", "/api/v1/auth/me", capability, ""); w.Code != 401 {
		t.Fatal("capability authenticated as login")
	}
	if w := f.request("GET", "/api/v1/input", capability, ""); w.Code != 401 {
		t.Fatal("capability authenticated as input")
	}
	r = httptest.NewRequest("GET", path, nil)
	r.AddCookie(&http.Cookie{Name: "gatehouse_auth", Value: f.user})
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("login cookie accepted")
	}
	for range 2 {
		w := f.request("GET", path, capability, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"username":"link-user"`) || strings.Contains(w.Body.String(), "verifier") {
			t.Fatalf("read = %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("missing capability response headers")
		}
	}
	if w := f.request("GET", path+"?capability="+url.QueryEscape(capability), "", ""); w.Code != 400 {
		t.Fatal("query capability accepted")
	}
	for _, body := range []string{`{}`, `{"password":""}`, `{"password":"bad\u0000password"}`, `{"password":"secret","username":"other"}`, `{"password":"secret"} {}`} {
		if w := f.request("POST", path, capability, body); w.Code != 400 {
			t.Fatalf("invalid password = %d: %s", w.Code, w.Body.String())
		}
	}
	if err := f.store.PasswordSet(context.Background(), f.principal, f.identity, 1, time.Now().Add(-time.Second), []byte("too late")); !errors.Is(err, database.ErrPasswordLinkExpired) {
		t.Fatalf("mutation accepted expired capability: %v", err)
	}
	if w := f.request("POST", path, capability, `{"password":"replacement password"}`); w.Code != 204 {
		t.Fatalf("set = %d: %s", w.Code, w.Body.String())
	}
	for _, token := range []string{capability, sibling} {
		for _, method := range []string{"GET", "POST"} {
			if w := f.request(method, path, token, `{"password":"replayed"}`); w.Code != 409 {
				t.Fatalf("replay = %d", w.Code)
			}
		}
	}
	if err, _ := f.tokens.Login(context.Background(), "gatehouse:link-user", []byte("original password")); err == nil {
		t.Fatal("old password accepted")
	}
	if err, _ := f.tokens.Login(context.Background(), "gatehouse:link-user", []byte("replacement password")); err != nil {
		t.Fatal(err)
	}
	if w := f.request("GET", "/api/v1/auth/me", f.user, ""); w.Code != 200 {
		t.Fatal("existing session invalidated")
	}
	var count int
	if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'identity.update' AND detail_identity = ?`, f.identity).Scan(&count); err != nil || count != 1 {
		t.Fatalf("activity count = %d: %v", count, err)
	}
}

func TestPasswordSetExpirationAndIdentityChanges(t *testing.T) {
	f := newPasswordSetFixture(t)
	ctx := context.Background()
	capability := f.link(t)
	// Encrypt authenticated test payloads to exercise expiry and claim validation.
	for _, test := range []struct {
		purpose string
		expires int64
		status  int
	}{
		{"identity.set-password", time.Now().Unix() - 1, 410},
		{"identity.set-password", 0, 401},
		{"login", time.Now().Add(time.Hour).Unix(), 401},
	} {
		payload, err := json.Marshal(auth.PasswordSetCapability{Purpose: test.purpose, Principal: f.principal, Identity: f.identity, IdentityRevision: 1, ExpiresAt: test.expires})
		if err != nil {
			t.Fatal(err)
		}
		err, encoded := f.tokens.Encrypt(ctx, []byte("gatehouse identity set-password capability v1"), payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"GET", "POST"} {
			if w := f.request(method, "/api/v1/auth/set-password", encoded, `{"password":"expired"}`); w.Code != test.status {
				t.Fatalf("invalid claims = %d, want %d", w.Code, test.status)
			}
		}
	}
	if err, _ := f.store.SystemPrincipalEnabledSet(ctx, f.principal, false); err != nil {
		t.Fatal(err)
	}
	if w := f.request("GET", "/api/v1/auth/set-password", capability, ""); w.Code != 401 {
		t.Fatal("disabled principal accepted")
	}
	if w := f.request("POST", f.linkPath, f.admin, `{"expected_revision":1}`); w.Code != 404 {
		t.Fatal("minted for disabled principal")
	}
	if err, _ := f.store.SystemPrincipalEnabledSet(ctx, f.principal, true); err != nil {
		t.Fatal(err)
	}
	username := "renamed-link-user"
	if err, _ := f.store.SystemPrincipalIdentityUpdate(ctx, f.principal, f.identity, 1, database.SystemPrincipalIdentityInput{Username: &username}); err != nil {
		t.Fatal(err)
	}
	if w := f.request("GET", "/api/v1/auth/set-password", capability, ""); w.Code != 409 {
		t.Fatal("renamed identity accepted stale link")
	}
	err, encoded, _ := f.tokens.MintPasswordSet(ctx, f.principal, f.identity, 2)
	if err != nil {
		t.Fatal(err)
	}
	enabled := false
	if err, _ := f.store.SystemPrincipalIdentityUpdate(ctx, f.principal, f.identity, 2, database.SystemPrincipalIdentityInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if w := f.request("GET", "/api/v1/auth/set-password", encoded, ""); w.Code != 401 {
		t.Fatal("disabled identity accepted")
	}
	if err, _ := f.store.SystemPrincipalIdentityRemove(ctx, f.principal, f.identity, 3); err != nil {
		t.Fatal(err)
	}
	if w := f.request("POST", "/api/v1/auth/set-password", encoded, `{"password":"deleted"}`); w.Code != 401 {
		t.Fatal("deleted identity accepted")
	}
	// Non-local identities cannot be issued or redeemed as password links.
	username, password := "external-login", "placeholder"
	err, principal := f.store.SystemPrincipalIdentityCreate(ctx, f.principal, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	id := principal.Identities[0].ID
	if _, err := f.store.ExecContext(ctx, `UPDATE gatehouse_identities SET key = 'external:login' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	err, externalCapability, _ := f.tokens.MintPasswordSet(ctx, f.principal, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if w := f.request("POST", strings.Replace(f.linkPath, f.identity, id, 1), f.admin, `{"expected_revision":1}`); w.Code != 404 {
		t.Fatal("minted password link for external identity")
	}
	if w := f.request("POST", "/api/v1/auth/set-password", externalCapability, `{"password":"external"}`); w.Code != 401 {
		t.Fatal("set password for external identity")
	}
}

func TestPasswordSetConcurrentRedemption(t *testing.T) {
	f := newPasswordSetFixture(t)
	capability := f.link(t)
	var group sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			statuses <- f.request("POST", "/api/v1/auth/set-password", capability, `{"password":"concurrent password"}`).Code
		}()
	}
	group.Wait()
	first, second := <-statuses, <-statuses
	if !((first == 204 && second == 409) || (first == 409 && second == 204)) {
		t.Fatalf("concurrent statuses = %d, %d", first, second)
	}
}
