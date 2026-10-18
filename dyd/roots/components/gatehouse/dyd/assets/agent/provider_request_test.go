package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gatehouse/database"
	"gatehouse/model"
)

func TestAgentProviderRequestPreservesToolConversationAcrossProtocols(t *testing.T) {
	call := func(id, code string) openAICompatibleToolCall {
		value := openAICompatibleToolCall{ID: id, Type: "function"}
		value.Function.Name = "lisp"
		arguments, _ := json.Marshal(map[string]string{"code": code, "reason": "test"})
		value.Function.Arguments = string(arguments)
		return value
	}
	messages := []openAICompatibleMessage{
		{Role: "user", Content: "calculate"},
		{Role: "assistant", ToolCalls: []openAICompatibleToolCall{call("call-a", "(int/add 1 2)"), call("call-b", "(int/add 3 4)")}},
		{Role: "tool", ToolCallID: "call-a", Content: "3"},
		{Role: "tool", ToolCallID: "call-b", Content: "7"},
	}
	endpoint, key := "https://new.invalid/custom", "encrypted"
	for _, protocol := range []string{"openai-responses", "openai-chat-completions", "openai-codex"} {
		t.Run(protocol, func(t *testing.T) {
			selected := &database.WorkspaceAgentModel{Alias: "test", Protocol: protocol, Endpoint: &endpoint, APIKey: &key,
				Keychain: &model.KeychainRef{Id: "test", Version: 1}, Model: "new-model", Parameters: `{}`, MaxOutputTokens: 123}
			err, request := agentProviderRequestPrepare(selected, false, messages)
			if err != nil {
				t.Fatal(err)
			}
			if protocol != "openai-chat-completions" {
				if request.Responses.Model != "new-model" || request.Responses.MaxOutputTokens != 123 || !reflect.DeepEqual(request.Responses.Input, openAIResponsesInput(messages)) {
					t.Fatalf("Responses request = %#v", request.Responses)
				}
			} else if request.Chat.Model != "new-model" || request.Chat.MaxTokens != 123 || !reflect.DeepEqual(request.Chat.Messages[1:], messages) {
				t.Fatalf("Chat request = %#v", request.Chat)
			}
		})
	}
}

func TestAgentProviderRequestUsesCurrentPrompt(t *testing.T) {
	endpoint, key, prompt := "https://provider.invalid/custom", "encrypted", "OLD_CONFIGURED_PROMPT"
	for _, protocol := range []string{"openai-responses", "openai-chat-completions", "openai-codex"} {
		selected := &database.WorkspaceAgentModel{Alias: "test", Protocol: protocol, Endpoint: &endpoint, APIKey: &key,
			Keychain: &model.KeychainRef{Id: "test", Version: 1}, Model: "old-model", Parameters: `{}`, MaxOutputTokens: 123, SystemPrompt: &prompt}
		prompt = "OLD_CONFIGURED_PROMPT"
		messages := []openAICompatibleMessage{{Role: "user", Content: "question"}}
		err, first := agentProviderRequestPrepare(selected, false, messages)
		if err != nil {
			t.Fatal(err)
		}
		prompt, selected.Model, selected.MaxOutputTokens = "NEW_CONFIGURED_PROMPT", "new-model", 456
		err, second := agentProviderRequestPrepare(selected, false, messages)
		if err != nil {
			t.Fatal(err)
		}
		oldPrompt, newPrompt := "", ""
		if protocol != "openai-chat-completions" {
			oldPrompt, newPrompt = first.Responses.Instructions, second.Responses.Instructions
			if second.Responses.Model != "new-model" || second.Responses.MaxOutputTokens != 456 {
				t.Fatal("Responses request retained old model settings")
			}
		} else {
			oldPrompt, newPrompt = first.Chat.Messages[0].Content, second.Chat.Messages[0].Content
			if second.Chat.Model != "new-model" || second.Chat.MaxTokens != 456 {
				t.Fatal("Chat request retained old model settings")
			}
		}
		if !strings.Contains(oldPrompt, "OLD_CONFIGURED_PROMPT") || !strings.Contains(newPrompt, "NEW_CONFIGURED_PROMPT") || strings.Contains(newPrompt, "OLD_CONFIGURED_PROMPT") {
			t.Fatalf("prompts did not follow current configuration: %q / %q", oldPrompt, newPrompt)
		}
	}
}

func TestAgentProviderRequestPromptReplacement(t *testing.T) {
	endpoint, key, appendText := "https://provider.invalid", "encrypted", "appended"
	for _, protocol := range []string{"openai-responses", "openai-chat-completions"} {
		for _, replacement := range []string{"", "replacement only"} {
			selected := &database.WorkspaceAgentModel{Alias: "test", Protocol: protocol, Endpoint: &endpoint, APIKey: &key,
				Keychain: &model.KeychainRef{Id: "test", Version: 1}, Model: "test-model", Parameters: `{}`, MaxOutputTokens: 123, SystemPrompt: &appendText}
			before := mcmtrProfileFingerprint(selected, false)
			selected.SystemPromptReplacement = &replacement
			if mcmtrProfileFingerprint(selected, false) == before {
				t.Fatal("prompt replacement did not invalidate the context profile")
			}
			messages := []openAICompatibleMessage{{Role: "user", Content: "question"}}
			err, request := agentProviderRequestPrepare(selected, false, messages)
			if err != nil {
				t.Fatal(err)
			}
			if protocol == "openai-responses" {
				if request.Responses.Instructions != replacement {
					t.Fatalf("replacement instructions = %q", request.Responses.Instructions)
				}
			} else if replacement == "" {
				if !reflect.DeepEqual(request.Chat.Messages, messages) {
					t.Fatal("empty replacement retained a system message")
				}
			} else if request.Chat.Messages[0].Content != replacement || len(request.Chat.Messages) != 2 {
				t.Fatalf("replacement messages = %#v", request.Chat.Messages)
			}
		}
	}
}
