package agent

import (
	"encoding/json"
	"fmt"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

// The exact request lives in the thinking event, not in DBOS. Replaying a round
// may call the provider again, but uses the original request even after tools
// have added events to the transcript. Credentials are resolved at call time.
type agentProviderRequest struct {
	Protocol          string                           `json:"protocol"`
	BaseURL           string                           `json:"base_url"`
	ProviderID        string                           `json:"provider_id"`
	Chat              *openAICompatibleRequest         `json:"chat,omitempty"`
	Responses         *openAIResponsesRequest          `json:"responses,omitempty"`
	RateLimits        *config.WorkspaceAgentRateLimits `json:"rate_limits,omitempty"`
	MaxOutputTokens   int                              `json:"max_output_tokens"`
	MaxTurns          int                              `json:"max_turns"`
	Compaction        string                           `json:"compaction"`
	Principal         model.PrincipalRef               `json:"principal"`
	Round             int                              `json:"round"`
	ToolOffset        int                              `json:"tool_offset"`
	BuiltinModel      string                           `json:"builtin_model,omitempty"`
	BuiltinParameters string                           `json:"builtin_parameters,omitempty"`
}

func agentProviderRequestPrepare(selected *database.WorkspaceAgentModel, projectChat bool, messages []openAICompatibleMessage) (error, agentProviderRequest) {
	request := agentProviderRequest{Protocol: selected.Protocol, ProviderID: selected.ProviderID,
		RateLimits: selected.RateLimits, MaxOutputTokens: selected.MaxOutputTokens,
		MaxTurns: selected.MaxTurns, Compaction: selected.Compaction}
	if selected.Protocol == "builtin" {
		request.BuiltinModel, request.BuiltinParameters = selected.Model, selected.Parameters
		return nil, request
	}
	if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
		return fmt.Errorf("reply with provider %q: missing credentials", selected.ProviderID), request
	}
	request.BaseURL = *selected.BaseURL
	err, effort := openAICompatibleReasoningEffort(selected.Parameters)
	if err != nil {
		return err, request
	}
	switch request.Protocol {
	case "openai-chat-completions":
		request.Chat = &openAICompatibleRequest{
			Model: selected.Model, Messages: openAIRequestMessages(selected, messages, projectChat),
			Tools: []openAICompatibleTool{openAICompatibleLispTool()}, ResponseFormat: openAIFinalReplyResponseFormat(),
			ParallelToolCalls: true, ReasoningEffort: effort, MaxTokens: selected.MaxOutputTokens,
		}
	case "openai-responses":
		var reasoning *openAIResponsesReasoning
		if effort != "" {
			reasoning = &openAIResponsesReasoning{Effort: effort}
		}
		request.Responses = &openAIResponsesRequest{
			Model: selected.Model, Instructions: openAISystemPromptFor(selected, projectChat), Input: openAIResponsesInput(messages),
			Tools: []openAIResponsesTool{openAIResponsesLispTool()}, Text: openAIResponsesFinalReplyText(),
			ParallelToolCalls: true, Reasoning: reasoning, MaxOutputTokens: selected.MaxOutputTokens,
		}
	default:
		return fmt.Errorf("unsupported provider protocol %q", request.Protocol), request
	}
	return nil, request
}

func agentProviderRequestFromThinking(thinking model.SessionEvent) (error, agentProviderRequest) {
	var request agentProviderRequest
	encoded, err := json.Marshal(thinking.Payload["provider_request"])
	if err != nil {
		return err, request
	}
	if err := json.Unmarshal(encoded, &request); err != nil {
		return fmt.Errorf("decode thinking provider request: %w", err), request
	}
	if request.Protocol == "" || request.Protocol != "builtin" && request.BaseURL == "" {
		return fmt.Errorf("thinking event %q has no provider request", thinking.Ref.Id), request
	}
	return nil, request
}
