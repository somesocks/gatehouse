package agent

import (
	"encoding/json"
	"testing"
	"time"

	"gatehouse/model"
)

func TestOpenAICompatibleMessagesReplaysToolHistory(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.PrincipalRef{Id: "user"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	user := event(session, "user", "message.text", &principal, nil, map[string]interface{}{"text": "Find the report."})
	call := event(session, "call", "tool.call", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(report/find)", "reason": "Find the requested report."})
	result := event(session, "result", "tool.result", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "output": "report-42"})
	result.Parent = &call.Ref
	reply := event(session, "reply", "message.text", nil, &agent, map[string]interface{}{"text": "I found it."})
	failedCall := event(session, "failed-call", "tool.call", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-2", "code": "(report/delete \"report-42\")", "reason": "Try to remove a duplicate."})
	failed := event(session, "failed", "tool.failed", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-2", "output": "permission denied"})
	failed.Parent = &failedCall.Ref
	nextUser := event(session, "next-user", "message.text", &principal, nil, map[string]interface{}{"text": "What happened next?"})

	err, messages := openAICompatibleMessages([]model.SessionEvent{user, call, result, reply, failedCall, failed, nextUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 7 {
		t.Fatalf("message count = %d, want 7", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Find the report." {
		t.Fatalf("user message = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || len(messages[1].ToolCalls) != 1 || messages[1].ToolCalls[0].ID != "call-1" || messages[1].ToolCalls[0].Function.Name != "lisp" {
		t.Fatalf("tool call = %#v", messages[1])
	}
	var arguments map[string]string
	if err := json.Unmarshal([]byte(messages[1].ToolCalls[0].Function.Arguments), &arguments); err != nil || arguments["code"] != "(report/find)" || arguments["reason"] != "Find the requested report." {
		t.Fatalf("tool call arguments = %q, %v", messages[1].ToolCalls[0].Function.Arguments, err)
	}
	if messages[2].Role != "tool" || messages[2].ToolCallID != "call-1" || messages[2].Content != "report-42" {
		t.Fatalf("tool result = %#v", messages[2])
	}
	if messages[3].Role != "assistant" || messages[3].Content != "I found it." {
		t.Fatalf("assistant message = %#v", messages[3])
	}
	if messages[4].Role != "assistant" || len(messages[4].ToolCalls) != 1 || messages[4].ToolCalls[0].ID != "call-2" {
		t.Fatalf("failed tool call = %#v", messages[4])
	}
	if messages[5].Role != "tool" || messages[5].ToolCallID != "call-2" || messages[5].Content != "permission denied" {
		t.Fatalf("failed tool output = %#v", messages[5])
	}
	if messages[6].Role != "user" || messages[6].Content != "What happened next?" {
		t.Fatalf("next user message = %#v", messages[6])
	}

	input := openAIResponsesInput(messages)
	if len(input) != len(messages) {
		t.Fatalf("Responses input count = %d, want %d", len(input), len(messages))
	}
	var historicalCall struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(input[1], &historicalCall); err != nil || historicalCall.Type != "function_call" || historicalCall.CallID != "call-1" || historicalCall.Name != "lisp" || historicalCall.Arguments != messages[1].ToolCalls[0].Function.Arguments {
		t.Fatalf("Responses function call = %#v, %v", historicalCall, err)
	}
	var historicalOutput struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal(input[5], &historicalOutput); err != nil || historicalOutput.Type != "function_call_output" || historicalOutput.CallID != "call-2" || historicalOutput.Output != "permission denied" {
		t.Fatalf("Responses function output = %#v, %v", historicalOutput, err)
	}
}

func TestToolOutputCreatedAtFollowsCall(t *testing.T) {
	callCreatedAt := "2099-01-01T00:00:00.000Z"
	outputCreatedAt := toolOutputCreatedAt(callCreatedAt)
	callTime, err := time.Parse("2006-01-02T15:04:05.000Z", callCreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	outputTime, err := time.Parse("2006-01-02T15:04:05.000Z", outputCreatedAt)
	if err != nil || outputTime.Before(callTime.Add(time.Millisecond)) {
		t.Fatalf("toolOutputCreatedAt(%q) = %q, %v", callCreatedAt, outputCreatedAt, err)
	}
}

func TestOpenAICompatibleMessagesReplaysToolBatch(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.PrincipalRef{Id: "user"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	user := event(session, "user", "message.text", &principal, nil, map[string]interface{}{"text": "Check both reports."})
	first := event(session, "first", "tool.call", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(report/get 1)", "reason": "Check the first report.", "batch": float64(0), "position": float64(1)})
	second := event(session, "second", "tool.call", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-2", "code": "(report/get 2)", "reason": "Check the second report.", "batch": float64(0), "position": float64(0)})
	first.Parent = &user.Ref
	second.Parent = &user.Ref
	firstOutput := event(session, "first-output", "tool.result", nil, &agent, map[string]interface{}{"output": "first"})
	firstOutput.Parent = &first.Ref
	secondOutput := event(session, "second-output", "tool.failed", nil, &agent, map[string]interface{}{"output": "not found"})
	secondOutput.Parent = &second.Ref

	err, messages := openAICompatibleMessages([]model.SessionEvent{user, first, second, firstOutput, secondOutput})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(messages))
	}
	if messages[1].Role != "assistant" || len(messages[1].ToolCalls) != 2 || messages[1].ToolCalls[0].ID != "call-2" || messages[1].ToolCalls[1].ID != "call-1" {
		t.Fatalf("batched calls = %#v", messages[1])
	}
	if messages[2].Role != "tool" || messages[2].ToolCallID != "call-1" || messages[2].Content != "first" {
		t.Fatalf("first output = %#v", messages[2])
	}
	if messages[3].Role != "tool" || messages[3].ToolCallID != "call-2" || messages[3].Content != "not found" {
		t.Fatalf("second output = %#v", messages[3])
	}
}

func TestOpenAICompatibleMessagesRejectsInvalidToolHistory(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	output := event(session, "output", "tool.result", nil, &agent, map[string]interface{}{"output": "orphaned"})
	output.Parent = &model.SessionEventRef{Session: session, Id: "missing"}
	if err, _ := openAICompatibleMessages([]model.SessionEvent{output}); err == nil {
		t.Fatal("openAICompatibleMessages() accepted an orphaned tool output")
	}
}

func event(session model.SessionRef, id, kind string, principal *model.PrincipalRef, agent *model.WorkspaceAgentRef, payload map[string]interface{}) model.SessionEvent {
	return model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Kind: kind, AuthorPrincipal: principal, AuthorAgent: agent, Payload: payload}
}
