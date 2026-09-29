package httpservice

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"gatehouse/apischema"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

//go:embed transcript_schema.json
var transcriptSchemaJSON []byte

func TestTranscriptProjectionMatchesAPISchema(t *testing.T) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(transcriptSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	const schemaID = "https://gatehouse.test/transcript-schema.json"
	document.(map[string]any)["$id"] = schemaID
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaID, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatal(err)
	}
	file := map[string]interface{}{"id": "sfi_test", "name": "report.txt", "size": 12, "fingerprint": "sha256:test"}
	entries := []database.SessionEventTreeEntry{
		{Event: model.SessionEvent{Kind: model.SessionEventKindMessageText, Payload: map[string]interface{}{"text": "Hello", "attachments": []interface{}{file}, "agents": []string{"wag_test"}}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindAgentRequest, Payload: map[string]interface{}{"agent": "wag_test"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindAgentSuccess, Payload: map[string]interface{}{"text": "Reply", "attachments": []interface{}{file}}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindAgentFailure, Payload: map[string]interface{}{"code": "failed"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindThinkingRequest, Payload: map[string]interface{}{"turn": 0}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindThinkingUpdate, Payload: map[string]interface{}{"reason": "rate_limit", "until": "2026-01-01T00:00:00Z"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindThinkingSuccess, Payload: map[string]interface{}{}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindThinkingFailure, Payload: map[string]interface{}{"code": "cancelled"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindToolRequest, Payload: map[string]interface{}{"name": "lisp", "reason": "Inspect", "call_id": "call", "code": "(+ 1 2)"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindToolSuccess, Payload: map[string]interface{}{"name": "lisp", "call_id": "call", "output": "3"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindToolFailure, Payload: map[string]interface{}{"name": "lisp", "call_id": "call", "output": "failure", "code": "execution_failed"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindToolFailure, Payload: map[string]interface{}{"name": "lisp", "call_id": "old-call", "output": "failure without code"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindApprovalRequest, Payload: map[string]interface{}{"description": "Approve"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindApprovalSuccess, Payload: map[string]interface{}{}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindApprovalFailure, Payload: map[string]interface{}{"code": "rejected"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindInputRequest, Payload: map[string]interface{}{"description": "Review", "form": map[string]interface{}{"private": true}}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindInputSuccess, Payload: map[string]interface{}{"result": map[string]interface{}{"private": true}}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindInputFailure, Payload: map[string]interface{}{"code": "cancelled"}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindCancelRequest, Payload: map[string]interface{}{}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindCancelSuccess, Payload: map[string]interface{}{}}},
		{Event: model.SessionEvent{Kind: model.SessionEventKindCancelFailure, Payload: map[string]interface{}{"code": "already_completed"}}},
		{Event: model.SessionEvent{Kind: "gateway.notice", Payload: map[string]interface{}{"text": "unpublished"}}},
	}
	for index := range entries {
		entries[index].Event.Ref = model.SessionEventRef{Id: "sev_test", Session: model.SessionRef{Id: "ses_test", Workspace: model.WorkspaceRef{Id: "wsp_test"}}}
		entries[index].Event.CreatedAt = "2026-01-01T00:00:00.000Z"
	}
	sessionEventTranscriptProject(entries)
	for _, entry := range entries {
		encoded, err := json.Marshal(entry.Event)
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(value); err != nil {
			t.Errorf("projected %s event does not match API schema: %v", entry.Event.Kind, err)
		}
		codecErr, wire := transcriptEventWire(entry.Event)
		if codecErr != nil {
			t.Errorf("encode %s transcript event: %v", entry.Event.Kind, codecErr)
			continue
		}
		if err := schema.Validate(wire); err != nil {
			t.Errorf("Go-encoded %s event does not match API schema: %v", entry.Event.Kind, err)
		}
		if !reflect.DeepEqual(wire, value) {
			t.Errorf("Go-encoded %s event = %#v, want %#v", entry.Event.Kind, wire, value)
		}
	}
	if entries[15].Event.Payload["form"] != nil || entries[16].Event.Payload["result"] != nil || entries[9].Event.Payload["output"] != nil {
		t.Fatal("transcript exposed private input or tool data")
	}
	if entries[21].Event.Kind != "other" || entries[21].Event.Payload["original_kind"] != "gateway.notice" {
		t.Fatalf("unrecognized event = %#v", entries[21].Event)
	}
	root := &sessionEventTreeResponse{Event: entries[0].Event, Children: []*sessionEventTreeResponse{{Event: entries[1].Event, Children: []*sessionEventTreeResponse{}}}}
	if err, trees := transcriptEventTrees([]*sessionEventTreeResponse{root}); err != nil || len(trees) != 1 || len(trees[0].Children) != 1 {
		t.Fatalf("nested transcript tree = (%#v, %v)", trees, err)
	}
	invalid := entries[3].Event
	invalid.Payload = map[string]interface{}{}
	if err, _ := transcriptEventWire(invalid); err == nil {
		t.Fatal("transcript codec accepted an agent failure without a code")
	}
}

func TestTranscriptServesToolFailureWithoutCode(t *testing.T) {
	fixture := newInputTestFixture(t)
	ctx := context.Background()
	err, input := fixture.store.SessionEventGet(ctx, fixture.input)
	if err != nil || input == nil || input.Parent == nil || input.AuthorAgent == nil {
		t.Fatalf("input request = (%#v, %v)", input, err)
	}
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	failure := model.SessionEvent{
		Ref: model.SessionEventRef{Session: fixture.input.Session, Id: id}, Parent: input.Parent,
		Kind: model.SessionEventKindToolFailure, AuthorAgent: input.AuthorAgent,
		Payload: map[string]interface{}{"name": "lisp", "call_id": "call-1", "output": "legacy failure"},
	}
	if err, _ := fixture.store.SessionEventsCreate(ctx, failure); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/events?view=transcript"
	response := inputTestRequest(fixture.handler, http.MethodGet, path, fixture.login, "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET transcript = %d %s", response.Code, response.Body.String())
	}
	var trees []*sessionEventTreeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &trees); err != nil {
		t.Fatal(err)
	}
	var projected *model.SessionEvent
	for _, tree := range trees {
		for _, child := range tree.Children {
			for _, grandchild := range child.Children {
				if grandchild.Event.Ref == failure.Ref {
					projected = &grandchild.Event
				}
			}
		}
	}
	if projected == nil || projected.Kind != model.SessionEventKindToolFailure || projected.Payload["output"] != "legacy failure" {
		t.Fatalf("projected code-less failure = %#v", projected)
	}
	if _, hasCode := projected.Payload["code"]; hasCode {
		t.Fatalf("transcript invented a failure code: %#v", projected.Payload)
	}
}

func TestTranscriptGoCodecRoundTrip(t *testing.T) {
	for _, input := range []string{
		`{"created_at":"2026-01-01T00:00:00.000Z","kind":"thinking.success","payload":{},"ref":{"id":"sev_test","session":{"id":"ses_test","workspace":{"id":"wsp_test"}}}}`,
		`{"created_at":"2026-01-01T00:00:00.000Z","kind":"message.text","payload":{"attachments":[{"id":"sfi_test","name":"report.txt","size":9007199254740993,"fingerprint":"sha256:test"}]},"ref":{"id":"sev_test","session":{"id":"ses_test","workspace":{"id":"wsp_test"}}}}`,
	} {
		decoder := json.NewDecoder(bytes.NewReader([]byte(input)))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		if err, event := apischema.DecodeTranscriptEvent(decoded); err != nil {
			t.Fatalf("decode transcript event: %v", err)
		} else if err, encoded := apischema.EncodeTranscriptEvent(event); err != nil {
			t.Fatalf("encode transcript event: %v", err)
		} else if !reflect.DeepEqual(encoded, decoded) {
			t.Fatalf("round-trip transcript event = %#v, want %#v", encoded, decoded)
		}
	}
}
