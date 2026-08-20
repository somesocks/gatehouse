package agent

import (
	"strings"
	"testing"

	"gatehouse/database"
	"gatehouse/model"
)

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

func TestOpenAISystemPromptDocumentsSessionAndProjectCapabilities(t *testing.T) {
	for _, binding := range []string{"@native:gatehouse/session/v1", "@native:gatehouse/project/v1", "session/files/list", "session/files/read", "session/notes/list", "session/notes/read", "session/notes/create", "session/notes/remove", "project/info/get", "project/info/set", "project/files/list", "project/files/read", "project/notes/list", "project/notes/read", "project/notes/create", "project/notes/remove"} {
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
