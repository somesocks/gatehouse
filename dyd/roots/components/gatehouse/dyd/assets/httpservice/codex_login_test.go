package httpservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/model"
)

type fakeCodexLogin struct{ calls int }

func (client *fakeCodexLogin) Start(context.Context) (codexlogin.StartResult, error) {
	client.calls++
	return codexlogin.StartResult{DeviceAuthID: "device", UserCode: "ABCD-EFGH", VerificationURI: "https://auth.openai.com/codex/device", ExpiresAt: time.Now().Add(15 * time.Minute), PollAfterMS: 5000}, nil
}
func (client *fakeCodexLogin) Poll(_ context.Context, id, code string) (codexlogin.PollResult, error) {
	client.calls++
	return codexlogin.PollResult{Status: "pending", PollAfterMS: 5000}, nil
}

func TestCodexLoginAdministration(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	principal, identity := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(context.Background(), auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeCodexLogin{}
	mux := http.NewServeMux()
	const base = "/api/v1/system/agent-providers/openai-codex/login/"
	mux.HandleFunc(base+"start", codexLoginStart(store, tokens, client))
	mux.HandleFunc(base+"poll", codexLoginPoll(store, tokens, client))
	request := func(handler http.Handler, action, method, body, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, base+action, strings.NewReader(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("login response can be cached")
		}
		return w
	}
	// Exercise registration on the production mux without making upstream calls.
	production := Handler(config.HTTPService{API: true}, store, tokens)
	for _, action := range []string{"start", "poll"} {
		if w := request(production, action, http.MethodPost, `{}`, ""); w.Code != 401 {
			t.Fatalf("unauthenticated %s = %d", action, w.Code)
		}
		if w := request(mux, action, http.MethodGet, `{}`, token); w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Fatalf("GET %s = %d", action, w.Code)
		}
	}
	for _, body := range []string{``, `{"unexpected":true}`, `{} {}`, strings.Repeat(" ", 8193) + `{}`} {
		if w := request(mux, "start", http.MethodPost, body, token); w.Code != 400 {
			t.Fatalf("invalid start = %d", w.Code)
		}
	}
	for _, body := range []string{`{}`, `{"device_auth_id":"device"}`, `{"device_auth_id":" ","user_code":"code"}`, `{"device_auth_id":"device","user_code":"code","access_token":"secret"}`} {
		if w := request(mux, "poll", http.MethodPost, body, token); w.Code != 400 {
			t.Fatalf("invalid poll = %d", w.Code)
		}
	}
	if client.calls != 0 {
		t.Fatal("invalid request reached upstream")
	}
	if w := request(mux, "start", http.MethodPost, `{}`, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"device_auth_id":"device"`) {
		t.Fatalf("start = %d %s", w.Code, w.Body.String())
	}
	if w := request(mux, "poll", http.MethodPost, `{"device_auth_id":"device","user_code":"code"}`, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatalf("poll = %d %s", w.Code, w.Body.String())
	}
	if client.calls != 2 {
		t.Fatalf("upstream calls = %d", client.calls)
	}
	if _, err := store.ExecContext(context.Background(), `UPDATE gatehouse_system_grants SET enabled = FALSE`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"start", "poll"} {
		if w := request(mux, action, http.MethodPost, `{}`, token); w.Code != 403 {
			t.Fatalf("non-manager %s = %d", action, w.Code)
		}
	}
	if client.calls != 2 {
		t.Fatal("non-manager reached upstream")
	}
}
