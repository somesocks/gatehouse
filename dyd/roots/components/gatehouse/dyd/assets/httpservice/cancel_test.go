package httpservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type cancellationTestDispatcher struct {
	store  *database.Store
	calls  int
	reject bool
}

func (*cancellationTestDispatcher) Reconcile() error { return nil }

func (dispatcher *cancellationTestDispatcher) CancelReply(ctx context.Context, request model.SessionEvent) (error, model.SessionEvent) {
	dispatcher.calls++
	if dispatcher.reject {
		return database.ErrSessionReplyAlreadyCompleted, model.SessionEvent{}
	}
	return dispatcher.store.SessionEventsCreate(ctx, request)
}

func TestSessionMessageCancelUsesReplyCancellationDispatcher(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	principalRef, _ := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	principal := model.Principal{Ref: principalRef, Enabled: true}
	session := model.SessionRef{Workspace: workspaces["engineering"], Id: "ses_00000000000000000000000000"}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = ? AND alias = 'assistant'`, session.Workspace.Id).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	messageID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionMessagesCreate(ctx, model.SessionEvent{
		Ref: model.SessionEventRef{Session: session, Id: messageID}, Kind: "message.text", AuthorPrincipal: &principal,
		Payload: map[string]interface{}{"text": "hello", "agents": []string{agentID}},
	}); err != nil {
		t.Fatal(err)
	}
	err, requests := store.SessionEventsGet(ctx, session)
	if err != nil || len(requests) != 2 {
		t.Fatalf("agent request events = (%#v, %v)", requests, err)
	}
	var agentRequest model.SessionEventRef
	for _, event := range requests {
		if event.Kind == "agent.request" {
			agentRequest = event.Ref
		}
	}
	if agentRequest.Id == "" {
		t.Fatalf("no agent request in events %#v", requests)
	}
	path := "/api/v1/workspaces/" + session.Workspace.Id + "/sessions/" + session.Id + "/messages/" + agentRequest.Id + "/cancel"
	dispatcher := &cancellationTestDispatcher{store: store}
	handler := HandlerWithReplyDispatcher(config.HTTPService{API: true}, store, dispatcher, tokens)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identity":"gatehouse:alice","password":"correct password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	var credentials loginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || dispatcher.calls != 1 {
		t.Fatalf("cancel response = %d %s, dispatcher calls = %d", response.Code, response.Body.String(), dispatcher.calls)
	}
	var cancellation model.SessionEvent
	if err := json.Unmarshal(response.Body.Bytes(), &cancellation); err != nil {
		t.Fatal(err)
	}
	if cancellation.Kind != "cancel.request" || cancellation.Parent == nil || *cancellation.Parent != agentRequest || cancellation.AuthorPrincipal == nil || cancellation.AuthorPrincipal.Ref != principalRef {
		t.Fatalf("cancellation = %#v", cancellation)
	}
	err, stored := store.SessionEventGet(ctx, cancellation.Ref)
	if err != nil || stored == nil || stored.Kind != "cancel.request" {
		t.Fatalf("stored cancellation = (%#v, %v)", stored, err)
	}
	dispatcher.reject = true
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, request)
	if rejected.Code != http.StatusConflict || dispatcher.calls != 2 {
		t.Fatalf("already completed cancellation = %d %s, dispatcher calls = %d", rejected.Code, rejected.Body.String(), dispatcher.calls)
	}
}
