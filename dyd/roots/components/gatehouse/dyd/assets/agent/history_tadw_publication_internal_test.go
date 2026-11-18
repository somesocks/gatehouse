package agent

import (
	"bytes"
	"encoding/json"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

// Expose the existing selection scope for integration-test expectations.
func TADWHistoryCacheScopeForTest(session model.SessionRef, selected *database.WorkspaceAgentModel, project bool) (database.ContextCacheHintScope, error) {
	scope, err := tadwHistoryCacheScopeForAgent(session, selected, project)
	return scope.databaseScope(), err
}

func TestTADWPublicationScopeMatchesSentWire(t *testing.T) {
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		for _, emptyPrompt := range []bool{false, true} {
			name := protocol
			if emptyPrompt {
				name += "/empty-prompt"
			}
			t.Run(name, func(t *testing.T) {
				selected, prepared, transcript, limits, hints := tadwProviderFixture(t, protocol)
				if emptyPrompt {
					text := ""
					selected.SystemPromptReplacement = &text
					prepared.Scope = tadwPreparationScope(t, prepared.Scope.Session, selected, false)
				}
				err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints)
				if err != nil {
					t.Fatal(err)
				}
				original, _ := json.Marshal(request.body())
				scope, err := tadwHistoryCacheScopeForSentRequest(prepared.Scope.Session, selected, request)
				if err != nil || scope != prepared.Scope {
					t.Fatal("publication must match the prepared prefix", scope, prepared.Scope, err)
				}
				after, _ := json.Marshal(request.body())
				if !bytes.Equal(original, after) {
					t.Fatal("scope derivation mutated the wire request")
				}
				// Reservations can lower output limits after preparation. Publication uses
				// the actual wire fields, not the selected model's configured limit.
				capped := selected.MaxOutputTokens / 2
				if request.Chat != nil {
					request.Chat.MaxTokens = capped
				} else {
					request.Responses.MaxOutputTokens = capped
				}
				scope, err = tadwHistoryCacheScopeForSentRequest(prepared.Scope.Session, selected, request)
				if err != nil {
					t.Fatal(err)
				}
				expectedSelected := *selected
				expectedSelected.MaxOutputTokens = capped
				expected := tadwPreparationScope(t, prepared.Scope.Session, &expectedSelected, false)
				if scope != expected {
					t.Fatal("publication did not fingerprint the effective output cap", scope, expected)
				}
				if protocol == config.AgentProviderProtocolOpenAICodex {
					if scope.Profile != prepared.Scope.Profile {
						t.Fatal("a field absent from Codex wire changed its profile")
					}
				} else if scope.Profile == prepared.Scope.Profile {
					t.Fatal("effective output cap must change the profile")
				}
				cappedBody, _ := json.Marshal(request.body())
				// Add an event before the separate footer; neither content nor message
				// count is part of the alignment fingerprint.
				if request.Chat != nil {
					last := len(request.Chat.Messages) - 1
					footer := request.Chat.Messages[last]
					request.Chat.Messages[last] = openAICompatibleMessage{Role: "user", Content: "new event"}
					request.Chat.Messages = append(request.Chat.Messages, footer)
				} else {
					last := len(request.Responses.Input) - 1
					footer := request.Responses.Input[last]
					request.Responses.Input[last] = openAIResponsesMessage("user", "new event")
					request.Responses.Input = append(request.Responses.Input, footer)
				}
				unchanged, err := tadwHistoryCacheScopeForSentRequest(prepared.Scope.Session, selected, request)
				if err != nil || unchanged != scope {
					t.Fatal("growing transcript changed publication compatibility", err)
				}
				if bytes.Equal(original, cappedBody) && protocol != config.AgentProviderProtocolOpenAICodex {
					t.Fatal("fixture did not change the effective cap")
				}
			})
		}
	}
}
