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

func TestSessionParticipantAPI(t *testing.T) {
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
	username, password := "participant-bob", "password"
	err, principal := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, ordinary := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: principal.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["engineering"]
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
	base := "/api/v1/workspaces/" + workspace.Id + "/sessions/ses_00000000000000000000000000"
	request("GET", base+"/participants", "", "", 401)
	request("GET", base+"/participants", "", ordinary, 404) // Workspace manager is not a chat manager.
	request("POST", base+"/grants", `{"principal":"`+bob+`","role":"manager"}`, ordinary, 404)
	for _, query := range []string{"?kind=invalid", "?cursor=invalid", "?limit=101"} {
		request("GET", base+"/participants"+query, "", manager, 400)
	}
	for _, body := range []string{`{}`, `{"principal":"` + bob + `","group":"invalid","role":"member"}`, `{"principal":"` + bob + `","role":"owner"}`, `{"principal":"` + bob + `","role":"member","extra":true}`} {
		request("POST", base+"/grants", body, manager, 400)
	}
	created := request("POST", base+"/grants", `{"principal":"`+bob+`","role":"member"}`, manager, 201)
	var grant database.SessionParticipantGrant
	if err := json.Unmarshal(created.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	request("POST", base+"/grants", `{"principal":"`+bob+`","role":"member"}`, manager, 409)
	roster := request("GET", base+"/participants", "", ordinary, 200)
	var page struct {
		Participants []database.SessionParticipant `json:"participants"`
		CanManage    bool                          `json:"can_manage_participants"`
	}
	if err := json.Unmarshal(roster.Body.Bytes(), &page); err != nil || len(page.Participants) != 2 || page.CanManage {
		t.Fatalf("member roster = %#v %v", page, err)
	}
	for _, suffix := range []string{"/principal-options", "/group-options"} {
		request("GET", base+suffix, "", ordinary, 403)
		request("GET", base+suffix, "", manager, 200)
	}
	grantPath := base + "/grants/" + grant.ID
	request("PATCH", grantPath, `{"role":"manager","expected_revision":1}`, ordinary, 403)
	request("PATCH", grantPath, `{"role":"contributor"}`, manager, 400)
	request("PATCH", grantPath, `{"role":"contributor","expected_revision":1}`, manager, 200)
	request("PATCH", grantPath, `{"enabled":false,"expected_revision":1}`, manager, 409)
	request("POST", base+"/grants", `{"principal":"`+alice.Id+`","role":"member"}`, ordinary, 403)
	group := groupID(t, ctx, store, "engineering", "developers")
	request("POST", base+"/grants", `{"group":"`+group+`","role":"contributor"}`, manager, 201)
	groupPage := request("GET", base+"/participants?kind=group", "", ordinary, 200)
	if err := json.Unmarshal(groupPage.Body.Bytes(), &page); err != nil || len(page.Participants) != 1 || page.Participants[0].ID != group {
		t.Fatalf("group roster = %#v %v", page, err)
	}
	other := strings.Replace(grantPath, workspace.Id, workspaces["operations"].Id, 1)
	request("PATCH", other, `{"enabled":false,"expected_revision":2}`, manager, 404)
	request("PATCH", grantPath, `{"enabled":false,"expected_revision":2}`, manager, 200)
	request("GET", base+"/participants", "", ordinary, 404)
	request("POST", base+"/grants", `{"principal":"`+bob+`","role":"contributor"}`, manager, 201)
	request("GET", base+"/participants", "", ordinary, 200)
	request("PATCH", base+"/grants/sgr_00000000000000000000000000", `{"enabled":false,"expected_revision":1}`, manager, 409)
}
