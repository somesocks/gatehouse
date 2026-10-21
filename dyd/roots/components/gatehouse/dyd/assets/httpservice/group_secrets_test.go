package httpservice

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestGroupSecretsAPI(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, managerToken := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil { t.Fatal(err) }
	workspace := workspaces["engineering"]
	group := groupID(t, ctx, store, "engineering", "developers")
	manager, member := authz.Manager, authz.Member
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, nil, &manager, 1, nil); err != nil { t.Fatal(err) }
	var bob string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil { t.Fatal(err) }
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Manager); err != nil { t.Fatal(err) }
	if err := store.SystemGroupMemberCreate(ctx, workspace.Id, group, bob); err != nil { t.Fatal(err) }
	username, password := "secret-bob", "password"
	err, principal := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil { t.Fatal(err) }
	err, memberToken := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: principal.Identities[0].ID})
	if err != nil { t.Fatal(err) }
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	base := "/api/v1/workspaces/"+workspace.Id+"/groups/"+group+"/secrets"
	request := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" { r.Header.Set("Authorization", "Bearer "+token) }
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status { t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, status, w.Body.String()) }
		if strings.Contains(w.Body.String(), "VALUE_SENTINEL") || strings.Contains(w.Body.String(), "ciphertext") { t.Fatal("HTTP response exposed a credential") }
		return w
	}
	request("GET", base, "", "", 401)
	request("POST", base, `{"description":"Deploy","value":"VALUE_SENTINEL"}`, memberToken, 403)
	for _, body := range []string{`{}`, `{"description":"","value":"x"}`, `{"description":"Deploy","value":""}`, `{"description":"Deploy","value":"x","expected_revision":1}`, `{"description":"Deploy","value":"x","extra":true}`, `{"description":"Deploy","value":"x"} {}`} {
		request("POST", base, body, managerToken, 400)
	}
	created := request("POST", base, `{"description":"Deployment token","value":"VALUE_SENTINEL"}`, managerToken, 201)
	var secret groupSecretResponse
	if err := json.Unmarshal(created.Body.Bytes(), &secret); err != nil || secret.Revision != 1 { t.Fatalf("create: %#v %v", secret, err) }
	path := base+"/"+secret.ID
	request("GET", path, "", memberToken, 200)
	pageResponse := request("GET", base+"?limit=1&name=Deployment", "", memberToken, 200)
	var page struct {
		Secrets []groupSecretResponse `json:"secrets"`
		CanManage bool `json:"can_manage_secrets"`
	}
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil || len(page.Secrets) != 1 || page.CanManage || pageResponse.Header().Get("Cache-Control") != "no-store" { t.Fatalf("member list: %#v %v", page, err) }
	request("GET", base+"?cursor=invalid", "", managerToken, 400)
	request("PATCH", path, `{"description":"Denied","expected_revision":1}`, memberToken, 403)
	request("DELETE", path, `{"expected_revision":1}`, memberToken, 403)
	request("PATCH", path, `{"description":"Rotated"}`, managerToken, 400)
	request("PATCH", path, `{"description":"Rotated","expected_revision":1}`, managerToken, 200)
	request("PATCH", path, `{"value":"NEW_VALUE_SENTINEL","expected_revision":1}`, managerToken, 409)
	request("PATCH", path, `{"value":"NEW_VALUE_SENTINEL","expected_revision":2}`, managerToken, 200)
	var ciphertext string
	if err := store.QueryRow(`SELECT ciphertext FROM gatehouse_group_secrets WHERE id = ?`, secret.ID).Scan(&ciphertext); err != nil || !strings.HasPrefix(ciphertext, "gh-enc:") || strings.Contains(ciphertext, "VALUE_SENTINEL") { t.Fatalf("credential not encrypted: %v", err) }
	err, encrypted := keychain.ParseResource(ciphertext)
	if err != nil { t.Fatal(err) }
	err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil { t.Fatal(err) }
	defer ring.Close()
	err, keys := ring.Get(ctx, []model.KeychainRef{*encrypted.Key})
	if err != nil { t.Fatal(err) }
	defer func() { for _, key := range keys { clear(key) } }()
	ref := model.GroupSecretRef{Workspace: workspace, Group: model.GroupRef{Id: group}, Id: secret.ID}
	err, value := keychain.Open(keys[*encrypted.Key], database.GroupSecretAssociatedData(ref), encrypted)
	if err != nil || string(value) != "NEW_VALUE_SENTINEL" { t.Fatalf("credential could not be decrypted: %v", err) }
	clear(value)
	wrongRef := ref
	wrongRef.Group.Id = groupID(t, ctx, store, "operations", "operators")
	if err, _ := keychain.Open(keys[*encrypted.Key], database.GroupSecretAssociatedData(wrongRef), encrypted); err == nil { t.Fatal("credential decrypted under a different group") }
	request("GET", "/api/v1/workspaces/"+workspaces["operations"].Id+"/groups/"+wrongRef.Group.Id+"/secrets/"+secret.ID, "", managerToken, 404)
	// System administrator and workspace manager status do not replace group-manager membership.
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, bob, nil, &manager, 1, nil); err != nil { t.Fatal(err) }
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, nil, &member, 2, nil); err != nil { t.Fatal(err) }
	request("PATCH", path, `{"description":"Denied","expected_revision":3}`, managerToken, 403)
	request("DELETE", path, `{"expected_revision":2}`, memberToken, 409)
	request("DELETE", path, `{}`, memberToken, 400)
	request("DELETE", path, `{"expected_revision":3}`, memberToken, 204)
	request("GET", path, "", memberToken, 404)
	var enabled bool
	if err := store.QueryRow(`SELECT ciphertext, enabled FROM gatehouse_group_secrets WHERE id = ?`, secret.ID).Scan(&ciphertext, &enabled); err != nil || ciphertext != "" || enabled { t.Fatalf("deleted value retained: %t %v", enabled, err) }
}
