package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFileAcceptsBuiltinDummyFixedReplyProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
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
  - id: builtin
    revision: 1
    protocol: builtin
    base_url: https://example.test
agent_models: []
`,
		"model": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.echo
    parameters:
      text: Fallback reply.
`,
		"parameters": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
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
  - id: openai
    revision: 1
    protocol: openai-chat-completions
    base_url: https://example.test/v1
    api_key:
      sources: [env:OPENAI_API_KEY]
agent_models:
  - id: chat
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

func TestResolveWorkspaceAgentsMaxTurns(t *testing.T) {
	for name, contents := range map[string]string{
		"default": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
        priority: 1
`,
		"configured": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
        priority: 1
        max_turns: 3
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
			want := DefaultWorkspaceAgentMaxTurns
			if name == "configured" {
				want = 3
			}
			if state.WorkspaceAgents[0].MaxTurns != want {
				t.Fatalf("max turns = %d, want %d", state.WorkspaceAgents[0].MaxTurns, want)
			}
		})
	}
}

func TestResolveWorkspaceAgentsSystemPrompt(t *testing.T) {
	for name, contents := range map[string]string{
		"default": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
        priority: 1
`,
		"custom": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
        priority: 1
        system_prompt: Custom instructions.
`,
		"empty": `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
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
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
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

func TestValidateFileRejectsNonPositiveWorkspaceAgentMaxTurns(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		contents := `
api_version: v1
agent_providers:
  - id: builtin
    revision: 1
    protocol: builtin
agent_models:
  - id: fallback
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters:
      text: Fallback reply.
workspaces:
  - key: engineering
    agents:
      - model: fallback
        priority: 1
        max_turns: ` + value + `
`
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		err, document := ValidateFile(path)
		if err == nil {
			err, _ = ResolveState(document)
		}
		if err == nil {
			t.Fatal("configuration accepted an invalid workspace agent max_turns")
		}
	}
}
