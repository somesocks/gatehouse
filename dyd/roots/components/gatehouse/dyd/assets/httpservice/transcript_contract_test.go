package httpservice

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"gatehouse/database"
	"gatehouse/model"
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
	}
	if entries[14].Event.Payload["form"] != nil || entries[15].Event.Payload["result"] != nil || entries[9].Event.Payload["output"] != nil {
		t.Fatal("transcript exposed private input or tool data")
	}
	if entries[20].Event.Kind != "other" || entries[20].Event.Payload["original_kind"] != "gateway.notice" {
		t.Fatalf("unrecognized event = %#v", entries[20].Event)
	}
}
