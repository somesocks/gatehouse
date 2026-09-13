package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFileAcceptsBuiltinDummyFixedReplyProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
`), 0o600); err != nil {
		t.Fatal(err)
	}

	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, state := ResolveState(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.AgentProviders) != 1 || state.AgentProviders[0].Protocol != agentProviderProtocolBuiltin || state.AgentProviders[0].BaseURL != nil || state.AgentProviders[0].Keychain != nil {
		t.Fatalf("builtin provider = %#v", state.AgentProviders)
	}
}

func TestValidateFileRejectsInvalidBuiltinProviderModels(t *testing.T) {
	for name, contents := range map[string]string{
		"credentials": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
    base_url: https://example.test
agent_models: []
`,
		"model": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.echo
    parameters:
      text: Fallback reply.
`,
		"parameters": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: ""
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err, _ := ValidateFile(path); err == nil {
				t.Fatal("ValidateFile() accepted an invalid builtin provider model")
			}
		})
	}
}

func TestValidateFileRejectsChatCompletionsReasoningWithTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
agent_providers:
  - alias: openai
    revision: 1
    protocol: openai-chat-completions
    base_url: https://example.test/v1
    api_key:
      sources: [env:OPENAI_API_KEY]
agent_models:
  - alias: chat
    revision: 1
    provider: openai
    model: gpt-5.6-luna
    parameters:
      reasoning_effort: low
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err, _ := ValidateFile(path); err == nil {
		t.Fatal("ValidateFile() accepted unsupported Chat Completions reasoning effort")
	}
}

func TestResolveAgentModelLimits(t *testing.T) {
	for name, contents := range map[string]string{
		"default": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
`,
		"configured": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
    max_turns: 3
    max_output_tokens: 2000
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			err, document := ValidateFile(path)
			if err != nil {
				t.Fatal(err)
			}
			err, state := ResolveState(document)
			if err != nil || len(state.AgentModels) != 1 {
				t.Fatalf("ResolveState() = (%#v, %v)", state.AgentModels, err)
			}
			wantTurns, wantOutputTokens := DefaultAgentModelMaxTurns, DefaultAgentModelMaxOutputTokens
			if name == "configured" {
				wantTurns, wantOutputTokens = 3, 2000
			}
			model := state.AgentModels[0]
			if model.MaxTurns != wantTurns || model.MaxOutputTokens != wantOutputTokens {
				t.Fatalf("model limits = (%d, %d), want (%d, %d)", model.MaxTurns, model.MaxOutputTokens, wantTurns, wantOutputTokens)
			}
		})
	}
}

func TestDefaultAgentModelCompaction(t *testing.T) {
	for model, want := range map[string]AgentModelCompaction{
		"gpt-5.6-luna":                  {Algorithm: "mcmtr", HistoryBytes: 384 * 1024, BufferBytes: 192 * 1024},
		"openai/gpt-5.3-codex":          {Algorithm: "mcmtr", HistoryBytes: 384 * 1024, BufferBytes: 192 * 1024},
		"anthropic/claude-sonnet-5":     {Algorithm: "mcmtr", HistoryBytes: 512 * 1024, BufferBytes: 240 * 1024},
		"google/gemini-3.1-pro-preview": {Algorithm: "mcmtr", HistoryBytes: 256 * 1024, BufferBytes: 120 * 1024},
		"x-ai/grok-4.6":                 {Algorithm: "mcmtr", HistoryBytes: 256 * 1024, BufferBytes: 120 * 1024},
		"unknown":                       {Algorithm: "mcmtr", HistoryBytes: DefaultAgentModelHistoryBytes, BufferBytes: DefaultAgentModelBufferBytes},
	} {
		got, err := resolveAgentModelCompaction(model, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("resolveAgentModelCompaction(%q, nil) = %#v, want %#v", model, got, want)
		}
	}
}

func TestResolveAgentModelCompaction(t *testing.T) {
	for name, contents := range map[string]string{
		"default": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
		`,
		"configured MCMTR": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
    compaction:
      algorithm: mcmtr
      history_bytes: 1000
      buffer_bytes: 100
		`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)), 0o600); err != nil {
				t.Fatal(err)
			}
			err, document := ValidateFile(path)
			if err != nil {
				t.Fatal(err)
			}
			err, state := ResolveState(document)
			if err != nil || len(state.AgentModels) != 1 {
				t.Fatalf("ResolveState() = (%#v, %v)", state.AgentModels, err)
			}
			want := AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: DefaultAgentModelHistoryBytes, BufferBytes: DefaultAgentModelBufferBytes}
			if name == "configured MCMTR" {
				want = AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: 1000, BufferBytes: 100}
			}
			if state.AgentModels[0].Compaction != want {
				t.Fatalf("compaction = %#v, want %#v", state.AgentModels[0].Compaction, want)
			}
		})
	}
}

func TestResolveWorkspaceAgentsSystemPrompt(t *testing.T) {
	for name, contents := range map[string]string{
		"default": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - alias: engineering
    agents:
      - alias: fallback
        model: fallback
        revision: 1
        priority: 1
`,
		"custom": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - alias: engineering
    agents:
      - alias: fallback
        model: fallback
        revision: 1
        priority: 1
        system_prompt: Custom instructions.
`,
		"empty": `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - alias: engineering
    agents:
      - alias: fallback
        model: fallback
        revision: 1
        priority: 1
        system_prompt: ""
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			err, document := ValidateFile(path)
			if err != nil {
				t.Fatal(err)
			}
			err, state := ResolveState(document)
			if err != nil || len(state.WorkspaceAgents) != 1 {
				t.Fatalf("ResolveState() = (%#v, %v)", state.WorkspaceAgents, err)
			}
			prompt := state.WorkspaceAgents[0].SystemPrompt
			switch name {
			case "default":
				if prompt != nil {
					t.Fatalf("system prompt = %q, want nil", *prompt)
				}
			case "custom":
				if prompt == nil || *prompt != "Custom instructions." {
					t.Fatalf("system prompt = %#v, want custom prompt", prompt)
				}
			case "empty":
				if prompt == nil || *prompt != "" {
					t.Fatalf("system prompt = %#v, want empty prompt", prompt)
				}
			}
		})
	}
}

func TestResolveWorkspaceAgentsLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - alias: engineering
    agents:
      - alias: fallback
        model: fallback
        revision: 1
        label: Fallback assistant
        priority: 1
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, state := ResolveState(document)
	if err != nil || len(state.WorkspaceAgents) != 1 || state.WorkspaceAgents[0].Label == nil || *state.WorkspaceAgents[0].Label != "Fallback assistant" {
		t.Fatalf("ResolveState() = (%#v, %v)", state.WorkspaceAgents, err)
	}
}

func TestValidateFileRejectsNonPositiveAgentModelLimits(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		contents := `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
    max_turns: ` + value + `
`
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err, _ := ValidateFile(path); err == nil {
			t.Fatal("configuration accepted an invalid agent model max_turns")
		}
	}
}

func TestValidateFileRejectsInvalidAgentModelCompaction(t *testing.T) {
	for name, compaction := range map[string]string{
		"unsupported algorithm":               "algorithm: unsupported",
		"nonpositive history":                 "algorithm: mcmtr\n      history_bytes: 0",
		"nonpositive buffer":                  "algorithm: mcmtr\n      buffer_bytes: -1",
		"buffer exceeds high-tier allocation": "algorithm: mcmtr\n      history_bytes: 100\n      buffer_bytes: 51",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			contents := `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
    compaction:
      ` + compaction + `
`
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err, _ := ValidateFile(path); err == nil {
				t.Fatal("configuration accepted invalid agent model compaction")
			}
		})
	}
}

func TestValidateFileRejectsWorkspaceAgentModelLimits(t *testing.T) {
	for name, limit := range map[string]string{
		"max turns":         "max_turns: 1",
		"max input tokens":  "max_input_tokens: 120000",
		"max output tokens": "max_output_tokens: 16000",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			contents := `
api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - alias: engineering
    agents:
      - alias: fallback
        model: fallback
        revision: 1
        priority: 1
        ` + limit + `
`
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err, _ := ValidateFile(path); err == nil {
				t.Fatal("configuration accepted workspace agent model limits")
			}
		})
	}
}
