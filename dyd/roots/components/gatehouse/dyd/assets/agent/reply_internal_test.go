package agent

import (
	"testing"

	"gatehouse/database"
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
