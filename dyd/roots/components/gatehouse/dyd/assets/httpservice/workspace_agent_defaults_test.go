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

func TestSystemWorkspaceAgentDefaultsAreIndependent(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	principal, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces["engineering"].Id
	var assistantID, modelID string
	if err := store.QueryRow(`SELECT id, model_id FROM gatehouse_workspace_agents WHERE workspace_id = ? AND alias = 'assistant'`, workspace).Scan(&assistantID, &modelID); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	decode := func(w *httptest.ResponseRecorder) database.SystemWorkspaceAgent {
		t.Helper()
		var bot database.SystemWorkspaceAgent
		if err := json.Unmarshal(w.Body.Bytes(), &bot); err != nil {
			t.Fatal(err)
		}
		return bot
	}
	base := "/api/v1/system/workspace-agents/" + workspace
	create := func(alias string, isDefault bool) database.SystemWorkspaceAgent {
		t.Helper()
		return decode(request("POST", base, fmt.Sprintf(`{"alias":%q,"model":%q,"default":%t,"enabled":true}`, alias, modelID, isDefault), 200))
	}
	reviewer := create("reviewer", true)
	private := create("private", false)
	if !reviewer.Default || private.Default {
		t.Fatal("creation did not preserve requested flags")
	}
	update := func(bot database.SystemWorkspaceAgent, isDefault, enabled bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		return request("PATCH", base+"/"+bot.ID, fmt.Sprintf(`{"model":%q,"default":%t,"enabled":%t,"expected_revision":%d}`, modelID, isDefault, enabled, bot.Revision), status)
	}
	private = decode(update(private, true, true, 200))
	for _, id := range []string{assistantID, reviewer.ID} {
		peer := decode(request("GET", base+"/"+id, "", 200))
		if !peer.Default || peer.Revision != 1 {
			t.Fatalf("changing a different default changed peer: %#v", peer)
		}
	}
	reviewer = decode(update(reviewer, false, true, 200))
	if reviewer.Default || reviewer.Revision != 2 {
		t.Fatalf("remove one default = %#v", reviewer)
	}
	stale := private
	private = decode(update(private, true, false, 200))
	if private.Default || private.Enabled {
		t.Fatalf("disable own default = %#v", private)
	}
	update(stale, false, true, 409)
	assistant := decode(request("GET", base+"/"+assistantID, "", 200))
	if !assistant.Default || assistant.Revision != 1 {
		t.Fatalf("other edits cleared initial default: %#v", assistant)
	}
	request("POST", base, fmt.Sprintf(`{"alias":"assistant","model":%q,"default":true,"enabled":true}`, modelID), 409)
	assistant = decode(request("GET", base+"/"+assistantID, "", 200))
	if !assistant.Default || assistant.Revision != 1 {
		t.Fatalf("failed creation changed default: %#v", assistant)
	}
}
