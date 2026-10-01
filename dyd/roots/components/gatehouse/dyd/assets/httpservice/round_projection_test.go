package httpservice

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestSessionEventsHideInternalRoundSnapshots(t *testing.T) {
	fixture := newInputTestFixture(t)
	ctx := context.Background()
	err, input := fixture.store.SessionEventGet(ctx, fixture.input)
	if err != nil || input == nil {
		t.Fatalf("input fixture = (%#v, %v)", input, err)
	}
	thinkingID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	thinking := model.SessionEvent{Ref: model.SessionEventRef{Session: fixture.input.Session, Id: thinkingID}, Parent: input.Parent,
		Kind: model.SessionEventKindThinkingRequest, AuthorAgent: input.AuthorAgent,
		Payload: map[string]interface{}{"turn": 0, "provider_request": map[string]interface{}{"instructions": "ROUND_INTERNAL_SENTINEL"}}}
	completedID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	completed := model.SessionEvent{Ref: model.SessionEventRef{Session: fixture.input.Session, Id: completedID}, Parent: &thinking.Ref,
		Kind: model.SessionEventKindThinkingSuccess, AuthorAgent: input.AuthorAgent,
		Payload: map[string]interface{}{"tool_requests": []model.SessionEventRef{*input.Parent}}}
	if err, _ := fixture.store.SessionEventsCreateBatch(ctx, []model.SessionEvent{thinking, completed}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/events"
	for _, suffix := range []string{"", "?view=transcript"} {
		response := inputTestRequest(fixture.handler, http.MethodGet, base+suffix, fixture.login, "")
		if response.Code != http.StatusOK {
			t.Fatalf("events view %q: %d %s", suffix, response.Code, response.Body.String())
		}
		for _, internal := range []string{"provider_request", "ROUND_INTERNAL_SENTINEL", "tool_requests"} {
			if strings.Contains(response.Body.String(), internal) {
				t.Fatalf("events view %q exposed %q", suffix, internal)
			}
		}
	}
	err, stored := fixture.store.SessionEventGet(ctx, thinking.Ref)
	if err != nil || stored == nil || stored.Payload["provider_request"] == nil {
		t.Fatalf("public projection altered the persisted snapshot: (%#v, %v)", stored, err)
	}
}
