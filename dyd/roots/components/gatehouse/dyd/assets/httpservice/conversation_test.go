package httpservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gatehouse/apischema"
	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/model"
)

func TestConversationBranchAPI(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := t.Context()
	person, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: person, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspaces["engineering"], Id: "ses_00000000000000000000000000"}
	base := "/api/v1/workspaces/" + session.Workspace.Id + "/sessions/" + session.Id
	handler := HandlerWithReplyDispatcher(config.HTTPService{API: true}, store, messageTestDispatcher{store}, tokens)
	call := func(method, path, body string, status int) *httptest.ResponseRecorder {
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
	// The branch uses the same public file metadata projection as the transcript.
	created := call(http.MethodPost, base+"/files", `{"name":"note.txt","media_type":"text/plain"}`, http.StatusCreated)
	var upload sessionFileCreateResponse
	if err := json.Unmarshal(created.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, upload.UploadURL, "hello", http.StatusNoContent)
	call(http.MethodPost, base+"/files/"+upload.File.Ref.Id+"/finish", "", http.StatusOK)
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id=?`, session.Workspace.Id).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	refs := map[string]model.SessionEventRef{}
	sequence := 0
	add := func(name, parent, kind string, agent bool, payload map[string]any, at int) {
		t.Helper()
		sequence++
		event := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_%026d", sequence)}, Kind: kind, Payload: payload, CreatedAt: fmt.Sprintf("2026-01-01T00:00:%02d.000Z", at)}
		if parent != "" {
			ref := refs[parent]
			event.Parent = &ref
		}
		if agent {
			event.AuthorAgent = &model.WorkspaceAgentRef{Workspace: session.Workspace, Id: bot}
		} else {
			event.AuthorPrincipal = &model.Principal{Ref: person, Enabled: true}
		}
		if err, _ := store.SessionEventsCreate(ctx, event); err != nil {
			t.Fatal(err)
		}
		refs[name] = event.Ref
	}
	add("a", "", "message.text", false, map[string]any{"text": "Hello", "agents": []string{bot}, "attachments": []string{upload.File.Ref.Id}}, 0)
	add("ar", "a", "agent.request", false, map[string]any{"agent": bot}, 3)
	add("human", "a", "message.text", false, map[string]any{"text": "Human reply"}, 3)
	add("hidden", "a", "agent.request", false, map[string]any{"agent": bot}, 3)
	add("hidden-answer", "hidden", "agent.success", true, map[string]any{"text": "HIDDEN_ANSWER"}, 20)
	add("at", "ar", "tool.request", true, map[string]any{"name": "lisp", "code": "EXECUTION_SECRET"}, 5)
	add("ai", "at", "input.request", true, map[string]any{"description": "Review", "form": map[string]any{"private": "FORM_SECRET"}}, 6)
	add("ais", "ai", "input.success", false, map[string]any{"result": map[string]any{"private": "RESULT_SECRET"}}, 7)
	add("answer", "ar", "agent.success", true, map[string]any{"text": "Done", "reasoning": "REASONING_SECRET", "attachments": []string{upload.File.Ref.Id}}, 20)
	add("b", "", "message.text", false, map[string]any{"text": "Second root"}, 1)
	add("br", "b", "agent.request", false, map[string]any{"agent": bot}, 3)
	add("bt", "br", "tool.request", true, map[string]any{"code": "EXECUTION_SECRET"}, 5)
	add("bi", "bt", "input.request", true, map[string]any{"description": "Enter details", "form": map[string]any{"private": "FORM_SECRET"}}, 6)
	add("approval", "bt", "approval.request", true, map[string]any{"description": "Allow?"}, 7)
	if _, err := store.Exec(`INSERT INTO gatehouse_session_input_drafts(workspace,session,input,draft,updated_at) VALUES(?,?,?,?,?)`, session.Workspace.Id, session.Id, refs["bi"].Id, `{"secret":"DRAFT_SECRET"}`, "2026-01-01T00:00:08.000Z"); err != nil {
		t.Fatal(err)
	}
	get := func(path string) map[string]any {
		t.Helper()
		w := call(http.MethodGet, path, "", http.StatusOK)
		for _, secret := range []string{"FORM_SECRET", "RESULT_SECRET", "DRAFT_SECRET", "EXECUTION_SECRET", "REASONING_SECRET"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("branch exposed %s", secret)
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
		decoder.UseNumber()
		var value map[string]any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		if err, _ := apischema.DecodeConversationBranch(value); err != nil {
			t.Fatalf("client contract rejected response: %v", err)
		}
		return value
	}
	first := get(base + "/events/branch?limit=1")
	entries := first["entries"].([]any)
	entry := entries[0].(map[string]any)
	if len(entries) != 1 || entry["has_more_replies"] != true || len(entry["previews"].([]any)) != 2 {
		t.Fatalf("entry = %#v", entry)
	}
	if first["parent"] != nil {
		t.Fatal("root branch has a parent")
	}
	event := entry["event"].(map[string]any)
	attachment := event["payload"].(map[string]any)["attachments"].([]any)[0].(map[string]any)
	if attachment["id"] != upload.File.Ref.Id || attachment["name"] != "note.txt" || attachment["size"] != json.Number("5") || attachment["fingerprint"] == "" {
		t.Fatalf("file metadata = %#v", attachment)
	}
	agents := first["agent_requests"].([]any)
	agent := agents[0].(map[string]any)
	if len(agents) != 1 || agent["state"] != "completed" || agent["outcome"].(map[string]any)["kind"] != "agent.success" {
		t.Fatalf("agent = %#v", agents)
	}
	outcome := agent["outcome"].(map[string]any)
	if outcome["parent"].(map[string]any)["id"] != refs["ar"].Id || outcome["author_agent"].(map[string]any)["alias"] != "assistant" {
		t.Fatalf("lost actual parent or attribution: %#v", outcome)
	}
	interactions := first["interactions"].([]any)
	if len(interactions) != 1 || interactions[0].(map[string]any)["response"].(map[string]any)["kind"] != "input.success" {
		t.Fatalf("interactions = %#v", interactions)
	}
	cursor := first["next_cursor"].(string)
	second := get(base + "/events/branch?limit=1&cursor=" + url.QueryEscape(cursor))
	if second["next_cursor"] != nil || second["agent_requests"].([]any)[0].(map[string]any)["state"] != "waiting" || len(second["interactions"].([]any)) != 2 {
		t.Fatalf("second page = %#v", second)
	}
	direct := get(base + "/events/branch?parent=" + refs["at"].Id)
	if direct["parent"].(map[string]any)["id"] != refs["at"].Id || len(direct["interactions"].([]any)) != 1 || len(direct["agent_requests"].([]any)) != 0 {
		t.Fatalf("direct form = %#v", direct)
	}
	empty := get(base + "/events/branch?parent=" + refs["answer"].Id)
	if len(empty["entries"].([]any)) != 0 || len(empty["interactions"].([]any)) != 0 {
		t.Fatalf("empty branch = %#v", empty)
	}
	for _, query := range []string{"limit=0", "limit=101", "limit=x", "parent=bad", "cursor=bad", "parent=" + refs["a"].Id + "&cursor=" + url.QueryEscape(cursor)} {
		call(http.MethodGet, base+"/events/branch?"+query, "", http.StatusBadRequest)
	}
	call(http.MethodGet, base+"/events/branch?parent=sev_00000000000000000000000999", "", http.StatusNotFound)
	other := session
	other.Id = "ses_00000000000000000000000001"
	otherEvent := model.SessionEvent{Ref: model.SessionEventRef{Session: other, Id: "sev_00000000000000000000000998"}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: person, Enabled: true}, Payload: map[string]any{"text": "Other session"}}
	if err, _ := store.SessionEventsCreate(ctx, otherEvent); err != nil {
		t.Fatal(err)
	}
	call(http.MethodGet, base+"/events/branch?parent="+otherEvent.Ref.Id, "", http.StatusNotFound)
	call(http.MethodGet, strings.Replace(base, session.Id, other.Id, 1)+"/events/branch?cursor="+url.QueryEscape(cursor), "", http.StatusBadRequest)
	call(http.MethodPost, base+"/events/branch", "", http.StatusMethodNotAllowed)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, base+"/events/branch", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", unauthenticated.Code)
	}
	// Revocation takes effect on every read, including requests with valid cursors.
	if _, err := store.Exec(`UPDATE gatehouse_session_grants SET enabled=FALSE WHERE workspace=? AND session=?`, session.Workspace.Id, session.Id); err != nil {
		t.Fatal(err)
	}
	call(http.MethodGet, base+"/events/branch?cursor="+url.QueryEscape(cursor), "", http.StatusNotFound)
}
