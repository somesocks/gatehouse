package agent

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

// A transient wire request. Neither it nor the rendered inputs are persisted in
// thinking.request; the event's agent and parent references are authoritative.
type agentProviderRequest struct {
	Chat      *openAICompatibleRequest
	Responses *openAIResponsesRequest
	codex     bool
}

func (request agentProviderRequest) body() any {
	if request.Chat != nil {
		return request.Chat
	}
	if request.Responses != nil {
		if request.codex {
			return codexRequestFromResponses(*request.Responses)
		}
		return request.Responses
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) agentProviderRequestForThinking(ctx context.Context, thinking model.SessionEvent) (error, *database.WorkspaceAgentModel, agentProviderRequest) {
	if thinking.Kind != model.SessionEventKindThinkingRequest || thinking.AuthorAgent == nil || thinking.Parent == nil {
		return fmt.Errorf("invalid thinking request"), nil, agentProviderRequest{}
	}
	err, selected := runtime.store.WorkspaceAgentModelGet(ctx, thinking.AuthorAgent.Workspace, thinking.AuthorAgent.Id)
	if err != nil {
		return err, nil, agentProviderRequest{}
	}
	if selected == nil {
		return fmt.Errorf("agent model or provider is unavailable"), nil, agentProviderRequest{}
	}
	selected.SystemPromptReplacement = runtime.systemPromptReplacement
	if text, ok := runtime.systemPromptReplacements[selected.Ref]; ok {
		selected.SystemPromptReplacement = &text
	}
	if selected.Protocol == "builtin" {
		return nil, selected, agentProviderRequest{}
	}
	err, projectChat, messages := runtime.agentRoundContext(ctx, thinking, selected)
	if err != nil {
		return err, selected, agentProviderRequest{}
	}
	err, request := agentProviderRequestPrepare(selected, projectChat, messages)
	return err, selected, request
}

func agentProviderRequestPrepare(selected *database.WorkspaceAgentModel, projectChat bool, messages []openAICompatibleMessage) (error, agentProviderRequest) {
	request := agentProviderRequest{}
	if selected.Protocol == "builtin" {
		return nil, request
	}
	if selected.Endpoint == nil || selected.Keychain == nil || selected.APIKey == nil {
		return fmt.Errorf("reply with provider %q: missing credentials", selected.ProviderID), request
	}
	err, effort := openAICompatibleReasoningEffort(selected.Parameters)
	if err != nil {
		return err, request
	}
	switch selected.Protocol {
	case "openai-chat-completions":
		if effort != "" && effort != "none" {
			return fmt.Errorf("Chat Completions tool use requires reasoning_effort none"), request
		}
		request.Chat = &openAICompatibleRequest{
			Model: selected.Model, Messages: openAIRequestMessages(selected, messages, projectChat),
			Tools: []openAICompatibleTool{openAICompatibleLispTool()}, ResponseFormat: openAIFinalReplyResponseFormat(),
			ParallelToolCalls: true, ReasoningEffort: effort, MaxTokens: selected.MaxOutputTokens,
		}
	case "openai-responses", config.AgentProviderProtocolOpenAICodex:
		request.codex = selected.Protocol == config.AgentProviderProtocolOpenAICodex
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
		return fmt.Errorf("unsupported provider protocol %q", selected.Protocol), request
	}
	return nil, request
}
