package httpservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
)

type fakeCodexLogin struct {
	calls  int
	result *codexlogin.PollOutcome
	onPoll func()
}

func (client *fakeCodexLogin) Start(context.Context) (codexlogin.StartResult, error) {
	client.calls++
	return codexlogin.StartResult{DeviceAuthID: "device", UserCode: "ABCD-EFGH", VerificationURI: "https://auth.openai.com/codex/device", ExpiresAt: time.Now().Add(15 * time.Minute), PollAfterMS: 5000}, nil
}
func (client *fakeCodexLogin) Poll(_ context.Context, id, code string) (codexlogin.PollOutcome, error) {
	client.calls++
	if client.onPoll != nil {
		client.onPoll()
	}
	if client.result != nil {
		return *client.result, nil
	}
	return codexlogin.PollOutcome{PollResult: codexlogin.PollResult{Status: "pending", PollAfterMS: 5000}}, nil
}

func codexTestRing(t *testing.T, store *database.Store) *keychain.Keyring {
	t.Helper()
	err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: "test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	return ring
}

func TestCodexLoginAdministration(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	principal, identity := principalIdentityRefs(t, context.Background(), store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(context.Background(), auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeCodexLogin{}
	ring := codexTestRing(t, store)
	err, provider := store.SystemAgentProviderCreate(context.Background(), database.SystemAgentProviderInput{Alias: "chatgpt", Protocol: config.AgentProviderProtocolOpenAICodex, Keychain: &model.KeychainRef{Id: "test", Version: 1}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	startBody := `{"provider":"` + provider.ID + `","expected_revision":1}`
	pollBody := `{"provider":"` + provider.ID + `","expected_revision":1,"device_auth_id":"device","user_code":"code"}`
	mux := http.NewServeMux()
	const base = "/api/v1/system/agent-providers/openai-codex/login/"
	mux.HandleFunc(base+"start", codexLoginStart(store, ring, tokens, client))
	mux.HandleFunc(base+"poll", codexLoginPoll(store, ring, tokens, client))
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
	if w := request(mux, "start", http.MethodPost, startBody, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"device_auth_id":"device"`) {
		t.Fatalf("start = %d %s", w.Code, w.Body.String())
	}
	if w := request(mux, "poll", http.MethodPost, pollBody, token); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"pending"`) {
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

func TestCodexLoginInstallsEncryptedCredentialAndFencesLateCompletion(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	ring := codexTestRing(t, store)
	principal, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	err, provider := store.SystemAgentProviderCreate(ctx, database.SystemAgentProviderInput{Alias: "chatgpt", Protocol: config.AgentProviderProtocolOpenAICodex, Keychain: &model.KeychainRef{Id: "test", Version: 1}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	client := &fakeCodexLogin{result: &codexlogin.PollOutcome{
		PollResult: codexlogin.PollResult{Status: "succeeded", Account: &codexlogin.Account{ID: "account", Email: "user@example.com"}, ExpiresAt: &expires},
		Credential: &codexlogin.Credential{Version: 1, AccessToken: "ACCESS_SENTINEL", RefreshToken: "REFRESH_SENTINEL", AccountID: "account", Email: "user@example.com", ExpiresAt: expires},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("/poll", codexLoginPoll(store, ring, tokens, client))
	mux.HandleFunc("/providers/{provider}/disconnect", codexProviderDisconnect(store, tokens))
	request := func(path string, input any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", path, strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	poll := func(revision int) *httptest.ResponseRecorder {
		return request("/poll", map[string]any{"provider": provider.ID, "expected_revision": revision, "device_auth_id": "device", "user_code": "code"})
	}
	response := poll(1)
	if response.Code != 200 {
		t.Fatalf("connect = %d %s", response.Code, response.Body.String())
	}
	for _, secret := range []string{"ACCESS_SENTINEL", "REFRESH_SENTINEL", "access_token", "refresh_token"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("response leaked %s", secret)
		}
	}
	err, record := store.AgentProviderCredentialGet(ctx, provider.ID)
	if err != nil || record == nil || record.Encrypted == nil || record.Provider.Revision != 2 || record.Provider.Account == nil || record.Provider.Account.Email != "user@example.com" {
		t.Fatalf("connected record = %#v %v", record, err)
	}
	if strings.Contains(*record.Encrypted, "SENTINEL") {
		t.Fatal("credential was not encrypted")
	}
	err, encrypted := keychain.ParseKey(*record.Encrypted)
	if err != nil {
		t.Fatal(err)
	}
	err, keys := ring.Get(ctx, []model.KeychainRef{*record.Provider.Keychain})
	if err != nil {
		t.Fatal(err)
	}
	err, plain := keychain.Open(keys[*record.Provider.Keychain], []byte("gh=v1|agent-provider|alias=chatgpt"), encrypted)
	if err != nil {
		t.Fatal(err)
	}
	var credential codexlogin.Credential
	if err := json.Unmarshal(plain, &credential); err != nil {
		t.Fatal(err)
	}
	clear(plain)
	clear(keys[*record.Provider.Keychain])
	if credential.Version != 1 || credential.AccessToken != "ACCESS_SENTINEL" || credential.RefreshToken != "REFRESH_SENTINEL" || credential.AccountID != "account" {
		t.Fatal("incorrect credential bundle")
	}
	// A stale request is rejected before it can consume another upstream code.
	calls := client.calls
	if response := poll(1); response.Code != 409 || client.calls != calls {
		t.Fatalf("stale connect = %d", response.Code)
	}
	// Configuration-only writes retain ciphertext and its account metadata.
	err, updated := store.SystemAgentProviderUpdate(ctx, provider.ID, 2, database.SystemAgentProviderInput{Alias: "chatgpt", Protocol: config.AgentProviderProtocolOpenAICodex, Keychain: record.Provider.Keychain, Enabled: false})
	if err != nil || updated.Revision != 3 || updated.Account == nil {
		t.Fatalf("metadata update = %#v %v", updated, err)
	}
	err, retained := store.AgentProviderCredentialGet(ctx, provider.ID)
	if err != nil || *retained.Encrypted != *record.Encrypted {
		t.Fatal("metadata edit replaced credential")
	}
	// A disconnect while exchange is in flight prevents a late reconnect.
	client.onPoll = func() {
		w := request("/providers/"+provider.ID+"/disconnect", map[string]any{"expected_revision": 3})
		if w.Code != 200 {
			t.Fatalf("disconnect = %d %s", w.Code, w.Body.String())
		}
	}
	if response := poll(3); response.Code != 409 {
		t.Fatalf("late connect = %d %s", response.Code, response.Body.String())
	}
	err, disconnected := store.AgentProviderCredentialGet(ctx, provider.ID)
	if err != nil || disconnected.Encrypted != nil || disconnected.Provider.Account != nil || disconnected.Provider.Revision != 4 || disconnected.Provider.Enabled {
		t.Fatalf("disconnect state = %#v %v", disconnected, err)
	}
	// Reconnect installs the new session only after another successful exchange.
	client.onPoll = nil
	client.result.Credential.AccessToken = "NEW_ACCESS_SENTINEL"
	if response := poll(4); response.Code != 200 {
		t.Fatalf("reconnect = %d %s", response.Code, response.Body.String())
	}
}
