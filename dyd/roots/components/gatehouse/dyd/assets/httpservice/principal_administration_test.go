package httpservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestPrincipalLoginAdministration(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	alice, aliceIdentity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: aliceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body, bearer string, status int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		handler.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		return response
	}
	decode := func(response *httptest.ResponseRecorder) database.SystemPrincipal {
		t.Helper()
		if strings.Contains(response.Body.String(), "password") || strings.Contains(response.Body.String(), "gh-ver:") {
			t.Fatal("administration response exposed credentials")
		}
		var principal database.SystemPrincipal
		if err := json.Unmarshal(response.Body.Bytes(), &principal); err != nil {
			t.Fatal(err)
		}
		return principal
	}
	const directory = "/api/v1/system/principals"
	request("POST", directory, `{"name":"New account"}`, "", 401)
	request("POST", directory, `{"name":"New account","username":"forbidden","password":"secret"}`, token, 400)
	request("POST", directory, `{"alias":"Bad Alias"}`, token, 400)
	request("POST", directory, `{} {}`, token, 400)
	principal := decode(request("POST", directory, `{"alias":"new-account","name":"New account"}`, token, 200))
	if len(principal.Identities) != 0 || !principal.Enabled || principal.Revision != 1 {
		t.Fatalf("new principal = %#v", principal)
	}
	request("POST", directory, `{"alias":"new-account"}`, token, 409)
	path := directory + "/" + principal.ID + "/identities"
	for _, body := range []string{`{}`, `{"username":"has spaces","password":"test"}`, `{"username":"gatehouse:test","password":"test"}`, `{"username":"user","password":""}`, `{"username":"user","password":"bad\u0000value"}`} {
		request("POST", path, body, token, 400)
	}
	principal = decode(request("POST", path, `{"username":"new-user","password":"first secret"}`, token, 200))
	login := principal.Identities[0]
	if principal.IdentityCount != 1 || login.Revision != 1 {
		t.Fatalf("added identity = %#v", principal)
	}
	request("POST", path, `{"username":"alice","password":"test"}`, token, 409)
	principal = decode(request("POST", path, `{"username":"second-user","password":"second secret"}`, token, 200))
	if len(principal.Identities) != 2 {
		t.Fatal("principal must support multiple logins")
	}
	identityPath := path + "/" + login.ID
	request("PATCH", identityPath, `{"username":"renamed"}`, token, 400)
	request("PATCH", identityPath, `{"expected_revision":0,"username":"renamed"}`, token, 409)
	request("PATCH", identityPath, `{"expected_revision":1,"username":"alice"}`, token, 409)
	request("PATCH", directory+"/"+alice.Id+"/identities/"+login.ID, `{"expected_revision":1,"password":"wrong owner"}`, token, 404)
	request("DELETE", directory+"/"+alice.Id+"/identities/"+login.ID, `{"expected_revision":1}`, token, 404)
	credentials := request("POST", "/api/v1/auth/login", `{"identity":"new-user","password":"first secret"}`, "", 200)
	var session loginResponse
	if err := json.Unmarshal(credentials.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	request("POST", directory, `{}`, session.AccessToken, 403)
	request("POST", path, `{"username":"denied","password":"test"}`, session.AccessToken, 403)
	request("PATCH", identityPath, `{"expected_revision":1,"enabled":false}`, session.AccessToken, 403)
	request("DELETE", identityPath, `{"expected_revision":1}`, session.AccessToken, 403)
	decode(request("PATCH", identityPath, `{"expected_revision":1,"username":"renamed","password":"replacement secret"}`, token, 200))
	request("GET", "/api/v1/auth/me", "", session.AccessToken, 200)
	request("POST", "/api/v1/auth/login", `{"identity":"new-user","password":"first secret"}`, "", 401)
	request("POST", "/api/v1/auth/login", `{"identity":"renamed","password":"first secret"}`, "", 401)
	request("POST", "/api/v1/auth/login", `{"identity":"renamed","password":"replacement secret"}`, "", 200)
	// Renaming alone must preserve the new verifier, and old revisions cannot delete it.
	decode(request("PATCH", identityPath, `{"expected_revision":2,"username":"renamed-again"}`, token, 200))
	request("POST", "/api/v1/auth/login", `{"identity":"renamed-again","password":"replacement secret"}`, "", 200)
	request("DELETE", identityPath, `{"expected_revision":2}`, token, 409)
	decode(request("PATCH", identityPath, `{"expected_revision":3,"enabled":false}`, token, 200))
	request("GET", "/api/v1/auth/me", "", session.AccessToken, 401)
	request("POST", "/api/v1/auth/login", `{"identity":"renamed-again","password":"replacement secret"}`, "", 401)
	decode(request("PATCH", identityPath, `{"expected_revision":4,"enabled":true}`, token, 200))
	request("GET", "/api/v1/auth/me", "", session.AccessToken, 200)
	principal = decode(request("DELETE", identityPath, `{"expected_revision":5}`, token, 200))
	if len(principal.Identities) != 1 {
		t.Fatal("delete did not remove identity")
	}
	request("GET", "/api/v1/auth/me", "", session.AccessToken, 401)
	request("DELETE", identityPath, `{"expected_revision":5}`, token, 404)
	principal = decode(request("POST", path, `{"username":"renamed-again","password":"reused secret"}`, token, 200))
	for _, identity := range principal.Identities {
		if identity.ID == login.ID {
			t.Fatal("username reuse revived deleted identity")
		}
	}
	request("GET", "/api/v1/auth/me", "", session.AccessToken, 401)
	request("POST", "/api/v1/auth/login", `{"identity":"second-user","password":"second secret"}`, "", 200)
	var count int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_identities WHERE id = ?`, login.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("identity was not physically deleted: %d %v", count, err)
	}
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE detail_identity = ? AND resource_kind = 'principal' AND resource_principal = ?`, login.ID, principal.ID).Scan(&count); err != nil || count != 6 {
		t.Fatalf("principal identity history: %d %v", count, err)
	}
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'identity.remove' AND detail_identity = ?`, login.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("identity removal event: %d %v", count, err)
	}
	// Removing the caller's identity invalidates its token on the next request.
	request("DELETE", fmt.Sprintf("%s/%s/identities/%s", directory, alice.Id, aliceIdentity), `{"expected_revision":1}`, token, 200)
	request("GET", directory, "", token, 401)
}
