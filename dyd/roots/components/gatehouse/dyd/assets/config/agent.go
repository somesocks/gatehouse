package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gatehouse/configschema"
)

const (
	agentProviderProtocolBuiltin                = "builtin"
	agentProviderProtocolOpenAIChatCompletions  = "openai-chat-completions"
	agentProviderProtocolOpenAIResponses        = "openai-responses"
	agentModelBuiltinDummyFixedReply      = "dummy.fixed-reply"
)

type AgentProvider struct {
	ID       string
	Revision int
	Protocol string
	BaseURL  *string
	Keychain *string
	Sources  []AgentProviderAPIKeySource
	Enabled  bool
}

type AgentProviderAPIKeySource string

type AgentModel struct {
	ID         string
	Revision   int
	Provider   string
	Model      string
	Parameters string
	Enabled    bool
}

type WorkspaceAgent struct {
	WorkspaceID string
	Model       string
	Priority    int
	MaxTurns    int
	SystemPrompt *string
	Enabled     bool
}

const DefaultWorkspaceAgentMaxTurns = 127

func ResolveAgentProviders(document configschema.GatehouseConfig) (error, []AgentProvider) {
	if document.AgentProviders == nil {
		return nil, []AgentProvider{}
	}
	providers := make([]AgentProvider, 0, len(*document.AgentProviders))
	ids := make(map[string]struct{}, len(*document.AgentProviders))
	for index, configured := range *document.AgentProviders {
		if !keychainID.MatchString(configured.Id) {
			return fmt.Errorf("agent_providers[%d].id must match %q", index, keychainID.String()), nil
		}
		if _, exists := ids[configured.Id]; exists {
			return fmt.Errorf("agent_providers[%d].id %q is duplicated", index, configured.Id), nil
		}
		if configured.Revision <= 0 {
			return fmt.Errorf("agent_providers[%d] has an invalid revision or protocol", index), nil
		}
		enabled := configured.Enabled == nil || *configured.Enabled
		provider := AgentProvider{ID: configured.Id, Revision: configured.Revision, Protocol: configured.Protocol, Enabled: enabled}
		switch configured.Protocol {
		case agentProviderProtocolBuiltin:
			if configured.BaseUrl != nil || configured.ApiKey != nil {
				return fmt.Errorf("agent_providers[%d] builtin providers do not accept base_url or api_key", index), nil
			}
		case agentProviderProtocolOpenAIChatCompletions, agentProviderProtocolOpenAIResponses:
			if configured.BaseUrl == nil || configured.ApiKey == nil {
				return fmt.Errorf("agent_providers[%d] OpenAI providers require base_url and api_key", index), nil
			}
			parsed, err := url.ParseRequestURI(*configured.BaseUrl)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return fmt.Errorf("agent_providers[%d].base_url must be an absolute HTTP URL", index), nil
			}
			keychain := defaultKeychainID
			if configured.ApiKey.Keychain != nil {
				keychain = *configured.ApiKey.Keychain
			}
			if !keychainID.MatchString(keychain) || len(configured.ApiKey.Sources) == 0 {
				return fmt.Errorf("agent_providers[%d].api_key is invalid", index), nil
			}
			sources := make([]AgentProviderAPIKeySource, 0, len(configured.ApiKey.Sources))
			seen := make(map[string]struct{}, len(configured.ApiKey.Sources))
			for sourceIndex, source := range configured.ApiKey.Sources {
				if _, exists := seen[source]; exists {
					return fmt.Errorf("agent_providers[%d].api_key.sources[%d] %q is duplicated", index, sourceIndex, source), nil
				}
				if !environmentReference.MatchString(source) && source != "stdin:" {
					return fmt.Errorf("agent_providers[%d].api_key.sources[%d] is invalid", index, sourceIndex), nil
				}
				seen[source] = struct{}{}
				sources = append(sources, AgentProviderAPIKeySource(source))
			}
			provider.BaseURL = configured.BaseUrl
			provider.Keychain = &keychain
			provider.Sources = sources
		default:
			return fmt.Errorf("agent_providers[%d] has an invalid protocol", index), nil
		}
		ids[configured.Id] = struct{}{}
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(left, right int) bool { return providers[left].ID < providers[right].ID })
	return nil, providers
}

func ResolveAgentModels(document configschema.GatehouseConfig, providers []AgentProvider) (error, []AgentModel) {
	if document.AgentModels == nil {
		return nil, []AgentModel{}
	}
	providerProtocols := make(map[string]string, len(providers))
	for _, provider := range providers {
		providerProtocols[provider.ID] = provider.Protocol
	}
	models := make([]AgentModel, 0, len(*document.AgentModels))
	ids := make(map[string]struct{}, len(*document.AgentModels))
	for index, configured := range *document.AgentModels {
		if !keychainID.MatchString(configured.Id) || configured.Revision <= 0 || strings.TrimSpace(configured.Model) == "" {
			return fmt.Errorf("agent_models[%d] is invalid", index), nil
		}
		if _, exists := ids[configured.Id]; exists {
			return fmt.Errorf("agent_models[%d].id %q is duplicated", index, configured.Id), nil
		}
		protocol, exists := providerProtocols[configured.Provider]
		if !exists {
			return fmt.Errorf("agent_models[%d].provider %q is not configured", index, configured.Provider), nil
		}
		parameters, ok := configured.Parameters.(map[string]any)
		if !ok {
			return fmt.Errorf("agent_models[%d].parameters must be an object", index), nil
		}
		if protocol == agentProviderProtocolBuiltin {
			text, textOK := parameters["text"].(string)
			if configured.Model != agentModelBuiltinDummyFixedReply || !textOK || strings.TrimSpace(text) == "" || len(parameters) != 1 {
				return fmt.Errorf("agent_models[%d] has an invalid builtin model or parameters", index), nil
			}
		}
		if err := validateOpenAIModelParameters(protocol, parameters); err != nil {
			return fmt.Errorf("agent_models[%d].parameters: %w", index, err), nil
		}
		encoded, err := json.Marshal(parameters)
		if err != nil {
			return fmt.Errorf("encode agent_models[%d].parameters: %w", index, err), nil
		}
		enabled := configured.Enabled == nil || *configured.Enabled
		ids[configured.Id] = struct{}{}
		models = append(models, AgentModel{ID: configured.Id, Revision: configured.Revision, Provider: configured.Provider, Model: configured.Model, Parameters: string(encoded), Enabled: enabled})
	}
	sort.Slice(models, func(left, right int) bool { return models[left].ID < models[right].ID })
	return nil, models
}

func validateOpenAIModelParameters(protocol string, parameters map[string]any) error {
	if protocol == agentProviderProtocolBuiltin {
		return nil
	}
	if len(parameters) == 0 {
		return nil
	}
	effort, ok := parameters["reasoning_effort"].(string)
	if !ok || len(parameters) != 1 {
		return fmt.Errorf("must contain only an optional reasoning_effort string")
	}
	valid := false
	for _, candidate := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
		if effort == candidate {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("reasoning_effort %q is invalid", effort)
	}
	if protocol == agentProviderProtocolOpenAIChatCompletions && effort != "none" {
		return fmt.Errorf("reasoning_effort must be none for openai-chat-completions tool use")
	}
	return nil
}

func ResolveWorkspaceAgents(document configschema.GatehouseConfig, models []AgentModel) (error, []WorkspaceAgent) {
	if document.Workspaces == nil {
		return nil, []WorkspaceAgent{}
	}
	modelEnabled := make(map[string]bool, len(models))
	for _, model := range models {
		modelEnabled[model.ID] = model.Enabled
	}
	agents := []WorkspaceAgent{}
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Agents == nil {
			continue
		}
		seen := make(map[string]struct{}, len(*workspace.Agents))
		for agentIndex, configured := range *workspace.Agents {
			if _, exists := seen[configured.Model]; exists {
				return fmt.Errorf("workspaces[%d].agents[%d].model %q is duplicated", workspaceIndex, agentIndex, configured.Model), nil
			}
			modelIsEnabled, exists := modelEnabled[configured.Model]
			if !exists || configured.Priority <= 0 {
				return fmt.Errorf("workspaces[%d].agents[%d] is invalid", workspaceIndex, agentIndex), nil
			}
			maxTurns := DefaultWorkspaceAgentMaxTurns
			if configured.MaxTurns != nil {
				maxTurns = *configured.MaxTurns
			}
			if maxTurns <= 0 {
				return fmt.Errorf("workspaces[%d].agents[%d].max_turns must be positive", workspaceIndex, agentIndex), nil
			}
			enabled := configured.Enabled == nil || *configured.Enabled
			if enabled && !modelIsEnabled {
				return fmt.Errorf("workspaces[%d].agents[%d].model %q is disabled", workspaceIndex, agentIndex, configured.Model), nil
			}
			seen[configured.Model] = struct{}{}
			agents = append(agents, WorkspaceAgent{WorkspaceID: workspace.Id, Model: configured.Model, Priority: configured.Priority, MaxTurns: maxTurns, SystemPrompt: configured.SystemPrompt, Enabled: enabled})
		}
	}
	sort.Slice(agents, func(left, right int) bool {
		if agents[left].WorkspaceID == agents[right].WorkspaceID {
			return agents[left].Model < agents[right].Model
		}
		return agents[left].WorkspaceID < agents[right].WorkspaceID
	})
	return nil, agents
}
