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
	"gatehouse/model"
)

func TestSystemWorkspaceGrantRoleAPI(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, aliceIdentity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, admin := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: aliceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	var bob string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	username, password := "bob-login", "bob password"
	err, principal := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, ordinary := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: principal.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	// The sysadmin does not need workspace membership to administer this grant.
	workspace := workspaces["private"]
	err, grant := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Member)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/system/workspaces/" + workspace.Id + "/grants/" + grant.ID
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	const body = `{"role":"contributor","expected_revision":1}`
	request("PATCH", path, body, "", 401)
	request("PATCH", path, body, ordinary, 403)
	request("GET", path, "", admin, 405)
	for _, body := range []string{`{}`, `{"role":"owner","expected_revision":1}`, `{"role":"contributor"}`, `{"role":"contributor","expected_revision":0}`, `{"role":"contributor","expected_revision":1,"principal":"other"}`, `{"role":"contributor","expected_revision":1} {}`} {
		request("PATCH", path, body, admin, 400)
	}
	request("PATCH", strings.Replace(path, workspace.Id, workspaces["engineering"].Id, 1), body, admin, 404)
	request("PATCH", strings.Replace(path, grant.ID, "wgr_00000000000000000000000000", 1), body, admin, 404)
	response := request("PATCH", path, body, admin, 200)
	var updated database.SystemWorkspaceGrant
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil || updated.ID != grant.ID || updated.Principal == nil || *updated.Principal != bob || updated.Role != authz.Contributor || updated.Revision != 2 || !updated.Enabled {
		t.Fatalf("updated grant = (%#v, %v)", updated, err)
	}
	request("PATCH", path, body, admin, 409)
	roster := request("GET", "/api/v1/system/workspaces/"+workspace.Id+"/principals", "", admin, 200)
	var page systemWorkspacePrincipalPage
	if err := json.Unmarshal(roster.Body.Bytes(), &page); err != nil || len(page.Principals) != 1 || len(page.Principals[0].Grants) != 1 || page.Principals[0].Grants[0].ID != grant.ID || page.Principals[0].Grants[0].Revision != 2 || page.Principals[0].Roles[0] != authz.Contributor {
		t.Fatalf("roster grant metadata = (%#v, %v)", page, err)
	}
	err, duplicate := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Manager)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_workspace_grants SET enabled = FALSE WHERE id = ?`, duplicate.ID); err != nil {
		t.Fatal(err)
	}
	collision := request("PATCH", path, `{"role":"manager","expected_revision":2}`, admin, 409)
	if !strings.Contains(collision.Body.String(), "Another grant") {
		t.Fatal("missing role collision explanation")
	}
	// The role change takes effect in authorization immediately.
	err, roles := store.WorkspaceRolesGet(ctx, workspace, model.PrincipalRef{Id: bob})
	if err != nil || !authz.WorkspaceAllows(roles, authz.WorkspaceProjectCreate) {
		t.Fatalf("updated role cannot create projects: %v %v", roles, err)
	}
	var activity int
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'workspace_grant.update' AND resource_workspace_grant = ?`, grant.ID).Scan(&activity); err != nil || activity != 1 {
		t.Fatalf("activity count = %d: %v", activity, err)
	}
}
