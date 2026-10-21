package httpservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestSystemGroupAdministrationAPI(t *testing.T) {
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
	username, password := "bob-admin-test", "password"
	err, principal := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, ordinary := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: principal.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["private"]
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Manager); err != nil {
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
	base := "/api/v1/system/workspaces/" + workspace.Id + "/groups"
	request("GET", base, "", "", 401)
	request("GET", base, "", ordinary, 403)
	request("POST", base, `{"alias":"denied"}`, ordinary, 403)
	for _, body := range []string{`{}`, `{"alias":"bad.alias"}`, `{"alias":"has spaces"}`, `{"alias":"test","unexpected":true}`, `{"alias":"test"} {}`} {
		request("POST", base, body, admin, 400)
	}
	created := request("POST", base, `{"alias":"admin-developers","name":"Developers"}`, admin, 201)
	var group database.SystemGroup
	if err := json.Unmarshal(created.Body.Bytes(), &group); err != nil || group.Revision != 1 || !group.Enabled {
		t.Fatalf("create = %#v %v", group, err)
	}
	request("POST", base, `{"alias":"admin-developers"}`, admin, 409)
	path := base + "/" + group.ID
	request("GET", path, "", ordinary, 403)
	request("PATCH", path, `{"enabled":false,"expected_revision":1}`, ordinary, 403)
	request("PATCH", path, `{"enabled":false}`, admin, 400)
	request("PATCH", path, `{"enabled":false,"expected_revision":1}`, admin, 200)
	request("PATCH", path, `{"name":"stale","expected_revision":1}`, admin, 409)
	request("PATCH", path, `{"alias":"admin-engineering","enabled":true,"expected_revision":2}`, admin, 200)
	otherPath := strings.Replace(path, workspace.Id, workspaces["engineering"].Id, 1)
	for _, suffix := range []string{"", "/members", "/agents", "/grants"} {
		request("GET", otherPath+suffix, "", admin, 404)
	}
	request("POST", otherPath+"/members", `{"principal":"`+bob+`"}`, admin, 404)
	request("POST", path+"/members", `{"principal":"`+bob+`"}`, ordinary, 403)
	request("POST", path+"/members", `{"principal":"`+bob+`"}`, admin, 201)
	request("POST", path+"/members", `{"principal":"`+bob+`"}`, admin, 409)
	request("POST", path+"/members", `{"principal":"prn_00000000000000000000000000"}`, admin, 404)
	memberPath := path + "/members/" + bob
	request("PATCH", memberPath, `{"enabled":false,"expected_revision":1}`, admin, 204)
	request("PATCH", memberPath, `{"enabled":true,"expected_revision":1}`, admin, 409)
	request("POST", path+"/members", `{"principal":"`+bob+`"}`, admin, 201)
	listed := request("GET", path+"/members", "", admin, 200)
	var roster struct {
		Members []database.SystemGroupMember `json:"members"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &roster); err != nil || len(roster.Members) != 1 || roster.Members[0].Revision != 3 {
		t.Fatalf("roster = %#v %v", roster, err)
	}
	var modelID, foreignBot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_agent_models WHERE alias = 'assistant'`).Scan(&modelID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = ?`, workspaces["engineering"].Id).Scan(&foreignBot); err != nil {
		t.Fatal(err)
	}
	err, bot := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "group-assistant", ModelID: modelID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	agentsPath := path + "/agents"
	request("GET", agentsPath, "", "", 401)
	request("GET", agentsPath, "", ordinary, 403)
	request("POST", agentsPath, `{"agent":"`+bot.ID+`"}`, ordinary, 403)
	request("POST", otherPath+"/agents", `{"agent":"`+bot.ID+`"}`, admin, 404)
	for _, body := range []string{`{}`, `{"agent":"bad"}`, `{"agent":"` + bob + `"}`, `{"agent":"` + bot.ID + `","unexpected":true}`, `{"agent":"` + bot.ID + `"} {}`} {
		request("POST", agentsPath, body, admin, 400)
	}
	request("POST", agentsPath, `{"agent":"`+foreignBot+`"}`, admin, 404)
	request("POST", agentsPath, `{"agent":"wag_00000000000000000000000000"}`, admin, 404)
	request("POST", agentsPath, `{"agent":"`+bot.ID+`"}`, admin, 201)
	request("POST", agentsPath, `{"agent":"`+bot.ID+`"}`, admin, 409)
	botPath := agentsPath + "/" + bot.ID
	request("PATCH", botPath, `{"enabled":false,"expected_revision":1}`, ordinary, 403)
	request("PATCH", botPath, `{"enabled":false}`, admin, 400)
	request("PATCH", botPath, `{"enabled":false,"expected_revision":0}`, admin, 400)
	request("PATCH", botPath, `{"enabled":false,"expected_revision":1}`, admin, 204)
	request("POST", agentsPath, `{"agent":"`+bot.ID+`"}`, admin, 201)
	request("PATCH", botPath, `{"enabled":true,"expected_revision":1}`, admin, 409)
	request("PATCH", agentsPath+"/"+foreignBot, `{"enabled":false,"expected_revision":1}`, admin, 404)
	request("PATCH", strings.Replace(botPath, workspace.Id, workspaces["engineering"].Id, 1), `{"enabled":true,"expected_revision":2}`, admin, 404)
	request("PATCH", botPath, `{"enabled":true,"expected_revision":2}`, admin, 409)
	request("PATCH", botPath, `{"enabled":true,"expected_revision":3}`, admin, 204)
	var botRoster struct {
		Agents []database.SystemGroupAgent `json:"agents"`
	}
	if err := json.Unmarshal(request("GET", agentsPath+"?status=enabled&name=group-assistant", "", admin, 200).Body.Bytes(), &botRoster); err != nil || len(botRoster.Agents) != 1 || botRoster.Agents[0].Revision != 3 || !botRoster.Agents[0].AgentEnabled {
		t.Fatalf("bot roster = %#v %v", botRoster, err)
	}
	request("GET", agentsPath+"?status=invalid", "", admin, 400)
	request("GET", agentsPath+"?cursor="+bob, "", admin, 400)
	request("DELETE", botPath, "", admin, 405)
	request("POST", path+"/grants", `{"role":"member"}`, admin, 404)
	request("GET", path+"/grants", "", admin, 404)
	for i := 0; i < 3; i++ {
		request("POST", base, fmt.Sprintf(`{"alias":"page-%d","enabled":false}`, i), admin, 201)
	}
	cursor := ""
	seen := map[string]bool{}
	for {
		response := request("GET", base+"?name=page-&status=disabled&limit=1&cursor="+cursor, "", admin, 200)
		var page struct {
			Groups []database.SystemGroup `json:"groups"`
			Next   string                 `json:"next_cursor"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || len(page.Groups) != 1 || seen[page.Groups[0].ID] {
			t.Fatalf("page = %#v %v", page, err)
		}
		seen[page.Groups[0].ID] = true
		cursor = page.Next
		if cursor == "" {
			break
		}
		if len(seen) > 3 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("paged groups = %d", len(seen))
	}
	request("GET", base+"?status=invalid", "", admin, 400)
	request("GET", base+"?cursor="+bob, "", admin, 400)
}

func TestGroupMemberOptionsRequireCurrentWorkspaceAdmission(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["private"]
	alias := "admission-test"
	err, group := store.SystemGroupCreate(ctx, workspace.Id, database.SystemGroupInput{Alias: &alias})
	if err != nil {
		t.Fatal(err)
	}
	var bob string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	base := "/api/v1/system/workspaces/" + workspace.Id + "/groups/" + group.ID
	request := func(method, suffix, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, base+suffix, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, suffix, w.Code, w.Body.String())
		}
		return w
	}
	options := func(want int) {
		t.Helper()
		var page struct {
			Principals []database.SystemWorkspacePrincipalOption `json:"principals"`
		}
		if err := json.Unmarshal(request("GET", "/member-options?name=bob&limit=1", "", 200).Body.Bytes(), &page); err != nil || len(page.Principals) != want {
			t.Fatalf("options: %#v %v", page, err)
		}
		if want == 1 && page.Principals[0].ID != bob {
			t.Fatalf("wrong workspace principal: %#v", page)
		}
	}
	options(0)
	request("POST", "/members", `{"principal":"`+bob+`"}`, 400)
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Member); err != nil {
		t.Fatal(err)
	}
	options(1)
	request("POST", "/members", `{"principal":"`+bob+`"}`, 201)
	if err, removed := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, bob); err != nil || !removed {
		t.Fatalf("revoke admission: %t %v", removed, err)
	}
	options(0)
	request("PATCH", "/members/"+bob, `{"enabled":false,"expected_revision":1}`, 204)
	request("PATCH", "/members/"+bob, `{"enabled":true,"expected_revision":2}`, 400)
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Member); err != nil {
		t.Fatal(err)
	}
	options(1)
	request("PATCH", "/members/"+bob, `{"enabled":true,"expected_revision":2}`, 204)
	request("POST", "/grants", `{"role":"manager"}`, 404)
}
