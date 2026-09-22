package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gatehouse/configschema"
)

const (
	agentProviderProtocolBuiltin               = "builtin"
	agentProviderProtocolOpenAIChatCompletions = "openai-chat-completions"
	agentProviderProtocolOpenAIResponses       = "openai-responses"
	agentModelBuiltinDummyFixedReply           = "dummy.fixed-reply"
)

type AgentProvider struct {
	Alias    string
	Revision int
	Protocol string
	BaseURL  *string
	Keychain *string
	Sources  []AgentProviderAPIKeySource
	Enabled  bool
}

type AgentProviderAPIKeySource string

type AgentModel struct {
	Alias           string
	Revision        int
	ProviderAlias   string
	Model           string
	Parameters      string
	Compaction      AgentModelCompaction
	MaxTurns        int
	MaxOutputTokens int
	Enabled         bool
}

type AgentModelCompaction struct {
	Algorithm    string `json:"algorithm"`
	HistoryBytes int    `json:"history_bytes"`
	BufferBytes  int    `json:"buffer_bytes"`
}

type WorkspaceAgent struct {
	WorkspaceID  string
	Alias        string
	ModelAlias   string
	Revision     int
	Label        *string
	SystemPrompt *string
	Prelude      *string
	RateLimits   *WorkspaceAgentRateLimits
	Default      bool
	Enabled      bool
}

// TokenBucket configures a bounded token credit balance and refill rate.
type TokenBucket struct {
	MinimumBalance  int `json:"minimum_balance"`
	MaximumBalance  int `json:"maximum_balance"`
	RefillPerMinute int `json:"refill_per_minute"`
}

// WorkspaceAgentRateLimits defines optional workspace-global and per-user token buckets.
type WorkspaceAgentRateLimits struct {
	WorkspaceInput  *TokenBucket `json:"workspace_input,omitempty"`
	WorkspaceOutput *TokenBucket `json:"workspace_output,omitempty"`
	UserInput       *TokenBucket `json:"user_input,omitempty"`
	UserOutput      *TokenBucket `json:"user_output,omitempty"`
}

func (limits *WorkspaceAgentRateLimits) Empty() bool {
	return limits == nil || (limits.WorkspaceInput == nil && limits.WorkspaceOutput == nil && limits.UserInput == nil && limits.UserOutput == nil)
}

func ValidateWorkspaceAgentRateLimits(limits *WorkspaceAgentRateLimits) error {
	if limits == nil {
		return nil
	}
	for _, bucket := range []struct {
		name  string
		value *TokenBucket
	}{
		{name: "workspace_input", value: limits.WorkspaceInput},
		{name: "workspace_output", value: limits.WorkspaceOutput},
		{name: "user_input", value: limits.UserInput},
		{name: "user_output", value: limits.UserOutput},
	} {
		if bucket.value != nil && (bucket.value.MinimumBalance > 0 || bucket.value.MaximumBalance <= 0 || bucket.value.RefillPerMinute <= 0) {
			return fmt.Errorf("%s minimum_balance must not be positive and maximum_balance and refill_per_minute must be positive", bucket.name)
		}
	}
	return nil
}

var workspaceAgentAlias = regexp.MustCompile(`^[a-z0-9_-]+(/[a-z0-9_-]+)*$`)

const (
	DefaultAgentModelMaxTurns        = 127
	DefaultAgentModelMaxOutputTokens = 16000
	DefaultAgentModelHistoryBytes    = 96 * 1024
	DefaultAgentModelBufferBytes     = 48 * 1024
)

func defaultAgentModelCompaction(model string) AgentModelCompaction {
	// Known reasoning models receive profiles tuned for their context and pricing.
	switch model {
	case "gpt-5.6", "gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol", "openai/gpt-5.6", "openai/gpt-5.6-luna", "openai/gpt-5.6-terra", "openai/gpt-5.6-sol", "gpt-5.3-codex", "openai/gpt-5.3-codex":
		return AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: 384 * 1024, BufferBytes: 192 * 1024}
	case "claude-sonnet-5", "claude-opus-5", "anthropic/claude-sonnet-5", "anthropic/claude-opus-5":
		return AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: 512 * 1024, BufferBytes: 240 * 1024}
	case "gemini-3.1-pro-preview", "google/gemini-3.1-pro-preview", "grok-4.6", "x-ai/grok-4.6":
		return AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: 256 * 1024, BufferBytes: 120 * 1024}
	default:
		return AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: DefaultAgentModelHistoryBytes, BufferBytes: DefaultAgentModelBufferBytes}
	}
}

func ResolveAgentProviders(document configschema.GatehouseConfig) (error, []AgentProvider) {
	if document.AgentProviders == nil {
		return nil, []AgentProvider{}
	}
	providers := make([]AgentProvider, 0, len(*document.AgentProviders))
	aliases := make(map[string]struct{}, len(*document.AgentProviders))
	for index, configured := range *document.AgentProviders {
		if !keychainID.MatchString(configured.Alias) {
			return fmt.Errorf("agent_providers[%d].alias must match %q", index, keychainID.String()), nil
		}
		if _, exists := aliases[configured.Alias]; exists {
			return fmt.Errorf("agent_providers[%d].alias %q is duplicated", index, configured.Alias), nil
		}
		if configured.Revision <= 0 {
			return fmt.Errorf("agent_providers[%d] has an invalid revision or protocol", index), nil
		}
		enabled := configured.Enabled == nil || *configured.Enabled
		provider := AgentProvider{Alias: configured.Alias, Revision: configured.Revision, Protocol: configured.Protocol, Enabled: enabled}
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
		aliases[configured.Alias] = struct{}{}
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(left, right int) bool { return providers[left].Alias < providers[right].Alias })
	return nil, providers
}

func ResolveAgentModels(document configschema.GatehouseConfig, providers []AgentProvider) (error, []AgentModel) {
	if document.AgentModels == nil {
		return nil, []AgentModel{}
	}
	providerProtocols := make(map[string]string, len(providers))
	for _, provider := range providers {
		providerProtocols[provider.Alias] = provider.Protocol
	}
	models := make([]AgentModel, 0, len(*document.AgentModels))
	aliases := make(map[string]struct{}, len(*document.AgentModels))
	for index, configured := range *document.AgentModels {
		if !keychainID.MatchString(configured.Alias) || configured.Revision <= 0 || strings.TrimSpace(configured.Model) == "" {
			return fmt.Errorf("agent_models[%d] is invalid", index), nil
		}
		if _, exists := aliases[configured.Alias]; exists {
			return fmt.Errorf("agent_models[%d].alias %q is duplicated", index, configured.Alias), nil
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
		compaction, err := resolveAgentModelCompaction(configured.Model, configured.Compaction)
		if err != nil {
			return fmt.Errorf("agent_models[%d].compaction: %w", index, err), nil
		}
		maxTurns := DefaultAgentModelMaxTurns
		if configured.MaxTurns != nil {
			maxTurns = *configured.MaxTurns
		}
		if maxTurns <= 0 {
			return fmt.Errorf("agent_models[%d].max_turns must be positive", index), nil
		}
		maxOutputTokens := DefaultAgentModelMaxOutputTokens
		if configured.MaxOutputTokens != nil {
			maxOutputTokens = *configured.MaxOutputTokens
		}
		if maxOutputTokens <= 0 {
			return fmt.Errorf("agent_models[%d].max_output_tokens must be positive", index), nil
		}
		enabled := configured.Enabled == nil || *configured.Enabled
		aliases[configured.Alias] = struct{}{}
		models = append(models, AgentModel{Alias: configured.Alias, Revision: configured.Revision, ProviderAlias: configured.Provider, Model: configured.Model, Parameters: string(encoded), Compaction: compaction, MaxTurns: maxTurns, MaxOutputTokens: maxOutputTokens, Enabled: enabled})
	}
	sort.Slice(models, func(left, right int) bool { return models[left].Alias < models[right].Alias })
	return nil, models
}

func resolveAgentModelCompaction(model string, configured *configschema.GatehouseConfigAgentModelsValuesCompaction) (AgentModelCompaction, error) {
	compaction := defaultAgentModelCompaction(model)
	if configured == nil {
		return compaction, nil
	}
	compaction.Algorithm = configured.Algorithm
	if configured.HistoryBytes != nil {
		compaction.HistoryBytes = *configured.HistoryBytes
	}
	if configured.BufferBytes != nil {
		compaction.BufferBytes = *configured.BufferBytes
	}
	if compaction.Algorithm != "mcmtr" {
		return AgentModelCompaction{}, fmt.Errorf("algorithm %q is unsupported", compaction.Algorithm)
	}
	if compaction.HistoryBytes <= 0 {
		return AgentModelCompaction{}, fmt.Errorf("history_bytes must be positive")
	}
	if compaction.BufferBytes <= 0 || compaction.BufferBytes > compaction.HistoryBytes/2 {
		return AgentModelCompaction{}, fmt.Errorf("buffer_bytes must be positive and at most one half of history_bytes")
	}
	return compaction, nil
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
		modelEnabled[model.Alias] = model.Enabled
	}
	agents := []WorkspaceAgent{}
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Agents == nil {
			continue
		}
		seen := make(map[string]struct{}, len(*workspace.Agents))
		defaults := 0
		for agentIndex, configured := range *workspace.Agents {
			if !workspaceAgentAlias.MatchString(configured.Alias) {
				return fmt.Errorf("workspaces[%d].agents[%d].alias is invalid", workspaceIndex, agentIndex), nil
			}
			if _, exists := seen[configured.Alias]; exists {
				return fmt.Errorf("workspaces[%d].agents[%d].alias %q is duplicated", workspaceIndex, agentIndex, configured.Alias), nil
			}
			modelIsEnabled, exists := modelEnabled[configured.Model]
			if !exists || configured.Revision <= 0 {
				return fmt.Errorf("workspaces[%d].agents[%d] is invalid", workspaceIndex, agentIndex), nil
			}
			enabled := configured.Enabled == nil || *configured.Enabled
			if enabled && !modelIsEnabled {
				return fmt.Errorf("workspaces[%d].agents[%d].model %q is disabled", workspaceIndex, agentIndex, configured.Model), nil
			}
			isDefault := configured.Default != nil && *configured.Default
			if isDefault && !enabled {
				return fmt.Errorf("workspaces[%d].agents[%d] default agent is disabled", workspaceIndex, agentIndex), nil
			}
			if enabled && isDefault {
				defaults++
			}
			rateLimits, err := resolveWorkspaceAgentRateLimits(configured.RateLimits)
			if err != nil {
				return fmt.Errorf("workspaces[%d].agents[%d].rate_limits: %w", workspaceIndex, agentIndex, err), nil
			}
			seen[configured.Alias] = struct{}{}
			agents = append(agents, WorkspaceAgent{WorkspaceID: workspace.Alias, Alias: configured.Alias, ModelAlias: configured.Model, Revision: configured.Revision, Label: configured.Label, SystemPrompt: configured.SystemPrompt, Prelude: configured.Prelude, RateLimits: rateLimits, Default: isDefault, Enabled: enabled})
		}
		if defaults > 1 {
			return fmt.Errorf("workspaces[%d].agents configures multiple defaults", workspaceIndex), nil
		}
	}
	sort.Slice(agents, func(left, right int) bool {
		if agents[left].WorkspaceID == agents[right].WorkspaceID {
			return agents[left].Alias < agents[right].Alias
		}
		return agents[left].WorkspaceID < agents[right].WorkspaceID
	})
	return nil, agents
}

func resolveWorkspaceAgentRateLimits(value any) (*WorkspaceAgentRateLimits, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode rate limits: %w", err)
	}
	limits := WorkspaceAgentRateLimits{}
	if err := json.Unmarshal(encoded, &limits); err != nil {
		return nil, fmt.Errorf("decode rate limits: %w", err)
	}
	if limits.Empty() {
		return nil, nil
	}
	if err := ValidateWorkspaceAgentRateLimits(&limits); err != nil {
		return nil, err
	}
	return &limits, nil
}
