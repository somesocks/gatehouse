package httpservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestMessageReplies(t *testing.T) {
	ctx := context.Background()
	tokens, store, workspaces := testBearerTokens(t)
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspaces["engineering"], Id: "ses_00000000000000000000000000"}
	base := "/api/v1/workspaces/" + session.Workspace.Id + "/sessions/" + session.Id
	handler := HandlerWithReplyDispatcher(config.HTTPService{API: true}, store, messageTestDispatcher{store}, tokens)
	post := func(body string, status int) model.SessionEvent {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, base+"/messages", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("POST message = %d, want %d: %s", w.Code, status, w.Body.String())
		}
		var event model.SessionEvent
		if status == http.StatusAccepted {
			if err := json.Unmarshal(w.Body.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
		}
		return event
	}
	root := post(`{"text":"Root message","targets":[]}`, http.StatusAccepted)
	if root.Parent != nil {
		t.Fatal("omitted parent did not create a root message")
	}
	if event := post(`{"text":"Another root","parent":null}`, http.StatusAccepted); event.Parent != nil {
		t.Fatal("null parent did not create a root message")
	}
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id=?`, session.Workspace.Id).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	reply := post(fmt.Sprintf(`{"text":"Reply to a user","parent":%q,"targets":[{"kind":"agent","id":%q}]}`, root.Ref.Id, bot), http.StatusAccepted)
	if reply.Parent == nil || *reply.Parent != root.Ref {
		t.Fatalf("reply parent = %#v, want %#v", reply.Parent, root.Ref)
	}
	err, stored := store.SessionEventGet(ctx, reply.Ref)
	if err != nil || stored == nil || stored.Parent == nil || *stored.Parent != root.Ref {
		t.Fatalf("persisted reply = %#v: %v", stored, err)
	}
	err, queued := store.SessionEventChildGet(ctx, reply.Ref, model.SessionEventKindAgentRequest)
	if err != nil || queued == nil || queued.Payload["agent"] != bot || queued.Parent == nil || *queued.Parent != reply.Ref {
		t.Fatalf("reply did not dispatch its own agent request: %#v %v", queued, err)
	}
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	answer := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &queued.Ref, Kind: model.SessionEventKindAgentSuccess, AuthorAgent: &model.WorkspaceAgentRef{Workspace: session.Workspace, Id: bot}, Payload: map[string]any{"text": "Agent answer"}}
	if err, _ := store.SessionEventsCreate(ctx, answer); err != nil {
		t.Fatal(err)
	}
	followup := post(fmt.Sprintf(`{"text":"Reply to the agent","parent":%q,"agents":[%q]}`, answer.Ref.Id, bot), http.StatusAccepted)
	if followup.Parent == nil || *followup.Parent != answer.Ref {
		t.Fatalf("agent-answer reply parent = %#v", followup.Parent)
	}
	human := post(fmt.Sprintf(`{"text":"Reply without recipients","parent":%q}`, reply.Ref.Id), http.StatusAccepted)
	err, inherited := store.SessionEventChildGet(ctx, human.Ref, model.SessionEventKindAgentRequest)
	if err != nil || human.Parent == nil || *human.Parent != reply.Ref || inherited != nil {
		t.Fatalf("nested reply inherited recipients or lost its parent: %#v %#v %v", human, inherited, err)
	}
	// Existing IDs from a different session or workspace must not be accepted
	// merely because the caller can access their containing sessions.
	otherSession := session
	otherSession.Id = "ses_00000000000000000000000001"
	otherWorkspace := model.SessionRef{Workspace: workspaces["operations"], Id: "ses_00000000000000000000000002"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: otherWorkspace, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	invalid := []string{"", "not-an-event-id", "sev_00000000000000000000000009"}
	for _, scope := range []model.SessionRef{otherSession, otherWorkspace} {
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			t.Fatal(err)
		}
		event := model.SessionEvent{Ref: model.SessionEventRef{Session: scope, Id: id}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]any{"text": "Other scope"}}
		if err, _ := store.SessionEventsCreate(ctx, event); err != nil {
			t.Fatal(err)
		}
		invalid = append(invalid, id)
	}
	counts := func() [2]int {
		t.Helper()
		var result [2]int
		if err := store.QueryRow(`SELECT (SELECT count(*) FROM gatehouse_session_events),(SELECT count(*) FROM gatehouse_activity_events)`).Scan(&result[0], &result[1]); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := counts()
	for _, parent := range invalid {
		post(fmt.Sprintf(`{"text":"Invalid reply","parent":%q,"agents":[%q]}`, parent, bot), http.StatusBadRequest)
	}
	if after := counts(); after != before {
		t.Fatalf("rejected replies wrote events/activity: %v -> %v", before, after)
	}
}
