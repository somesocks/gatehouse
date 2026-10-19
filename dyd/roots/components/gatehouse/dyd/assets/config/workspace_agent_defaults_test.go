package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMultipleWorkspaceAgentDefaults(t *testing.T) {
	for _, test := range []struct {
		name, yaml, json, contains string
		defaults                   map[string]bool
	}{
		{name: "no defaults", yaml: "[{alias: assistant, model: assistant, revision: 1}]", json: `[{"alias":"assistant","model":"assistant","revision":1}]`, defaults: map[string]bool{"assistant": false}},
		{name: "one default", yaml: "[{alias: assistant, model: assistant, revision: 1, default: true}]", json: `[{"alias":"assistant","model":"assistant","revision":1,"default":true}]`, defaults: map[string]bool{"assistant": true}},
		{name: "multiple defaults", yaml: "[{alias: assistant, model: assistant, revision: 1, default: true}, {alias: reviewer, model: assistant, revision: 1, default: true}, {alias: specialist, model: assistant, revision: 1, default: false}, {alias: private, model: assistant, revision: 1}]", json: `[{"alias":"assistant","model":"assistant","revision":1,"default":true},{"alias":"reviewer","model":"assistant","revision":1,"default":true},{"alias":"specialist","model":"assistant","revision":1,"default":false},{"alias":"private","model":"assistant","revision":1}]`, defaults: map[string]bool{"assistant": true, "reviewer": true, "specialist": false, "private": false}},
		{name: "disabled default", yaml: "[{alias: assistant, model: assistant, revision: 1, default: true, enabled: false}]", json: `[{"alias":"assistant","model":"assistant","revision":1,"default":true,"enabled":false}]`, contains: "default agent is disabled"},
	} {
		for _, format := range []string{"yaml", "json"} {
			t.Run(test.name+"/"+format, func(t *testing.T) {
				contents := fmt.Sprintf(`api_version: v1
agent_providers:
  - alias: builtin
    revision: 1
    protocol: builtin
agent_models:
  - alias: assistant
    revision: 1
    provider: builtin
    model: dummy.fixed-reply
    parameters: {text: Hello}
workspaces:
  - alias: engineering
    agents: %s
`, test.yaml)
				if format == "json" {
					contents = fmt.Sprintf(`{"api_version":"v1","agent_providers":[{"alias":"builtin","revision":1,"protocol":"builtin"}],"agent_models":[{"alias":"assistant","revision":1,"provider":"builtin","model":"dummy.fixed-reply","parameters":{"text":"Hello"}}],"workspaces":[{"alias":"engineering","agents":%s}]}`, test.json)
				}
				path := filepath.Join(t.TempDir(), "config."+format)
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
				err, document := ValidateFile(path)
				if test.contains != "" {
					if err == nil || !strings.Contains(err.Error(), test.contains) {
						t.Fatalf("validation error = %v, want %q", err, test.contains)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				err, state := ResolveState(document)
				if err != nil || len(state.WorkspaceAgents) != len(test.defaults) {
					t.Fatalf("resolved bots = %#v, %v", state.WorkspaceAgents, err)
				}
				for _, agent := range state.WorkspaceAgents {
					want, exists := test.defaults[agent.Alias]
					if !exists || agent.Default != want || agent.ModelAlias != "assistant" {
						t.Fatalf("unexpected bot default: %#v", agent)
					}
				}
			})
		}
	}
}
