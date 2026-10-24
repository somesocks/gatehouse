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
	"gatehouse/typed_id"
)

func TestRecipientScopesAndAutomaticBotMembership(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, manager := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	var bob, bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	username, password := "recipient-bob", "password"
	err, person := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, contributor := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: person.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["engineering"]
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Guest); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberCreate(ctx, workspace.Id, groupID(t, ctx, store, "engineering", "developers"), bob); err != nil {
		t.Fatal(err)
	}
	id, _ := typed_id.New(typed_id.Session)
	session := model.SessionRef{Workspace: workspace, Id: id}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, &bob, nil, authz.Member); err != nil {
		t.Fatal(err)
	}
	handler := HandlerWithReplyDispatcher(config.HTTPService{API: true}, store, messageTestDispatcher{store}, tokens)
	base := "/api/v1/workspaces/" + workspace.Id + "/sessions/" + id
	request := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, base+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s = %d: %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	var page struct {
		Targets   []model.MessageTarget `json:"targets"`
		CanManage bool                  `json:"can_manage_participants"`
	}
	before := request("GET", "/recipients?kind=agent", "", contributor, 200)
	if err := json.Unmarshal(before.Body.Bytes(), &page); err != nil || len(page.Targets) != 0 || page.CanManage {
		t.Fatalf("before join = %#v %v", page, err)
	}
	request("GET", "/recipients?kind=agent&scope=out", "", contributor, 403)
	outside := request("GET", "/recipients?kind=agent&scope=out", "", manager, 200)
	if err := json.Unmarshal(outside.Body.Bytes(), &page); err != nil || len(page.Targets) != 1 || !page.CanManage {
		t.Fatalf("outside = %#v %v", page, err)
	}
	body := `{"text":"Hello","targets":[{"kind":"agent","id":"` + bot + `"}]}`
	request("POST", "/messages", body, contributor, 403)
	request("POST", "/messages", body, manager, 202)
	request("POST", "/messages", body, contributor, 202)
	inside := request("GET", "/recipients?kind=agent", "", contributor, 200)
	if err := json.Unmarshal(inside.Body.Bytes(), &page); err != nil || len(page.Targets) != 1 || page.Targets[0].Ref.Id != bot {
		t.Fatalf("joined recipients = %#v %v", page, err)
	}
	roster := request("GET", "/participants?kind=agent", "", manager, 200)
	var participants struct {
		Participants []database.SessionParticipant `json:"participants"`
	}
	if err := json.Unmarshal(roster.Body.Bytes(), &participants); err != nil || len(participants.Participants) != 1 || participants.Participants[0].Grants[0].Role != authz.Member {
		t.Fatalf("joined roster = %#v %v", participants, err)
	}
	grant := participants.Participants[0].Grants[0]
	request("PATCH", "/grants/"+grant.ID, `{"enabled":false,"expected_revision":1}`, manager, 200)
	request("POST", "/messages", body, contributor, 403)
	request("GET", "/agent-options", "", contributor, 403)
	request("GET", "/agent-options", "", manager, 200)
	request("POST", "/grants", `{"agent":"`+bot+`","role":"member"}`, manager, 201)
}
