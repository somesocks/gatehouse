package agent

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func tadwPreparationTestAgent(session model.SessionRef) *database.WorkspaceAgentModel {
	endpoint, key, prompt := "https://provider.invalid/inference", "TADW_CREDENTIAL_CIPHERTEXT_SENTINEL", "TADW_SYSTEM_PROMPT_SENTINEL"
	return &database.WorkspaceAgentModel{
		Ref: model.WorkspaceAgentRef{Workspace: session.Workspace, Id: fmt.Sprintf("wag_%026d", 1)}, Alias: "assistant", Revision: 7,
		AgentModel: model.AgentModelRef{Id: fmt.Sprintf("amd_%026d", 1)}, ProviderID: fmt.Sprintf("apr_%026d", 1),
		Protocol: "openai-responses", Endpoint: &endpoint, Keychain: &model.KeychainRef{Id: "keychain", Version: 1}, APIKey: &key,
		Model: "model-x", Parameters: `{}`, Compaction: "mcmtr-v4", MaxTurns: 10, MaxOutputTokens: 123, ResultInlineBytes: 512, SystemPrompt: &prompt,
	}
}

func tadwCheckpointTestCacheScope(t *testing.T, testCase string, session model.SessionRef) tadwHistoryCacheScope {
	t.Helper()
	if strings.HasPrefix(testCase, "cache-prepared-") {
		return tadwPreparationScope(t, session, tadwPreparationTestAgent(session), false)
	}
	return tadwCacheTestScope(session)
}

func tadwPreparationScope(t *testing.T, session model.SessionRef, selected *database.WorkspaceAgentModel, project bool) tadwHistoryCacheScope {
	t.Helper()
	scope, err := tadwHistoryCacheScopeForAgent(session, selected, project)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestTADWHistoryPreparationDeterministicAndSecretFree(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		t.Run(protocol, func(t *testing.T) {
			selected := tadwPreparationTestAgent(session)
			selected.Protocol = protocol
			if protocol == config.AgentProviderProtocolOpenAICodex {
				endpoint := config.OpenAICodexEndpoint
				selected.Endpoint = &endpoint
			}
			before, _ := json.Marshal(selected)
			scope := tadwPreparationScope(t, session, selected, false)
			if scope != tadwPreparationScope(t, session, selected, false) {
				t.Fatal("nondeterministic scope")
			}
			for _, key := range []string{scope.CacheScope, scope.Profile} {
				decoded, err := hex.DecodeString(key)
				if err != nil || len(decoded) != 32 {
					t.Fatal("not SHA-256", key)
				}
			}
			encoded, _ := json.Marshal(scope)
			for _, private := range []string{*selected.APIKey, *selected.SystemPrompt, *selected.Endpoint, selected.Keychain.Id} {
				if strings.Contains(string(encoded), private) {
					t.Fatal("raw configuration/credential leaked")
				}
			}
			after, _ := json.Marshal(selected)
			if string(before) != string(after) {
				t.Fatal("mutated selected configuration")
			}
			err, template := agentProviderRequestPrepare(selected, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := tadwPreparationProfileFingerprint(template)
			if err != nil || profile != scope.Profile {
				t.Fatal("not derived from actual provider template", profile, scope.Profile, err)
			}
			// Neither transcript content nor selection budgets are inputs to the scope.
			another := scope
			another.Session.Id = "another-session"
			otherSession := session
			otherSession.Id = "another-session"
			if another != tadwPreparationScope(t, otherSession, selected, false) {
				t.Fatal("session mixed into compatibility hashes")
			}
		})
	}
}

func TestTADWHistoryPreparationEffectiveConfigurationChanges(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	base := tadwPreparationScope(t, session, tadwPreparationTestAgent(session), false)
	cases := []struct {
		name           string
		mutate         func(*database.WorkspaceAgentModel)
		scope, profile bool
	}{
		{"workspace-agent", func(s *database.WorkspaceAgentModel) { s.Ref.Id = fmt.Sprintf("wag_%026d", 2) }, true, true},
		{"model-reference", func(s *database.WorkspaceAgentModel) { s.AgentModel.Id = fmt.Sprintf("amd_%026d", 2) }, true, false},
		{"provider-reference", func(s *database.WorkspaceAgentModel) { s.ProviderID = fmt.Sprintf("apr_%026d", 2) }, true, false},
		{"endpoint", func(s *database.WorkspaceAgentModel) { *s.Endpoint = "https://another.invalid/inference" }, true, false},
		{"protocol", func(s *database.WorkspaceAgentModel) { s.Protocol = "openai-chat-completions" }, true, true},
		{"wire-model-same-IDs", func(s *database.WorkspaceAgentModel) { s.Model = "model-y" }, true, true},
		{"credential-bundle", func(s *database.WorkspaceAgentModel) { *s.APIKey = "different-encrypted-account" }, true, false},
		{"keychain-reference", func(s *database.WorkspaceAgentModel) { s.Keychain.Id = "another-keychain" }, true, false},
		{"keychain-version", func(s *database.WorkspaceAgentModel) { s.Keychain.Version++ }, true, false},
		{"prompt-same-IDs", func(s *database.WorkspaceAgentModel) { *s.SystemPrompt = "different instructions" }, false, true},
		{"replacement", func(s *database.WorkspaceAgentModel) { v := "invocation override"; s.SystemPromptReplacement = &v }, false, true},
		{"empty-replacement", func(s *database.WorkspaceAgentModel) { v := ""; s.SystemPromptReplacement = &v }, false, true},
		{"alias-in-effective-prompt", func(s *database.WorkspaceAgentModel) { s.Alias = "different-assistant" }, false, true},
		{"inline-limit-in-effective-prompt", func(s *database.WorkspaceAgentModel) { s.ResultInlineBytes = 1024 }, false, true},
		{"reasoning", func(s *database.WorkspaceAgentModel) { s.Parameters = `{"reasoning_effort":"high"}` }, false, true},
		{"output-setting", func(s *database.WorkspaceAgentModel) { s.MaxOutputTokens = 456 }, false, true},
		{"binding-revision-only", func(s *database.WorkspaceAgentModel) { s.Revision++ }, false, false},
		{"provider-label-only", func(s *database.WorkspaceAgentModel) { v := "renamed"; s.ProviderAlias = &v }, false, false},
		{"unused-settings", func(s *database.WorkspaceAgentModel) {
			s.Compaction = "different"
			s.MaxTurns++
			v := "different prelude"
			s.Prelude = &v
		}, false, false},
		{"unused-parameters", func(s *database.WorkspaceAgentModel) { s.Parameters = `{"ignored":"value","reasoning_effort":""}` }, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			selected := tadwPreparationTestAgent(session)
			c.mutate(selected)
			changed := tadwPreparationScope(t, session, selected, false)
			if (changed.CacheScope != base.CacheScope) != c.scope || (changed.Profile != base.Profile) != c.profile {
				t.Fatal("unexpected compatibility change", changed, base)
			}
		})
	}
	selected := tadwPreparationTestAgent(session)
	project := tadwPreparationScope(t, session, selected, true)
	if project.CacheScope != base.CacheScope || project.Profile == base.Profile {
		t.Fatal("project prompt not represented")
	}
}

func TestTADWHistoryPreparationUsesEffectiveReplacementAndWireSettings(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		for _, replacement := range []string{"", "replacement only"} {
			selected := tadwPreparationTestAgent(session)
			selected.Protocol = protocol
			selected.SystemPromptReplacement = &replacement
			if protocol == config.AgentProviderProtocolOpenAICodex {
				endpoint := config.OpenAICodexEndpoint
				selected.Endpoint = &endpoint
			}
			before := tadwPreparationScope(t, session, selected, false)
			*selected.SystemPrompt = "unused append changed"
			selected.Alias = "unused-alias"
			selected.ResultInlineBytes++
			if after := tadwPreparationScope(t, session, selected, true); after != before {
				t.Fatal("unused configuration bypassed replacement", protocol, replacement)
			}
			if protocol == config.AgentProviderProtocolOpenAICodex {
				selected.MaxOutputTokens++
				if after := tadwPreparationScope(t, session, selected, false); after != before {
					t.Fatal("hashed a token setting absent from Codex wire body")
				}
			}
		}
	}
	// Input JSON spelling/order does not matter if the effective request is equal.
	a := tadwPreparationTestAgent(session)
	b := tadwPreparationTestAgent(session)
	a.Parameters = `{"ignored":1,"reasoning_effort":"medium"}`
	b.Parameters = `{ "reasoning_effort": "medium", "ignored": 2 }`
	if tadwPreparationScope(t, session, a, false) != tadwPreparationScope(t, session, b, false) {
		t.Fatal("hashed raw parameter spelling")
	}
}

func TestTADWHistoryPreparationFingerprintsActualToolsAndResponseFormat(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		for _, mutation := range []string{"tool-description", "tool-schema", "response-schema"} {
			selected := tadwPreparationTestAgent(session)
			selected.Protocol = protocol
			if protocol == config.AgentProviderProtocolOpenAICodex {
				endpoint := config.OpenAICodexEndpoint
				selected.Endpoint = &endpoint
			}
			err, template := agentProviderRequestPrepare(selected, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			before, err := tadwPreparationProfileFingerprint(template)
			if err != nil {
				t.Fatal(err)
			}
			if template.Chat != nil {
				switch mutation {
				case "tool-description":
					template.Chat.Tools[0].Function.Description += "changed"
				case "tool-schema":
					template.Chat.Tools[0].Function.Parameters = map[string]any{"changed": true}
				case "response-schema":
					template.Chat.ResponseFormat.JSONSchema.Schema = map[string]any{"changed": true}
				}
			} else {
				switch mutation {
				case "tool-description":
					template.Responses.Tools[0].Description += "changed"
				case "tool-schema":
					template.Responses.Tools[0].Parameters = map[string]any{"changed": true}
				case "response-schema":
					template.Responses.Text.Format.Schema = map[string]any{"changed": true}
				}
			}
			after, err := tadwPreparationProfileFingerprint(template)
			if err != nil || after == before {
				t.Fatal("wire definition not represented", protocol, mutation, err)
			}
		}
	}
	// Encoding is structured, not ambiguous delimiter concatenation.
	a, err := tadwPreparationDigest([]string{"a\x00b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := tadwPreparationDigest([]string{"a", "b\x00c"})
	if err != nil || a == b {
		t.Fatal("ambiguous digest", err)
	}
	if _, err := tadwPreparationDigest(func() {}); err == nil {
		t.Fatal("encoding failure ignored")
	}
}

func TestTADWHistoryPreparationValidation(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	mutations := []func(*database.WorkspaceAgentModel){
		func(s *database.WorkspaceAgentModel) { s.Ref.Workspace.Id = "other" },
		func(s *database.WorkspaceAgentModel) { s.Ref.Id = "" },
		func(s *database.WorkspaceAgentModel) { s.AgentModel.Id = "" },
		func(s *database.WorkspaceAgentModel) { s.ProviderID = "" },
		func(s *database.WorkspaceAgentModel) { s.Model = "" },
		func(s *database.WorkspaceAgentModel) { s.Protocol = "builtin" },
		func(s *database.WorkspaceAgentModel) { s.Protocol = "unsupported" },
		func(s *database.WorkspaceAgentModel) { s.Endpoint = nil },
		func(s *database.WorkspaceAgentModel) { *s.Endpoint = "" },
		func(s *database.WorkspaceAgentModel) { s.Keychain = nil },
		func(s *database.WorkspaceAgentModel) { s.Keychain.Version = 0 },
		func(s *database.WorkspaceAgentModel) { s.APIKey = nil },
		func(s *database.WorkspaceAgentModel) { *s.APIKey = "" },
		func(s *database.WorkspaceAgentModel) { s.Parameters = `invalid` },
		func(s *database.WorkspaceAgentModel) { s.Parameters = `{"reasoning_effort":"invalid"}` },
		func(s *database.WorkspaceAgentModel) {
			s.Protocol = "openai-chat-completions"
			s.Parameters = `{"reasoning_effort":"high"}`
		},
		func(s *database.WorkspaceAgentModel) { s.Protocol = config.AgentProviderProtocolOpenAICodex },
	}
	for i, mutate := range mutations {
		selected := tadwPreparationTestAgent(session)
		mutate(selected)
		got, err := tadwHistoryCacheScopeForAgent(session, selected, false)
		if err == nil || !reflect.DeepEqual(got, tadwHistoryCacheScope{}) {
			t.Fatal(i, got, err)
		}
	}
	if _, err := tadwHistoryCacheScopeForAgent(session, nil, false); err == nil {
		t.Fatal("nil selection")
	}
	selected := tadwPreparationTestAgent(session)
	if got, err := prepareTADWHistory(nil, nil, model.SessionEventRef{Session: session, Id: "anchor"}, selected, false, tadwWindowConfig{}); err == nil || !reflect.DeepEqual(got, tadwPreparedHistory{}) {
		t.Fatal(got, err)
	}
	empty := session
	empty.Id = ""
	if _, err := tadwHistoryCacheScopeForAgent(empty, selected, false); err == nil {
		t.Fatal("empty session")
	}
}
