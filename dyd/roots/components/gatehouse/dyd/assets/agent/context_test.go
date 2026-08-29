package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"gatehouse/model"
)

func TestCompileMCMTRContextKeepsOnlyNewestActiveToolBatchNative(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	historicalUser := event(session, "historical-user", "message.text", &principal, nil, map[string]interface{}{"text": "Earlier request."})
	active := event(session, "active", "message.text", &principal, nil, map[string]interface{}{"text": "Current request."})
	firstCall := event(session, "first-call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(first)", "reason": "First pass.", "batch": float64(0), "position": float64(0)})
	firstCall.Parent = &active.Ref
	firstResult := event(session, "first-result", "tool.success", nil, &agent, map[string]interface{}{"output": "first output"})
	firstResult.Parent = &firstCall.Ref
	intermediate := event(session, "intermediate", "message.text", nil, &agent, map[string]interface{}{"text": "Continuing."})
	intermediate.Parent = &active.Ref
	lastCall := event(session, "last-call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-2", "code": "(last)", "reason": "Last pass.", "batch": float64(1), "position": float64(0)})
	lastCall.Parent = &active.Ref
	lastResult := event(session, "last-result", "tool.failure", nil, &agent, map[string]interface{}{"output": "last output"})
	lastResult.Parent = &lastCall.Ref

	messages, _, err := compileMCMTRContext([]model.SessionEvent{historicalUser, active, firstCall, firstResult, intermediate, lastCall, lastResult}, active.Ref, mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}, mcmtrContextState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[0].Role != "assistant" || messages[1].Role != "user" || messages[2].Role != "assistant" || messages[3].Role != "tool" {
		t.Fatalf("messages = %#v", messages)
	}
	if !strings.HasPrefix(messages[0].Content, "<session-transcript>") || strings.Contains(messages[0].Content, "<events>") || !strings.Contains(messages[0].Content, `id="historical-user"`) || !strings.Contains(messages[0].Content, `id="first-call"`) || !strings.Contains(messages[0].Content, `id="first-result"`) || !strings.Contains(messages[0].Content, `id="intermediate"`) || strings.Contains(messages[0].Content, `id="last-call"`) || strings.Contains(messages[0].Content, `id="last-result"`) {
		t.Fatalf("transcript = %q", messages[0].Content)
	}
	if !strings.Contains(messages[1].Content, `root-id="active"`) || !strings.Contains(messages[1].Content, `author="principal:user"`) || !strings.Contains(messages[1].Content, `<text>Current request.</text>`) {
		t.Fatalf("active user = %#v", messages[1])
	}
	if len(messages[2].ToolCalls) != 1 || messages[2].ToolCalls[0].ID != "call-2" || messages[2].ToolCalls[0].Function.Arguments != `{"code":"(last)","reason":"Last pass."}` {
		t.Fatalf("native calls = %#v", messages[2])
	}
	if messages[3].ToolCallID != "call-2" || !strings.Contains(messages[3].Content, `root-id="active"`) || !strings.Contains(messages[3].Content, `author="agent:agent"`) || !strings.Contains(messages[3].Content, `<output>last output</output>`) {
		t.Fatalf("native output = %#v", messages[3])
	}
	encoded, err := json.Marshal(openAICompatibleRequest{Model: "test", Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	var chat openAICompatibleRequest
	if err := json.Unmarshal(encoded, &chat); err != nil || len(chat.Messages) != 4 || chat.Messages[1].Role != "user" || len(chat.Messages[2].ToolCalls) != 1 || chat.Messages[3].ToolCallID != "call-2" {
		t.Fatalf("Chat Completions messages = %#v, %v", chat.Messages, err)
	}
	input := openAIResponsesInput(messages)
	if len(input) != 4 || !strings.Contains(string(input[0]), "session-transcript") || !strings.Contains(string(input[1]), "Current request") || !strings.Contains(string(input[2]), `"type":"function_call"`) || !strings.Contains(string(input[3]), `"type":"function_call_output"`) {
		t.Fatalf("Responses input = %s", input)
	}
}

func TestCompileMCMTRContextTruncatesActiveUserAndNativeToolRecords(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	active := event(session, "active", "message.text", &principal, nil, map[string]interface{}{"text": strings.Repeat("u", 20*1024)})
	call := event(session, "call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": strings.Repeat("c", 20*1024), "reason": "Large call.", "batch": float64(0), "position": float64(0)})
	call.Parent = &active.Ref
	result := event(session, "result", "tool.success", nil, &agent, map[string]interface{}{"output": strings.Repeat("o", 20*1024)})
	result.Parent = &call.Ref

	messages, _, err := compileMCMTRContext([]model.SessionEvent{active, call, result}, active.Ref, mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}, mcmtrContextState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || !strings.Contains(messages[0].Content, `truncated="true"`) || len(messages[1].ToolCalls) != 1 || !strings.Contains(messages[2].Content, `truncated="true"`) {
		t.Fatalf("messages = %#v", messages)
	}
	var arguments struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(messages[1].ToolCalls[0].Function.Arguments), &arguments); err != nil || !strings.Contains(arguments.Reason, `event "call"`) || !strings.Contains(arguments.Reason, "size-bytes=") {
		t.Fatalf("native truncated arguments = %q, %v", messages[1].ToolCalls[0].Function.Arguments, err)
	}
	err, contents := sessionEventReadRange(call, 0, 1<<20)
	if err != nil || !strings.Contains(string(contents), strings.Repeat("c", 20*1024)) {
		t.Fatalf("sessionEventReadRange() = (%q, %v)", contents, err)
	}
}

func TestMCMTRHighTierAdvancesToHalfBuffer(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	records := make([]mcmtrRecord, 0, 4)
	for index := 0; index < 4; index++ {
		event := event(session, "user-"+string(rune('a'+index)), "message.text", &principal, nil, map[string]interface{}{"text": strings.Repeat("x", 400)})
		record, included, err := mcmtrRecordFor(event)
		if err != nil || !included {
			t.Fatalf("mcmtrRecordFor() = (%#v, %t, %v)", record, included, err)
		}
		record.stream = "user"
		records = append(records, record)
	}

	high, state := mcmtrHighTier(records, mcmtrContextState{}, 1024)
	if state.UserHighFrom != "user-d" || !high["user-d"] || len(high) != 1 {
		t.Fatalf("initial high tier = (%#v, %#v)", high, state)
	}
	_, next := mcmtrHighTier(records, state, 1024)
	if next.UserHighFrom != state.UserHighFrom {
		t.Fatalf("checkpoint advanced without filling buffer: %#v -> %#v", state, next)
	}
}

func TestSelectContextMessagesUsesUserMessageTiers(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	events := make([]model.SessionEvent, 22)
	for index := range events {
		events[index] = event(session, "message-"+string(rune('a'+index)), "message.text", &principal, nil, map[string]interface{}{"text": strings.Repeat("x", 4*1024)})
	}
	err, messages := openAICompatibleMessages(events)
	if err != nil {
		t.Fatal(err)
	}
	messages = selectContextMessages(messages, contextMaximumBytes)
	full, preview, short, omitted := 0, 0, 0, 0
	for _, message := range messages {
		if message.Message == nil {
			continue
		}
		switch {
		case message.Message.Omitted:
			omitted++
		case message.Message.Truncated && len(message.Content) == 1024:
			preview++
		case message.Message.Truncated && len(message.Content) == 250:
			short++
		case !message.Message.Truncated && len(message.Content) == 4*1024:
			full++
		default:
			t.Fatalf("unexpected message tier: %#v", message)
		}
	}
	if full != 3 || preview != 6 || short != 12 || omitted != 1 {
		t.Fatalf("tiers = full %d preview %d short %d omitted %d", full, preview, short, omitted)
	}
}

func TestSelectContextMessagesOmitsToolOutputWithoutCallArguments(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	request := event(session, "call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call", "code": strings.Repeat("x", 3*1024), "reason": "test"})
	output := event(session, "output", "tool.success", nil, &agent, map[string]interface{}{"output": "result"})
	output.Parent = &request.Ref
	err, messages := openAICompatibleMessages([]model.SessionEvent{request, output})
	if err != nil {
		t.Fatal(err)
	}
	messages = selectContextMessages(messages, contextMaximumBytes)
	messages = renderTranscriptMessages(messages)
	if len(messages) != 2 || messages[0].Content != "<events><event id=\"call\" kind=\"tool-call\" call-id=\"call\" /></events>" || messages[1].Role != "assistant" || messages[1].Content != "<events><event id=\"output\" parent-id=\"call\" kind=\"tool-result\" status=\"success\" truncated=\"true\" size-bytes=\"6\" shown-bytes=\"0\" /></events>" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestTranscriptMessageRendersNoReply(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "agent"}}
	event := event(session, "reply", "message.text", nil, &agent, map[string]interface{}{"text": ""})
	message, text, ok := transcriptMessageContent(event)
	if !ok {
		t.Fatal("transcriptMessageContent() rejected an empty reply")
	}
	rendered := renderTranscriptMessages([]openAICompatibleMessage{{Role: "assistant", Content: text, Message: message}})
	if len(rendered) != 1 || rendered[0].Content != "<events><event id=\"reply\" kind=\"message\"><no-reply /></event></events>" {
		t.Fatalf("rendered = %#v", rendered)
	}
}
