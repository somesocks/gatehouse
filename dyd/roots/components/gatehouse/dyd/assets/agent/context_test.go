package agent

import (
	"strings"
	"testing"

	"gatehouse/model"
)

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
