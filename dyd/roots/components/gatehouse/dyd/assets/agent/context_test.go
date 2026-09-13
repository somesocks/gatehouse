package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gatehouse/model"
)

func TestCompileMCMTRContextReplaysActiveToolBatchesNatively(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
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
	if len(messages) != 7 || messages[0].Role != "user" || messages[1].Role != "user" || messages[2].Role != "assistant" || messages[3].Role != "tool" || messages[4].Role != "assistant" || messages[5].Role != "assistant" || messages[6].Role != "tool" {
		t.Fatalf("messages = %#v", messages)
	}
	if !strings.Contains(messages[0].Content, `id="historical-user"`) || strings.Contains(messages[0].Content, "session-transcript") {
		t.Fatalf("historical user = %q", messages[0].Content)
	}
	if !strings.Contains(messages[1].Content, `root-id="active"`) || !strings.Contains(messages[1].Content, `author="principal:user"`) || !strings.Contains(messages[1].Content, `<text>Current request.</text>`) {
		t.Fatalf("active user = %#v", messages[1])
	}
	if len(messages[2].ToolCalls) != 1 || messages[2].ToolCalls[0].ID != "call-1" || messages[2].ToolCalls[0].Function.Arguments != `{"code":"(first)","reason":"First pass."}` || messages[3].ToolCallID != "call-1" || !strings.Contains(messages[3].Content, `<output>first output</output>`) {
		t.Fatalf("first native batch = %#v, %#v", messages[2], messages[3])
	}
	if !strings.Contains(messages[4].Content, `id="intermediate"`) || len(messages[5].ToolCalls) != 1 || messages[5].ToolCalls[0].ID != "call-2" || messages[6].ToolCallID != "call-2" || !strings.Contains(messages[6].Content, `<output>last output</output>`) {
		t.Fatalf("later records = %#v, %#v, %#v", messages[4], messages[5], messages[6])
	}
	encoded, err := json.Marshal(openAICompatibleRequest{Model: "test", Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	var chat openAICompatibleRequest
	if err := json.Unmarshal(encoded, &chat); err != nil || len(chat.Messages) != 7 || chat.Messages[1].Role != "user" || len(chat.Messages[2].ToolCalls) != 1 || chat.Messages[3].ToolCallID != "call-1" || len(chat.Messages[5].ToolCalls) != 1 || chat.Messages[6].ToolCallID != "call-2" {
		t.Fatalf("Chat Completions messages = %#v, %v", chat.Messages, err)
	}
	input := openAIResponsesInput(messages)
	if len(input) != 7 || !strings.Contains(string(input[0]), "Earlier request") || !strings.Contains(string(input[1]), "Current request") || !strings.Contains(string(input[2]), `"type":"function_call"`) || !strings.Contains(string(input[3]), `"type":"function_call_output"`) || !strings.Contains(string(input[5]), `"type":"function_call"`) || !strings.Contains(string(input[6]), `"type":"function_call_output"`) {
		t.Fatalf("Responses input = %s", input)
	}
}

func TestCompileMCMTRContextTruncatesActiveUserAndNativeToolRecords(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
	active := event(session, "active", "message.text", &principal, nil, map[string]interface{}{"text": strings.Repeat("u", 20*1024)})
	call := event(session, "call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": strings.Repeat("c", 20*1024), "reason": "Large call.", "batch": float64(0), "position": float64(0)})
	call.Parent = &active.Ref
	result := event(session, "result", "tool.success", nil, &agent, map[string]interface{}{"output": strings.Repeat("o", 20*1024)})
	result.Parent = &call.Ref

	messages, _, err := compileMCMTRContext([]model.SessionEvent{active, call, result}, active.Ref, mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}, mcmtrContextState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || !strings.Contains(messages[0].Content, `truncated="true"`) || messages[1].Role != "assistant" || len(messages[1].ToolCalls) != 0 || !strings.Contains(messages[1].Content, `kind="tool-call"`) || messages[2].Role != "assistant" || !strings.Contains(messages[2].Content, `kind="tool-result"`) {
		t.Fatalf("messages = %#v", messages)
	}
	err, contents := sessionEventReadRange(call, 0, 1<<20)
	if err != nil || !strings.Contains(string(contents), strings.Repeat("c", 20*1024)) {
		t.Fatalf("sessionEventReadRange() = (%q, %v)", contents, err)
	}
}

func TestCompileMCMTRContextCapsNativeToolResultContent(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
	active := event(session, "active", "message.text", &principal, nil, map[string]interface{}{"text": "Current request."})
	call := event(session, "call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(read)", "reason": "Read the value.", "batch": float64(0), "position": float64(0)})
	call.Parent = &active.Ref
	rawOutput := strings.Repeat("x", mcmtrToolResultContentMaximumBytes+1)
	result := event(session, "result", "tool.success", nil, &agent, map[string]interface{}{"output": rawOutput})
	result.Parent = &call.Ref

	messages, _, err := compileMCMTRContext([]model.SessionEvent{active, call, result}, active.Ref, mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}, mcmtrContextState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[2].Role != "tool" || messages[2].ToolCallID != "call-1" {
		t.Fatalf("messages = %#v", messages)
	}
	content := messages[2].Content
	if !strings.Contains(content, `truncated="true"`) || !strings.Contains(content, `size-bytes="4097"`) || !strings.Contains(content, `shown-bytes="4096"`) {
		t.Fatalf("tool output metadata = %q", content)
	}
	outputStart := strings.Index(content, "<output")
	if outputStart < 0 {
		t.Fatalf("tool output = %q", content)
	}
	outputStart += strings.Index(content[outputStart:], ">") + 1
	outputEnd := strings.Index(content[outputStart:], "</output>")
	if outputEnd < 0 || content[outputStart:outputStart+outputEnd] != rawOutput[:mcmtrToolResultContentMaximumBytes] {
		t.Fatalf("shown tool output = %q", content)
	}
}

func TestCompileMCMTRContextAppendsResolvedToolBatches(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
	active := event(session, "active", "message.text", &principal, nil, map[string]interface{}{"text": "Current request."})
	firstCall := event(session, "first-call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(first)", "reason": "First pass.", "batch": float64(0), "position": float64(0)})
	firstCall.Parent = &active.Ref
	firstResult := event(session, "first-result", "tool.success", nil, &agent, map[string]interface{}{"output": "first output"})
	firstResult.Parent = &firstCall.Ref
	secondCall := event(session, "second-call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-2", "code": "(second)", "reason": "Second pass.", "batch": float64(1), "position": float64(0)})
	secondCall.Parent = &active.Ref
	secondResult := event(session, "second-result", "tool.success", nil, &agent, map[string]interface{}{"output": "second output"})
	secondResult.Parent = &secondCall.Ref
	profile := mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}

	first, state, err := compileMCMTRContext([]model.SessionEvent{active, firstCall, firstResult}, active.Ref, profile, mcmtrContextState{})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := compileMCMTRContext([]model.SessionEvent{active, firstCall, firstResult, secondCall, secondResult}, active.Ref, profile, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first)+2 || !reflect.DeepEqual(second[:len(first)], first) {
		t.Fatalf("second context did not append first=%#v second=%#v", first, second)
	}
}

func TestMCMTRRecordMessagesRendersUnpairedResultAsHistory(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
	call := event(session, "call", "tool.request", nil, &agent, map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(read)", "reason": "Read the value.", "batch": float64(0), "position": float64(0)})
	result := event(session, "result", "tool.success", nil, &agent, map[string]interface{}{"output": "value"})
	result.Parent = &call.Ref
	callRecord, included, err := mcmtrRecordFor(call)
	if err != nil || !included {
		t.Fatalf("mcmtrRecordFor(call) = (%#v, %t, %v)", callRecord, included, err)
	}
	resultRecord, included, err := mcmtrRecordFor(result)
	if err != nil || !included {
		t.Fatalf("mcmtrRecordFor(result) = (%#v, %t, %v)", resultRecord, included, err)
	}
	callRecord.contents = mcmtrText{omitted: true, sizeBytes: len(callRecord.contents.value)}
	messages := mcmtrRecordMessages([]mcmtrRecord{callRecord, resultRecord})
	if len(messages) != 2 || messages[0].Role != "assistant" || len(messages[0].ToolCalls) != 0 || messages[1].Role != "assistant" || messages[1].ToolCallID != "" || !strings.Contains(messages[1].Content, `kind="tool-result"`) || !strings.Contains(messages[1].Content, `<output>value</output>`) {
		t.Fatalf("messages = %#v", messages)
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
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
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
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_agent"}
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
