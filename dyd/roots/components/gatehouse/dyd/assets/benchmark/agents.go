package benchmark

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"gatehouse/benchmarkschema"
)

// These rules belong to the v1 benchmark format. They deliberately do not call
// operational configuration resolvers or apply model-dependent runtime defaults.
var credentialEnvironmentPattern = regexp.MustCompile(`^env:[A-Za-z_][A-Za-z0-9_]*$`)

func validateProvider(index int, provider benchmarkschema.BenchmarkProvider) error {
	location := fmt.Sprintf("agent_providers[%d] %q", index, provider.Alias)
	switch provider.Protocol {
	case "builtin":
		if provider.BaseUrl != nil || provider.ApiKey != nil {
			return fmt.Errorf("%s: builtin providers do not accept base_url or api_key", location)
		}
	case "openai-chat-completions", "openai-responses":
		if provider.BaseUrl == nil || provider.ApiKey == nil {
			return fmt.Errorf("%s: OpenAI providers require base_url and api_key", location)
		}
		parsed, err := url.ParseRequestURI(*provider.BaseUrl)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%s.base_url must be an absolute HTTP URL", location)
		}
		if provider.ApiKey.Keychain != nil && !aliasPattern.MatchString(*provider.ApiKey.Keychain) {
			return fmt.Errorf("%s.api_key.keychain must match %s", location, aliasPattern)
		}
		if len(provider.ApiKey.Sources) == 0 {
			return fmt.Errorf("%s.api_key.sources must not be empty", location)
		}
		seen := map[string]bool{}
		for sourceIndex, source := range provider.ApiKey.Sources {
			if seen[source] {
				return fmt.Errorf("%s.api_key.sources[%d] %q is duplicated", location, sourceIndex, source)
			}
			if source != "stdin:" && !credentialEnvironmentPattern.MatchString(source) {
				return fmt.Errorf("%s.api_key.sources[%d] must be env:VARIABLE_NAME or stdin:", location, sourceIndex)
			}
			seen[source] = true
		}
	default:
		return fmt.Errorf("%s: unsupported protocol %q", location, provider.Protocol)
	}
	return nil
}

func validateModel(index int, model benchmarkschema.BenchmarkModel, providers map[string]benchmarkschema.BenchmarkProvider) error {
	location := fmt.Sprintf("agent_models[%d] %q", index, model.Alias)
	provider, exists := providers[model.Provider]
	if !exists {
		return fmt.Errorf("%s.provider %q is not configured", location, model.Provider)
	}
	if strings.TrimSpace(model.Model) == "" {
		return fmt.Errorf("%s.model must not be empty", location)
	}
	parameters, ok := model.Parameters.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.parameters must be an object", location)
	}
	if provider.Protocol == "builtin" {
		text, ok := parameters["text"].(string)
		if model.Model != "dummy.fixed-reply" || !ok || strings.TrimSpace(text) == "" || len(parameters) != 1 {
			return fmt.Errorf("%s: builtin models require dummy.fixed-reply and a nonempty text parameter", location)
		}
	} else if err := validateModelParameters(provider.Protocol, parameters); err != nil {
		return fmt.Errorf("%s.parameters: %w", location, err)
	}
	if model.MaxTurns != nil && *model.MaxTurns <= 0 {
		return fmt.Errorf("%s.max_turns must be positive", location)
	}
	if model.MaxOutputTokens != nil && *model.MaxOutputTokens <= 0 {
		return fmt.Errorf("%s.max_output_tokens must be positive", location)
	}
	if compaction := model.Compaction; compaction != nil {
		if compaction.HistoryBytes != nil && *compaction.HistoryBytes <= 0 {
			return fmt.Errorf("%s.compaction.history_bytes must be positive", location)
		}
		if compaction.BufferBytes != nil && *compaction.BufferBytes <= 0 {
			return fmt.Errorf("%s.compaction.buffer_bytes must be positive", location)
		}
		if compaction.HistoryBytes != nil && compaction.BufferBytes != nil && *compaction.BufferBytes > *compaction.HistoryBytes/2 {
			return fmt.Errorf("%s.compaction.buffer_bytes must be at most one half of history_bytes", location)
		}
	}
	return nil
}

func validateModelParameters(protocol string, parameters map[string]any) error {
	if len(parameters) == 0 {
		return nil
	}
	effort, ok := parameters["reasoning_effort"].(string)
	if !ok || len(parameters) != 1 {
		return fmt.Errorf("must contain only an optional reasoning_effort string")
	}
	switch effort {
	case "none", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("reasoning_effort %q is invalid", effort)
	}
	if protocol == "openai-chat-completions" && effort != "none" {
		return fmt.Errorf("reasoning_effort must be none for openai-chat-completions tool use")
	}
	return nil
}

func enabled(value *bool) bool {
	return value == nil || *value
}
