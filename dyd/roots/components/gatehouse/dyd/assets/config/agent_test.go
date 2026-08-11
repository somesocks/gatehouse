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
