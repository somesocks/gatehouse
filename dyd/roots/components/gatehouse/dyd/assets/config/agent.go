package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gatehouse/configschema"
)

const agentProviderProtocolOpenAICompatible = "openai-compatible"

type AgentProvider struct {
	ID       string
	Revision int
	Protocol string
	BaseURL  string
	Keychain string
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
	Enabled     bool
}

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
		if configured.Revision <= 0 || configured.Protocol != agentProviderProtocolOpenAICompatible {
			return fmt.Errorf("agent_providers[%d] has an invalid revision or protocol", index), nil
		}
		parsed, err := url.ParseRequestURI(configured.BaseUrl)
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
		enabled := configured.Enabled == nil || *configured.Enabled
		ids[configured.Id] = struct{}{}
		providers = append(providers, AgentProvider{ID: configured.Id, Revision: configured.Revision, Protocol: configured.Protocol, BaseURL: configured.BaseUrl, Keychain: keychain, Sources: sources, Enabled: enabled})
	}
	sort.Slice(providers, func(left, right int) bool { return providers[left].ID < providers[right].ID })
	return nil, providers
}

func ResolveAgentModels(document configschema.GatehouseConfig, providers []AgentProvider) (error, []AgentModel) {
	if document.AgentModels == nil { return nil, []AgentModel{} }
	providerIDs := make(map[string]struct{}, len(providers))
	for _, provider := range providers { providerIDs[provider.ID] = struct{}{} }
	models := make([]AgentModel, 0, len(*document.AgentModels))
	ids := make(map[string]struct{}, len(*document.AgentModels))
	for index, configured := range *document.AgentModels {
		if !keychainID.MatchString(configured.Id) || configured.Revision <= 0 || strings.TrimSpace(configured.Model) == "" {
			return fmt.Errorf("agent_models[%d] is invalid", index), nil
		}
		if _, exists := ids[configured.Id]; exists { return fmt.Errorf("agent_models[%d].id %q is duplicated", index, configured.Id), nil }
		if _, exists := providerIDs[configured.Provider]; !exists { return fmt.Errorf("agent_models[%d].provider %q is not configured", index, configured.Provider), nil }
		parameters, ok := configured.Parameters.(map[string]any)
		if !ok { return fmt.Errorf("agent_models[%d].parameters must be an object", index), nil }
		encoded, err := json.Marshal(parameters)
		if err != nil { return fmt.Errorf("encode agent_models[%d].parameters: %w", index, err), nil }
		enabled := configured.Enabled == nil || *configured.Enabled
		ids[configured.Id] = struct{}{}
		models = append(models, AgentModel{ID: configured.Id, Revision: configured.Revision, Provider: configured.Provider, Model: configured.Model, Parameters: string(encoded), Enabled: enabled})
	}
	sort.Slice(models, func(left, right int) bool { return models[left].ID < models[right].ID })
	return nil, models
}

func ResolveWorkspaceAgents(document configschema.GatehouseConfig, models []AgentModel) (error, []WorkspaceAgent) {
	if document.Workspaces == nil { return nil, []WorkspaceAgent{} }
	modelEnabled := make(map[string]bool, len(models))
	for _, model := range models { modelEnabled[model.ID] = model.Enabled }
	agents := []WorkspaceAgent{}
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Agents == nil { continue }
		seen := make(map[string]struct{}, len(*workspace.Agents))
		for agentIndex, configured := range *workspace.Agents {
			if _, exists := seen[configured.Model]; exists { return fmt.Errorf("workspaces[%d].agents[%d].model %q is duplicated", workspaceIndex, agentIndex, configured.Model), nil }
			modelIsEnabled, exists := modelEnabled[configured.Model]
			if !exists || configured.Priority <= 0 { return fmt.Errorf("workspaces[%d].agents[%d] is invalid", workspaceIndex, agentIndex), nil }
			enabled := configured.Enabled == nil || *configured.Enabled
			if enabled && !modelIsEnabled { return fmt.Errorf("workspaces[%d].agents[%d].model %q is disabled", workspaceIndex, agentIndex, configured.Model), nil }
			seen[configured.Model] = struct{}{}
			agents = append(agents, WorkspaceAgent{WorkspaceID: workspace.Id, Model: configured.Model, Priority: configured.Priority, Enabled: enabled})
		}
	}
	sort.Slice(agents, func(left, right int) bool { if agents[left].WorkspaceID == agents[right].WorkspaceID { return agents[left].Model < agents[right].Model }; return agents[left].WorkspaceID < agents[right].WorkspaceID })
	return nil, agents
}
