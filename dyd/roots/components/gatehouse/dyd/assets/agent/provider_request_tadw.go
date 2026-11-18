package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gatehouse/database"
)

// Each event occupies its own user message, followed by a separate orientation
// message. Bump this salt when provider packaging changes.
const tadwProviderConversationVersion = "tadw-event-user-xml-v3"
const tadwMaxProviderBreakpoints = 4

// OpenAI documents explicit breakpoints from GPT-5.6 onward on its public APIs.
// Compatible gateways and the Codex backend need not support those fields.
func tadwSupportsExplicitCache(selected *database.WorkspaceAgentModel) bool {
	if selected.Endpoint == nil {
		return false
	}
	switch selected.Protocol {
	case "openai-responses":
		if *selected.Endpoint != "https://api.openai.com/v1/responses" {
			return false
		}
	case "openai-chat-completions":
		if *selected.Endpoint != "https://api.openai.com/v1/chat/completions" {
			return false
		}
	default:
		return false
	}
	name, ok := strings.CutPrefix(selected.Model, "gpt-")
	if !ok {
		return false
	}
	version, _, _ := strings.Cut(name, "-")
	major, minor, hasMinor := strings.Cut(version, ".")
	majorNumber, err := strconv.ParseUint(major, 10, 32)
	if err != nil {
		return false
	}
	var minorNumber uint64
	if hasMinor {
		minorNumber, err = strconv.ParseUint(minor, 10, 32)
		if err != nil {
			return false
		}
	}
	return majorNumber > 5 || majorNumber == 5 && minorNumber >= 6
}

// agentProviderRequestPrepareTADW assembles one attempt's wire request.
// The inputs must come from history preparation and request-tooling assembly.
// It does not select/roll windows, hydrate/render events, access DBOS/storage,
// send requests or publish hints. Treat selected as immutable during this call.
func agentProviderRequestPrepareTADW(selected *database.WorkspaceAgentModel, projectChat bool, prepared tadwPreparedHistory, transcript tadwTranscript, limits tadwWindowConfig, hints tadwRequestHints, cacheCandidates ...string) (error, agentProviderRequest) {
	scope, request, err := tadwHistoryProviderTemplate(prepared.Scope.Session, selected, projectChat)
	if err != nil {
		return err, agentProviderRequest{}
	}
	if prepared.Agent != selected.Ref || scope != prepared.Scope {
		return fmt.Errorf("prepare TADW provider request: selected configuration does not match prepared history"), agentProviderRequest{}
	}
	suffix, err := renderTADWRequestHints(selected, hints)
	if err != nil {
		return err, agentProviderRequest{}
	}
	if hints.Request.Session != prepared.Scope.Session {
		return fmt.Errorf("prepare TADW provider request: hints belong to another session"), agentProviderRequest{}
	}
	budget, err := tadwReservedToolingBudget(prepared.History, limits, int64(len(suffix)))
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
	if len(transcript.Tooling) == 0 || transcript.Tooling[0].ID != hints.Request.Id {
		return fmt.Errorf("prepare TADW provider request: tooling must begin with the owning request"), agentProviderRequest{}
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
	var explicit *openAIPromptCacheMode
	if request.Chat != nil {
		explicit = request.Chat.PromptCacheOptions
	} else {
		explicit = request.Responses.PromptCacheOptions
	}
	var marks map[string]bool
	if explicit != nil {
		marks = map[string]bool{transcript.Tooling[len(transcript.Tooling)-1].ID: true}
		if len(transcript.History) != 0 {
			marks[transcript.History[len(transcript.History)-1].ID] = true
		}
		for _, id := range cacheCandidates {
			if len(marks) == tadwMaxProviderBreakpoints {
				break
			}
			if seen[id] {
				marks[id] = true
			}
		}
	}
	appendUser := func(id, content string) {
		var breakpoint *openAIPromptCacheMode
		if marks[id] {
			breakpoint = explicit
			request.tadwBreakpoints = append(request.tadwBreakpoints, id)
		}
		switch {
		case request.Chat != nil:
			message := openAICompatibleMessage{Role: "user", Content: content}
			if explicit != nil {
				message.textPart = &openAITextPart{Type: "text", Text: content, Breakpoint: breakpoint}
			}
			request.Chat.Messages = append(request.Chat.Messages, message)
		case explicit != nil:
			request.Responses.Input = append(request.Responses.Input, tadwExplicitCacheMessage(content, breakpoint))
		default:
			request.Responses.Input = append(request.Responses.Input, openAIResponsesMessage("user", content))
		}
	}
	for _, events := range [][]tadwRenderedEvent{transcript.History, transcript.Tooling} {
		for _, event := range events {
			appendUser(event.ID, event.XML)
		}
	}
	appendUser("", suffix) // Orientation is outside all reusable prefixes.
	return nil, request
}

func tadwExplicitCacheMessage(text string, breakpoint *openAIPromptCacheMode) json.RawMessage {
	encoded, _ := json.Marshal(struct {
		Role    string            `json:"role"`
		Content [1]openAITextPart `json:"content"`
	}{Role: "user", Content: [1]openAITextPart{{Type: "input_text", Text: text, Breakpoint: breakpoint}}})
	return encoded
}
