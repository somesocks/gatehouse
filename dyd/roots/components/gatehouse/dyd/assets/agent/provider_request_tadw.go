package agent

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"gatehouse/database"
)

// The conversation is one user message containing history XML followed by
// retained tooling XML, with no added envelope/separator or native tool blocks.
// Bump this internal salt if that provider-input packaging changes.
const tadwProviderConversationVersion = "tadw-single-user-xml-v1"

// agentProviderRequestPrepareTADW assembles one attempt's wire request.
// The inputs must come from history preparation and request-tooling assembly.
// It does not select/roll windows, hydrate/render events, access DBOS/storage,
// send requests or publish hints. Treat selected as immutable during this call.
func agentProviderRequestPrepareTADW(selected *database.WorkspaceAgentModel, projectChat bool, prepared tadwPreparedHistory, transcript tadwTranscript, limits tadwWindowConfig) (error, agentProviderRequest) {
	scope, request, err := tadwHistoryProviderTemplate(prepared.Scope.Session, selected, projectChat)
	if err != nil {
		return err, agentProviderRequest{}
	}
	if prepared.Agent != selected.Ref || scope != prepared.Scope {
		return fmt.Errorf("prepare TADW provider request: selected configuration does not match prepared history"), agentProviderRequest{}
	}
	budget, err := tadwFixedHistoryBudget(prepared.History, limits)
	if err != nil {
		return err, agentProviderRequest{}
	}
	if !slices.Equal(transcript.History, prepared.History.History) {
		return fmt.Errorf("prepare TADW provider request: fixed history fragments changed"), agentProviderRequest{}
	}
	seen := make(map[string]bool, len(transcript.History)+len(transcript.Tooling))
	for _, fragment := range transcript.History {
		if !utf8.ValidString(fragment.XML) {
			return fmt.Errorf("prepare TADW provider request: invalid UTF-8 history fragment %q", fragment.ID), agentProviderRequest{}
		}
		seen[fragment.ID] = true
	}
	if len(transcript.Tooling) == 0 {
		return fmt.Errorf("prepare TADW provider request: tooling must include the owning request"), agentProviderRequest{}
	}
	var used int64
	for _, fragment := range transcript.Tooling {
		if fragment.ID == "" || fragment.XML == "" || !utf8.ValidString(fragment.XML) {
			return fmt.Errorf("prepare TADW provider request: invalid tooling fragment %q", fragment.ID), agentProviderRequest{}
		}
		if seen[fragment.ID] {
			return fmt.Errorf("duplicate TADW context event %q", fragment.ID), agentProviderRequest{}
		}
		if fragment.Bytes() > budget-used {
			return &tadwToolingBudgetError{BudgetBytes: budget, UsedBytes: used, EventBytes: fragment.Bytes(), EventID: fragment.ID}, agentProviderRequest{}
		}
		used += fragment.Bytes()
		seen[fragment.ID] = true
	}
	message := openAICompatibleMessage{Role: "user", Content: transcript.String()}
	if request.Chat != nil {
		request.Chat.Messages = append(request.Chat.Messages, message)
	} else {
		request.Responses.Input = openAIResponsesInput([]openAICompatibleMessage{message})
	}
	return nil, request
}
