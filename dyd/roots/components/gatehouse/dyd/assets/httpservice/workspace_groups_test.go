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

func TestWorkspaceGroupManagement(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, admin := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	var bob string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	username, password := "group-bob", "password"
	err, principal := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, member := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: principal.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["engineering"]
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Guest); err != nil {
		t.Fatal(err)
	}
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
	base := "/api/v1/workspaces/" + workspace.Id + "/groups"
	capabilities := "/api/v1/workspaces/" + workspace.Id + "/group-capabilities"
	if !strings.Contains(request("GET", capabilities, "", admin, 200).Body.String(), `"create":true`) {
		t.Fatal("workspace manager cannot create")
	}
	if !strings.Contains(request("GET", capabilities, "", member, 200).Body.String(), `"create":false`) {
		t.Fatal("ordinary member can create")
	}
	request("POST", base, `{"alias":"no-access"}`, "", 401)
	request("POST", base, `{"alias":"not-allowed"}`, member, 403)
	var group database.SystemGroup
	if err := json.Unmarshal(request("POST", base, `{"alias":"delegated","name":"Delegated"}`, admin, 201).Body.Bytes(), &group); err != nil || group.Role != authz.Admin {
		t.Fatalf("creator role: %#v %v", group, err)
	}
	path := base + "/" + group.ID
	systemPath := "/api/v1/system/workspaces/" + workspace.Id + "/groups/" + group.ID
	for _, groupPath := range []string{path, systemPath} {
		for _, body := range []string{`{"role":"member","expected_revision":1}`, `{"enabled":false,"expected_revision":1}`} {
			response := request("PATCH", groupPath+"/members/"+alice.Id, body, admin, 409)
			if !strings.Contains(response.Body.String(), "at least one enabled admin") {
				t.Fatalf("last-manager error: %s", response.Body.String())
			}
		}
	}
	request("GET", path, "", member, 404)
	request("GET", strings.Replace(path, workspace.Id, workspaces["private"].Id, 1), "", admin, 404)
	request("POST", path+"/members", `{"principal":"`+bob+`","role":"contributor"}`, admin, 400)
	request("POST", path+"/members", `{"principal":"`+bob+`"}`, admin, 201)
	var roster struct {
		Members []database.SystemGroupMember `json:"members"`
	}
	if err := json.Unmarshal(request("GET", path+"/members", "", member, 200).Body.Bytes(), &roster); err != nil || len(roster.Members) != 2 {
		t.Fatalf("roster: %#v %v", roster, err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_principals SET enabled = FALSE WHERE id = ?`, bob); err != nil {
		t.Fatal(err)
	}
	var activeRoster struct {
		Members []database.SystemGroupMember `json:"members"`
		Next    string                       `json:"next_cursor"`
	}
	if err := json.Unmarshal(request("GET", path+"/members?limit=1&status=disabled", "", admin, 200).Body.Bytes(), &activeRoster); err != nil || len(activeRoster.Members) != 1 || activeRoster.Members[0].Principal != alice.Id || activeRoster.Next != "" {
		t.Fatalf("visible people: %#v %v", activeRoster, err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_principals SET enabled = TRUE WHERE id = ?`, bob); err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{"/member-options", "/agent-options"} {
		request("GET", path+destination, "", member, 403)
	}
	request("PATCH", path, `{"name":"denied","expected_revision":1}`, member, 403)
	request("PATCH", path+"/members/"+bob, `{"role":"admin","expected_revision":1}`, member, 403)
	request("PATCH", path+"/members/"+bob, `{"role":"admin","expected_revision":1}`, admin, 204)
	request("PATCH", path+"/members/"+bob, `{"role":"member","expected_revision":1}`, admin, 409)
	// A group manager need only be an ordinary workspace member.
	request("PATCH", path, `{"name":"Managed by Bob","expected_revision":1}`, member, 200)
	request("GET", path+"/member-options", "", member, 200)
	request("GET", path+"/agent-options", "", member, 200)
	request("GET", path+"/agent-options?cursor="+bob, "", member, 400)
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = ? LIMIT 1`, workspace.Id).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	request("POST", path+"/agents", `{"agent":"`+bot+`"}`, member, 201)
	request("PATCH", path+"/agents/"+bot, `{"enabled":false,"expected_revision":1}`, member, 204)
	if body := request("GET", path+"/agents?status=disabled&limit=1", "", member, 200).Body.String(); strings.Contains(body, bot) {
		t.Fatalf("removed bot is visible: %s", body)
	}
	request("POST", path+"/agents", `{"agent":"`+bot+`"}`, member, 201)
	request("PATCH", path+"/agents/"+bot, `{"enabled":true,"expected_revision":2}`, member, 409)
	if _, err := store.Exec(`UPDATE gatehouse_workspace_agents SET enabled = FALSE, "default" = FALSE WHERE id = ?`, bot); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"/agents?limit=1", "/agent-options?limit=1"} {
		if body := request("GET", path+suffix, "", member, 200).Body.String(); strings.Contains(body, bot) {
			t.Fatalf("deleted bot is visible: %s", body)
		}
	}
	request("POST", path+"/agents", `{"agent":"`+bot+`"}`, member, 404)
	if _, err := store.Exec(`UPDATE gatehouse_workspace_agents SET enabled = TRUE WHERE id = ?`, bot); err != nil {
		t.Fatal(err)
	}
	request("PATCH", path+"/members/"+alice.Id, `{"role":"member","expected_revision":1}`, member, 204)
	// Alice remains a workspace/system manager, but is not a group manager on workspace routes.
	request("PATCH", path, `{"name":"denied","expected_revision":2}`, admin, 403)
	request("PATCH", path, `{"enabled":false,"expected_revision":2}`, member, 200)
	request("GET", path, "", admin, 404)
	request("GET", path, "", member, 404)
	request("PATCH", path, `{"enabled":true,"expected_revision":3}`, member, 404)
	for _, token := range []string{admin, member} {
		var groups []database.SystemGroup
		if err := json.Unmarshal(request("GET", base+"?name=delegated&limit=1", "", token, 200).Body.Bytes(), &groups); err != nil || len(groups) != 0 {
			t.Fatalf("deleted group listed: %#v %v", groups, err)
		}
	}
	request("PATCH", systemPath, `{"enabled":true,"expected_revision":3}`, admin, 200)
	request("PATCH", path+"/members/"+alice.Id, `{"enabled":false,"expected_revision":2}`, member, 204)
	request("GET", path, "", admin, 404)
	if body := request("GET", path+"/members?status=disabled", "", member, 200).Body.String(); strings.Contains(body, alice.Id) {
		t.Fatalf("removed member is visible: %s", body)
	}
	request("POST", path+"/members", `{"principal":"`+alice.Id+`"}`, member, 201)
	if err, _ := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, bob); err != nil {
		t.Fatal(err)
	}
	request("PATCH", path, `{"name":"revoked","expected_revision":4}`, member, 404)
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Guest); err != nil {
		t.Fatal(err)
	}
	request("PATCH", path, `{"name":"restored","expected_revision":4}`, member, 200)
	request("PATCH", path+"/members/"+bob, `{"role":"member","expected_revision":2}`, member, 409)
	request("PATCH", path+"/members/"+bob, `{"enabled":false,"expected_revision":2}`, member, 409)
	// Appoint a replacement before the current manager steps down.
	request("PATCH", systemPath+"/members/"+alice.Id, `{"role":"admin","expected_revision":4}`, admin, 204)
	request("PATCH", path+"/members/"+bob, `{"role":"member","expected_revision":2}`, member, 204)
	request("PATCH", path, `{"name":"demoted","expected_revision":5}`, member, 403)
	// System administration can appoint managers independently of group membership.
	request("PATCH", systemPath+"/members/"+bob, `{"role":"admin","expected_revision":3}`, admin, 204)
	request("PATCH", path, `{"name":"appointed","expected_revision":5}`, member, 200)
}
