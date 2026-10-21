package httpservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

// HTTP unit tests isolate routing/authorization from the durable runtime. The
// runtime's atomic persistence and delivery are covered by agent tests.
type messageTestDispatcher struct {
	store *database.Store
}

func (dispatcher messageTestDispatcher) CreateMessage(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	return dispatcher.store.SessionMessagesCreate(ctx, event)
}

func (dispatcher messageTestDispatcher) RespondToApproval(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	return dispatcher.store.SessionApprovalResponseCreate(ctx, event)
}

func TestMessageSubmissionRequiresWorkflowDispatcher(t *testing.T) {
	ctx := context.Background()
	tokens, store, workspaces := testBearerTokens(t)
	principal, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, login := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspaces["engineering"].Id+"/sessions/ses_00000000000000000000000000/messages", strings.NewReader(`{"text":"Hello"}`))
	request.Header.Set("Authorization", "Bearer "+login)
	response := httptest.NewRecorder()
	Handler(config.HTTPService{API: true}, store, tokens).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing runtime accepted message: %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("missing runtime persisted undeliverable work: %d (%v)", count, err)
	}
}
