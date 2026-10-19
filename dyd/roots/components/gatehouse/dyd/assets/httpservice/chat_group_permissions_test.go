package httpservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestChatGroupInvitationAndDiscoveryAPI(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	workspace := workspaces["engineering"]
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, aliceToken := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	var bob, assistant, modelID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id, model_id FROM gatehouse_workspace_agents WHERE workspace_id = ?`, workspace.Id).Scan(&assistant, &modelID); err != nil {
		t.Fatal(err)
	}
	username, password := "partition-bob", "password"
	err, person := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	err, bobToken := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: person.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Contributor); err != nil {
		t.Fatal(err)
	}
	developers := groupID(t, ctx, store, "engineering", "developers")
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, developers, assistant); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, assistant, 1, database.SystemWorkspaceAgentInput{ModelID: modelID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	financeAlias := "finance"
	err, finance := store.SystemGroupCreate(ctx, workspace.Id, database.SystemGroupInput{Alias: &financeAlias})
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range []string{alice.Id, bob} {
		if err := store.SystemGroupMemberCreate(ctx, workspace.Id, finance.ID, principal); err != nil {
			t.Fatal(err)
		}
	}
	err, financeBot := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "finance", ModelID: modelID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, finance.ID, financeBot.ID); err != nil {
		t.Fatal(err)
	}
	err, general := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "general", ModelID: modelID, Default: true, Enabled: true})
	if err != nil {
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
	grant := func(group string) database.SessionParticipantGrant {
		t.Helper()
		var grant database.SessionParticipantGrant
		if err := json.Unmarshal(request("POST", base+"/grants", fmt.Sprintf(`{"group":%q,"role":"contributor"}`, group), aliceToken, 201).Body.Bytes(), &grant); err != nil {
			t.Fatal(err)
		}
		return grant
	}
	engineeringGrant, financeGrant := grant(developers), grant(finance.ID)
	// Alice invited Finance while a member, then left that group. Existing chat
	// interaction remains available, but it does not confer new invite authority.
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, finance.ID, alice.Id, false, 1); err != nil {
		t.Fatal(err)
	}
	assertAgents := func(path, token string, want ...string) {
		t.Helper()
		var agents []workspaceAgentResponse
		if err := json.Unmarshal(request("GET", path, "", token, 200).Body.Bytes(), &agents); err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, agent := range agents {
			ids = append(ids, agent.ID)
		}
		sort.Strings(ids)
		sort.Strings(want)
		if !slices.Equal(ids, want) {
			t.Fatalf("%s returned %#v, want %#v", path, ids, want)
		}
	}
	directory := "/api/v1/workspaces/" + workspace.Id + "/agents"
	assertAgents(directory, aliceToken, assistant, general.ID)
	assertAgents(directory, bobToken, financeBot.ID, general.ID)
	request("GET", base+"/events?view=transcript", "", "", 401)
	for _, attempt := range []struct{ token, bot string }{{aliceToken, financeBot.ID}, {bobToken, assistant}} {
		request("POST", base+"/messages", fmt.Sprintf(`{"text":"Hello","targets":[{"kind":"agent","id":%q}]}`, attempt.bot), attempt.token, 202)
	}
	var roster struct {
		Participants []database.SessionParticipant `json:"participants"`
	}
	rosterResponse := request("GET", base+"/participants?kind=agent", "", aliceToken, 200)
	if err := json.Unmarshal(rosterResponse.Body.Bytes(), &roster); err != nil || len(roster.Participants) != 2 {
		t.Fatalf("inherited roster = %#v %v", roster, err)
	}
	if strings.Contains(rosterResponse.Body.String(), engineeringGrant.ID) || strings.Contains(rosterResponse.Body.String(), financeGrant.ID) {
		t.Fatalf("bot roster exposed editable source group grant IDs: %s", rosterResponse.Body.String())
	}
	for _, bot := range roster.Participants {
		if len(bot.Grants) != 0 || len(bot.InheritedFrom) != 1 || bot.InheritedFrom[0].Role != "contributor" || (bot.InheritedFrom[0].Group != finance.ID && bot.InheritedFrom[0].Group != developers) {
			t.Fatalf("inherited participation missing read-only provenance: %#v", bot)
		}
	}
	newChat := request("POST", "/api/v1/workspaces/"+workspace.Id+"/sessions", `{}`, aliceToken, 201)
	var created sessionResponse
	if err := json.Unmarshal(newChat.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	separate := "/api/v1/workspaces/" + workspace.Id + "/sessions/" + created.ID
	for _, subject := range []struct{ kind, id string }{{"principal", bob}, {"group", finance.ID}, {"agent", financeBot.ID}} {
		request("POST", separate+"/grants", fmt.Sprintf(`{%q:%q,"role":"contributor"}`, subject.kind, subject.id), aliceToken, 404)
		request("POST", separate+"/messages", fmt.Sprintf(`{"text":"Hello","targets":[{"kind":%q,"id":%q}]}`, subject.kind, subject.id), aliceToken, 400)
		options := request("GET", separate+"/"+subject.kind+"-options", "", aliceToken, 200)
		if strings.Contains(options.Body.String(), subject.id) {
			t.Fatalf("ineligible subject leaked through options: %s", options.Body.String())
		}
	}
	request("GET", separate+"/events?view=transcript", "", bobToken, 404)
	request("GET", base+"/recipients?kind=agent&scope=out", "", bobToken, 403)
	request("PATCH", base+"/grants/"+engineeringGrant.ID, `{"enabled":false,"expected_revision":1}`, aliceToken, 200)
	assertAgents(directory, bobToken, financeBot.ID, general.ID)
	request("PATCH", base+"/grants/"+financeGrant.ID, `{"enabled":false,"expected_revision":1}`, aliceToken, 200)
	request("PATCH", base+"/grants/"+financeGrant.ID, `{"enabled":true,"expected_revision":2}`, aliceToken, 404)
	request("POST", base+"/grants", fmt.Sprintf(`{"group":%q,"role":"contributor"}`, finance.ID), aliceToken, 404)
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, finance.ID, alice.Id, true, 2); err != nil {
		t.Fatal(err)
	}
	request("PATCH", base+"/grants/"+financeGrant.ID, `{"enabled":true,"expected_revision":2}`, aliceToken, 200)
}
