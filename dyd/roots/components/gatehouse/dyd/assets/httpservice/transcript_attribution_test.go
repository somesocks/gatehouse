package httpservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestTranscriptHydratesBotAttributionIndependentlyOfParticipation(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	workspace := workspaces["engineering"]
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	var modelID string
	if err := store.QueryRow(`SELECT model_id FROM gatehouse_workspace_agents WHERE workspace_id = ?`, workspace.Id).Scan(&modelID); err != nil {
		t.Fatal(err)
	}
	original, renamed, secret := "Original assistant", "Archived assistant", "PRIVATE_BOT_CONFIGURATION"
	err, bot := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "archived", ModelID: modelID, Label: &original, SystemPrompt: &secret, Default: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	err, hidden := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "undisclosed-bot", ModelID: modelID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	handler := HandlerWithReplyDispatcher(config.HTTPService{API: true}, store, messageTestDispatcher{store}, tokens)
	request := func(method, path, body, bearer string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	chat := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	base := "/api/v1/workspaces/" + workspace.Id + "/sessions/" + chat.Id
	var message model.SessionEvent
	if err := json.Unmarshal(request("POST", base+"/messages", fmt.Sprintf(`{"text":"Hello","targets":[{"kind":"agent","id":%q}]}`, bot.ID), token, 202).Body.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	err, queued := store.SessionEventChildGet(ctx, message.Ref, model.SessionEventKindAgentRequest)
	if err != nil || queued == nil {
		t.Fatalf("queued request = %#v %v", queued, err)
	}
	type tree struct {
		Event    map[string]any `json:"event"`
		Children []tree         `json:"children"`
	}
	transcript := func(query string) map[string]map[string]any {
		t.Helper()
		response := request("GET", base+"/events?view=transcript&"+query, "", token, 200)
		for _, private := range []string{secret, modelID, hidden.ID, hidden.Alias} {
			if strings.Contains(response.Body.String(), private) {
				t.Fatalf("transcript leaked unreferenced/private bot data %q", private)
			}
		}
		var trees []tree
		if err := json.Unmarshal(response.Body.Bytes(), &trees); err != nil {
			t.Fatal(err)
		}
		result := map[string]map[string]any{}
		var visit func([]tree)
		visit = func(nodes []tree) {
			for _, node := range nodes {
				result[node.Event["ref"].(map[string]any)["id"].(string)] = node.Event
				visit(node.Children)
			}
		}
		visit(trees)
		return result
	}
	page := transcript("limit=1")
	target := page[queued.Ref.Id]["target_agent"].(map[string]any)
	if target["label"] != original || target["enabled"] != true || page[queued.Ref.Id]["author_agent"] != nil {
		t.Fatalf("queued target attribution = %#v", page[queued.Ref.Id])
	}
	ref := model.WorkspaceAgentRef{Workspace: workspace, Id: bot.ID}
	replyID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	reply := model.SessionEvent{Ref: model.SessionEventRef{Session: chat, Id: replyID}, Parent: &queued.Ref, Kind: model.SessionEventKindAgentSuccess, AuthorAgent: &ref, Payload: map[string]interface{}{"text": "Historical reply"}}
	if err, _ := store.SessionEventsCreate(ctx, reply); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, bot.ID, 1, database.SystemWorkspaceAgentInput{ModelID: modelID, Label: &renamed, SystemPrompt: &secret, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	err, roster, _ := store.SessionParticipantsSearch(ctx, chat, alice, database.SessionParticipantSearch{Kind: "agent", Limit: 10}, false)
	if err != nil || len(roster) != 1 {
		t.Fatalf("bot roster = %#v %v", roster, err)
	}
	disabled := false
	grant := roster[0].Grants[0]
	if err, _ := store.SessionParticipantGrantUpdate(ctx, chat, alice, grant.ID, nil, &disabled, grant.Revision); err != nil {
		t.Fatal(err)
	}
	page = transcript("limit=1")
	author := page[replyID]["author_agent"].(map[string]any)
	if author["label"] != renamed || author["alias"] != "archived" || author["enabled"] != false || author["ref"].(map[string]any)["id"] != bot.ID {
		t.Fatalf("historical author attribution = %#v", author)
	}
	if _, polluted := author["id"]; polluted {
		t.Fatal("display metadata was mixed into an identity reference")
	}
	if page[queued.Ref.Id]["target_agent"].(map[string]any)["label"] != renamed {
		t.Fatal("request target did not hydrate its current display name")
	}
	payload := page[message.Ref.Id]["payload"].(map[string]any)
	if payload["targets"].([]any)[0].(map[string]any)["name"] != original {
		t.Fatal("canonical send-time recipient label was overwritten")
	}
	if err, active := store.SessionAgentParticipantGet(ctx, chat, ref); err != nil || active {
		t.Fatalf("historical attribution granted participation: %t %v", active, err)
	}
	if err, directory := store.WorkspaceAgentsGet(ctx, workspace, alice); err != nil {
		t.Fatal(err)
	} else {
		for _, item := range directory {
			if item.ID == bot.ID {
				t.Fatal("history made removed private bot discoverable")
			}
		}
	}
	at, err := typed_id.Timestamp(typed_id.SessionEvent, message.Ref.Id)
	if err != nil {
		t.Fatal(err)
	}
	legacyID, err := typed_id.NewAt(typed_id.SessionEvent, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	legacy := model.SessionEvent{Ref: model.SessionEventRef{Session: chat, Id: legacyID}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "Legacy message", "agents": []string{bot.ID}}}
	if err, _ := store.SessionEventsCreate(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	page = transcript("limit=1&after_id=" + message.Ref.Id)
	if len(page) != 1 {
		t.Fatalf("attribution lookup expanded the event page: %#v", page)
	}
	payload = page[legacyID]["payload"].(map[string]any)
	if _, exists := payload["agents"]; exists {
		t.Fatal("transcript retained legacy delivery fields")
	}
	legacyTarget := payload["targets"].([]any)[0].(map[string]any)
	if legacyTarget["alias"] != "archived" || legacyTarget["name"] != renamed {
		t.Fatalf("legacy target attribution = %#v", legacyTarget)
	}
	if err, stored := store.SessionEventGet(ctx, legacy.Ref); err != nil || stored.Payload["agents"] == nil || stored.Payload["targets"] != nil {
		t.Fatalf("projection changed the stored event: %#v %v", stored, err)
	}
	if err, stored := store.SessionEventGet(ctx, reply.Ref); err != nil || stored.AuthorAgent == nil || *stored.AuthorAgent != ref {
		t.Fatalf("projection changed stored author identity: %#v %v", stored, err)
	}
	request("GET", base+"/agents", "", token, 404) // No separate historical-bot lookup API.
	request("GET", base+"/events?view=transcript", "", "", 401)
	var bob string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	username, password := "transcript-outsider", "password"
	err, outsider := store.SystemPrincipalIdentityCreate(ctx, bob, database.SystemPrincipalIdentityInput{Username: &username, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, bob, authz.Member); err != nil {
		t.Fatal(err)
	}
	err, outsiderToken := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: model.PrincipalRef{Id: bob}, Enabled: true}, Identity: outsider.Identities[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	request("GET", base+"/events?view=transcript", "", outsiderToken, 404)
}
