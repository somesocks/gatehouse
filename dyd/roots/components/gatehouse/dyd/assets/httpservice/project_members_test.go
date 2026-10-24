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

func TestProjectMemberAPI(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, manager := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	var bob string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	username, password := "project-bob", "password"
	err, principal := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, ordinary := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: principal.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["engineering"]
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Admin); err != nil {
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
	created := request("POST", "/api/v1/workspaces/"+workspace.Id+"/projects", `{"name":"Shared project"}`, manager, 201)
	var project projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + workspace.Id + "/projects/" + project.ID
	request("GET", base+"/members", "", "", 401)
	request("GET", base+"/members", "", ordinary, 404)
	request("POST", base+"/grants", `{"principal":"`+bob+`","role":"admin"}`, ordinary, 404)
	for _, query := range []string{"?kind=agent", "?cursor=invalid", "?limit=101"} {
		request("GET", base+"/members"+query, "", manager, 400)
	}
	for _, body := range []string{`{}`, `{"principal":"` + bob + `","group":"invalid","role":"guest"}`, `{"principal":"` + bob + `","role":"owner"}`, `{"agent":"wag_00000000000000000000000000","role":"guest"}`} {
		request("POST", base+"/grants", body, manager, 400)
	}
	added := request("POST", base+"/grants", `{"principal":"`+bob+`","role":"guest"}`, manager, 201)
	var grant database.ProjectGrant
	if err := json.Unmarshal(added.Body.Bytes(), &grant); err != nil || grant.Revision != 1 {
		t.Fatalf("created grant: %#v %v", grant, err)
	}
	request("POST", base+"/grants", `{"principal":"`+bob+`","role":"guest"}`, manager, 409)
	var page struct {
		Members   []database.ProjectMember `json:"members"`
		CanManage bool                     `json:"can_manage_access"`
	}
	roster := request("GET", base+"/members", "", ordinary, 200)
	if err := json.Unmarshal(roster.Body.Bytes(), &page); err != nil || len(page.Members) != 2 || page.CanManage || roster.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("member roster: %#v %v", page, err)
	}
	for _, suffix := range []string{"/principal-options", "/group-options"} {
		request("GET", base+suffix, "", ordinary, 403)
		request("GET", base+suffix, "", manager, 200)
	}
	path := base + "/grants/" + grant.ID
	request("PATCH", path, `{"role":"admin","expected_revision":1}`, ordinary, 403)
	request("PATCH", path, `{"role":"member"}`, manager, 400)
	request("PATCH", path, `{"role":"member","expected_revision":1}`, manager, 200)
	request("PATCH", path, `{"enabled":false,"expected_revision":1}`, manager, 409)
	group := groupID(t, ctx, store, "engineering", "developers")
	request("POST", base+"/grants", `{"group":"`+group+`","role":"member"}`, manager, 201)
	groupPage := request("GET", base+"/members?kind=group", "", ordinary, 200)
	if err := json.Unmarshal(groupPage.Body.Bytes(), &page); err != nil || len(page.Members) != 1 || page.Members[0].ID != group {
		t.Fatalf("group roster: %#v %v", page, err)
	}
	request("PATCH", strings.Replace(path, workspace.Id, workspaces["operations"].Id, 1), `{"enabled":false,"expected_revision":2}`, manager, 404)
	request("PATCH", path, `{"enabled":false,"expected_revision":2}`, manager, 200)
	request("GET", base+"/members", "", ordinary, 404)
	request("POST", base+"/grants", `{"principal":"`+bob+`","role":"admin"}`, manager, 201)
	request("POST", base+"/grants", `{"principal":"`+alice.Id+`","role":"member"}`, ordinary, 201)
	request("GET", base+"/members", "", ordinary, 200)
}
