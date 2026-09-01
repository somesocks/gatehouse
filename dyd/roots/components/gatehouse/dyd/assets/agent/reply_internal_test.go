package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gatehouse/database"
	"gatehouse/model"
)

func TestWatchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	requested := make(chan struct{})
	stop := watchCancellation(func(context.Context) bool {
		select {
		case <-requested:
			return true
		default:
			return false
		}
	}, cancel)
	defer stop()
	close(requested)
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Fatalf("cancellation context error = %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation watcher did not cancel the evaluation context")
	}
}

func TestOpenAISystemPromptFor(t *testing.T) {
	customPrompt := "Custom instructions."
	emptyPrompt := ""
	for name, prompt := range map[string]*string{
		"default": nil,
		"custom":  &customPrompt,
		"empty":   &emptyPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			got := openAISystemPromptFor(&database.WorkspaceAgentModel{SystemPrompt: prompt})
			if name == "default" && got != openAISystemPrompt {
				t.Fatalf("system prompt = %q, want default prompt", got)
			}
			if name == "custom" && got != "Custom instructions." {
				t.Fatalf("system prompt = %q, want custom prompt", got)
			}
			if name == "empty" && got != "" {
				t.Fatalf("system prompt = %q, want empty prompt", got)
			}
		})
	}
}

func TestOpenAIFinalReply(t *testing.T) {
	arguments, err := json.Marshal(map[string]interface{}{"text": "Done.", "attachments": []string{"sfi_01arz3ndektsv4rrffq69g5fav"}})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := openAIFinalReply(string(arguments))
	if err != nil || reply.Text != "Done." || len(reply.Attachments) != 1 || reply.Attachments[0] != "sfi_01arz3ndektsv4rrffq69g5fav" {
		t.Fatalf("openAIFinalReply() = (%#v, %v)", reply, err)
	}
	if _, err := openAIFinalReply(`{"text":"Done.","attachments":null}`); err == nil {
		t.Fatal("openAIFinalReply() accepted a null attachment list")
	}
	if reply, err := openAIFinalReply(`{"text":"","attachments":[]}`); err != nil || reply.Text != "" || len(reply.Attachments) != 0 {
		t.Fatalf("openAIFinalReply() empty reply = (%#v, %v)", reply, err)
	}
}

func TestOpenAIResponsesFinalReply(t *testing.T) {
	valid := openAIResponsesOutput{Type: "message", Status: "completed", Content: []openAIResponsesOutputContent{{Type: "output_text", Text: `{"text":"Done.","attachments":[]}`}}}
	reply, err := openAIResponsesFinalReply([]openAIResponsesOutput{{Type: "reasoning"}, valid})
	if err != nil || reply.Text != "Done." || len(reply.Attachments) != 0 {
		t.Fatalf("openAIResponsesFinalReply() = (%#v, %v)", reply, err)
	}
	later := openAIResponsesOutput{Type: "message", Status: "completed", Content: []openAIResponsesOutputContent{{Type: "output_text", Text: `{"text":"Later.","attachments":[]}`}}}
	reply, err = openAIResponsesFinalReply([]openAIResponsesOutput{valid, later})
	if err != nil || reply.Text != "Done." {
		t.Fatalf("openAIResponsesFinalReply() first valid output = (%#v, %v)", reply, err)
	}
	invalid := openAIResponsesOutput{Type: "message", Status: "completed", Content: []openAIResponsesOutputContent{{Type: "output_text", Text: `not JSON`}}}
	reply, err = openAIResponsesFinalReply([]openAIResponsesOutput{invalid, later})
	if err != nil || reply.Text != "Later." {
		t.Fatalf("openAIResponsesFinalReply() later valid output = (%#v, %v)", reply, err)
	}
	if _, err := openAIResponsesFinalReply([]openAIResponsesOutput{invalid}); err == nil {
		t.Fatal("openAIResponsesFinalReply() accepted no valid output text")
	}
}

func TestOpenAIRequestMessagesOmitsEmptySystemPrompt(t *testing.T) {
	emptyPrompt := ""
	messages := []openAICompatibleMessage{{Role: "user", Content: "hello"}}
	withoutPrompt := openAIRequestMessages(&database.WorkspaceAgentModel{SystemPrompt: &emptyPrompt}, messages)
	if len(withoutPrompt) != 1 || withoutPrompt[0].Role != "user" || withoutPrompt[0].Content != "hello" {
		t.Fatalf("messages without prompt = %#v, want user message only", withoutPrompt)
	}
	withDefaultPrompt := openAIRequestMessages(&database.WorkspaceAgentModel{}, messages)
	if len(withDefaultPrompt) != 2 || withDefaultPrompt[0].Role != "system" || withDefaultPrompt[0].Content != openAISystemPrompt || withDefaultPrompt[1].Role != "user" || withDefaultPrompt[1].Content != "hello" {
		t.Fatalf("messages with default prompt = %#v", withDefaultPrompt)
	}
}

func TestOpenAISystemPromptDocumentsSessionProjectAndWebCapabilities(t *testing.T) {
	for _, binding := range []string{"@native:gatehouse/session/v1", "@native:gatehouse/project/v1", "@native:gatehouse/web/v1", "session/files/list", "session/files/info", "session/files/read", "session/events/read", "session/files/create", "session/notes/list", "session/notes/read", "session/notes/revisions/list", "session/notes/revisions/read", "session/notes/create", "session/notes/update", "session/notes/remove", "project/info/get", "project/info/set", "project/files/list", "project/files/info", "project/files/read", "project/notes/list", "project/notes/read", "project/notes/revisions/list", "project/notes/revisions/read", "project/notes/create", "project/notes/update", "project/notes/remove", "project/secrets/list", "project/secrets/read", "web/search", "web/fetch"} {
		if !strings.Contains(openAISystemPrompt, binding) {
			t.Fatalf("system prompt does not document %q", binding)
		}
	}
}

func TestSessionToolCallWorkflowIDUsesToolRequestID(t *testing.T) {
	request := model.SessionEventRef{Id: "sev_00000000000000000000000000"}
	if got, want := sessionToolCallWorkflowID(request), "session-tool-call:sev_00000000000000000000000000"; got != want {
		t.Fatalf("sessionToolCallWorkflowID() = %q, want %q", got, want)
	}
}

func TestSessionApprovalWorkflowIDUsesApprovalRequestID(t *testing.T) {
	request := model.SessionEventRef{Id: "sev_00000000000000000000000000"}
	if got, want := sessionApprovalWorkflowID(request), "session-approval:sev_00000000000000000000000000"; got != want {
		t.Fatalf("sessionApprovalWorkflowID() = %q, want %q", got, want)
	}
}
