package httpservice

import (
	"context"
	"encoding/json"
	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceSpecificSystemSearches(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(w, r)
		return w
	}

	// Each directory is still served by its own resource API and typed model.
	principalPage := request("/api/v1/system/principals?limit=1&name=alice")
	var principalResult struct {
		Principals []database.SystemPrincipal `json:"principals"`
		Next       string                     `json:"next_cursor"`
	}
	if err := json.Unmarshal(principalPage.Body.Bytes(), &principalResult); err != nil || principalPage.Code != 200 || len(principalResult.Principals) != 1 || principalResult.Principals[0].ID != alice.Id || principalResult.Principals[0].IdentityCount != 1 || len(principalResult.Principals[0].Identities) != 0 {
		t.Fatalf("principal search = %d %s (%v)", principalPage.Code, principalPage.Body, err)
	}
	if strings.Contains(principalPage.Body.String(), "verifier") {
		t.Fatal("principal directory exposed verifier")
	}

	err, provider := store.SystemAgentProviderCreate(ctx, database.SystemAgentProviderInput{Alias: "search-test", Protocol: "builtin", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	providerPage := request("/api/v1/system/agent-providers?limit=1&name=search-test&status=disabled")
	var providerResult struct {
		Providers []database.SystemAgentProvider `json:"providers"`
		Next      string                         `json:"next_cursor"`
	}
	if err := json.Unmarshal(providerPage.Body.Bytes(), &providerResult); err != nil || providerPage.Code != 200 || len(providerResult.Providers) != 1 || providerResult.Providers[0].ID != provider.ID || providerResult.Providers[0].Enabled {
		t.Fatalf("agent provider search = %d %s (%v)", providerPage.Code, providerPage.Body, err)
	}

	// The first page is bounded, has a keyset cursor, and subsequent pages do
	// not repeat a row. No provider or model collection is loaded first.
	first := request("/api/v1/system/agent-providers?limit=1")
	providerResult = struct {
		Providers []database.SystemAgentProvider `json:"providers"`
		Next      string                         `json:"next_cursor"`
	}{}
	if err := json.Unmarshal(first.Body.Bytes(), &providerResult); err != nil || len(providerResult.Providers) != 1 || providerResult.Next == "" {
		t.Fatalf("first provider page = %s (%v)", first.Body, err)
	}
	id := providerResult.Providers[0].ID
	second := request("/api/v1/system/agent-providers?limit=1&cursor=" + providerResult.Next)
	providerResult.Providers = nil
	providerResult.Next = ""
	if err := json.Unmarshal(second.Body.Bytes(), &providerResult); err != nil || len(providerResult.Providers) != 1 || providerResult.Providers[0].ID == id {
		t.Fatalf("second provider page = %s (%v)", second.Body, err)
	}

	models := request("/api/v1/system/agent-models?limit=1&name=assistant")
	var modelResult struct {
		Models []database.SystemAgentModel `json:"models"`
	}
	if err := json.Unmarshal(models.Body.Bytes(), &modelResult); err != nil || models.Code != 200 || len(modelResult.Models) != 1 {
		t.Fatalf("model search = %d %s (%v)", models.Code, models.Body, err)
	}
	storage := request("/api/v1/system/storage-providers?limit=1&status=enabled")
	var storageResult struct {
		Providers []database.SystemStorageProvider `json:"providers"`
	}
	if err := json.Unmarshal(storage.Body.Bytes(), &storageResult); err != nil || storage.Code != 200 || len(storageResult.Providers) != 1 {
		t.Fatalf("storage search = %d %s (%v)", storage.Code, storage.Body, err)
	}
	grants := request("/api/v1/system/grants?limit=1&name=Alice")
	var grantsResult struct {
		Grants     []model.SystemGrant        `json:"grants"`
		Principals []database.SystemPrincipal `json:"principals"`
	}
	if err := json.Unmarshal(grants.Body.Bytes(), &grantsResult); err != nil || grants.Code != 200 || len(grantsResult.Grants) != 1 || len(grantsResult.Principals) != 1 || grantsResult.Principals[0].ID != alice.Id {
		t.Fatalf("grant search = %d %s (%v)", grants.Code, grants.Body, err)
	}
	for _, path := range []string{"/api/v1/system/agent-providers?limit=101", "/api/v1/system/agent-models?limit=1&cursor=invalid", "/api/v1/system/storage-providers?limit=1&status=invalid", "/api/v1/system/principals?limit=1&cursor=invalid", "/api/v1/system/grants?limit=1&cursor=invalid"} {
		if w := request(path); w.Code != 400 {
			t.Fatalf("invalid search %s = %d", path, w.Code)
		}
	}
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/system/principals?limit=1", nil))
	if unauthenticated.Code != 401 {
		t.Fatalf("anonymous search = %d", unauthenticated.Code)
	}
}
