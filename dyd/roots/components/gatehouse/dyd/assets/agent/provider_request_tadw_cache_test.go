package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
)

func TestTADWExplicitCacheWire(t *testing.T) {
	for _, tc := range []struct{ protocol, path, model, field, blockType string }{
		{"openai-responses", "/v1/responses", "gpt-5.6-luna", "input", "input_text"},
		{"openai-chat-completions", "/v1/chat/completions", "gpt-6.2-astra", "messages", "text"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			testTADWExplicitCacheWire(t, tc.protocol, "https://api.openai.com"+tc.path, tc.model, tc.field, tc.blockType)
		})
	}
}

func testTADWExplicitCacheWire(t *testing.T, protocol, endpoint, model, field, blockType string) {
	t.Helper()
	selected, prepared, transcript, limits, hints := tadwProviderFixture(t, protocol)
	selected.Model, selected.Endpoint = model, &endpoint
	prepared.Scope = tadwPreparationScope(t, prepared.Scope.Session, selected, false)
	for n := 4; n <= 6; n++ {
		transcript.Tooling = append(transcript.Tooling, tadwRenderedEvent{
			ID: fmt.Sprintf("sev_%026d", n+30), XML: "<tool.success />\n",
		})
	}
	prepared.History, limits = tadwTideBudget(prepared.History, transcript.ToolingBytes()+int64(len(tadwTestHintsXML(t, selected, hints))))
	history := transcript.History[0].ID
	owner, previous, anchor := transcript.Tooling[0].ID, transcript.Tooling[2].ID, transcript.Tooling[1].ID
	end := transcript.Tooling[len(transcript.Tooling)-1].ID
	err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints,
		"rolled-out", previous, previous, history, anchor, owner)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	body, err := json.Marshal(request.body())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	var options openAIPromptCacheMode
	if err := json.Unmarshal(wire["prompt_cache_options"], &options); err != nil || options.Mode != "explicit" {
		t.Fatal("incorrect explicit cache options", err, options)
	}
	var items []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(wire[field], &items); err != nil {
		t.Fatal(err)
	}
	if field == "messages" {
		var system string
		if len(items) == 0 || items[0].Role != "system" || json.Unmarshal(items[0].Content, &system) != nil || system != openAISystemPromptFor(selected, false) {
			t.Fatal("Chat system message must keep its original string content")
		}
		items = items[1:]
	}
	wantText := tadwExpectedUserMessages(transcript, tadwTestHintsXML(t, selected, hints))
	if len(items) != len(wantText) {
		t.Fatal("incorrect item count")
	}
	var marked []int
	for i, item := range items {
		var content []openAITextPart
		if err := json.Unmarshal(item.Content, &content); err != nil {
			t.Fatal(err)
		}
		if item.Role != "user" || len(content) != 1 || content[0].Type != blockType || content[0].Text != wantText[i] {
			t.Fatalf("item %d changed event content or role", i)
		}
		if mark := content[0].Breakpoint; mark != nil {
			if mark.Mode != "explicit" {
				t.Fatal("wrong marker mode", mark)
			}
			marked = append(marked, i)
		}
	}
	if !slices.Equal(marked, []int{0, 2, 3, len(wantText) - 2}) || !slices.Equal(request.tadwBreakpoints, []string{history, anchor, previous, end}) {
		t.Fatal("wrong marked events", marked, request.tadwBreakpoints)
	}
	scope, err := tadwHistoryCacheScopeForSentRequest(prepared.Scope.Session, selected, request)
	if err != nil || scope != prepared.Scope {
		t.Fatal("publication scope diverged from preparation", err)
	}
	err, other := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints)
	if err != nil || !slices.Equal(other.tadwBreakpoints, []string{history, end}) {
		t.Fatal("missing cold-request boundaries", err, other.tadwBreakpoints)
	}
	if otherScope, err := tadwHistoryCacheScopeForSentRequest(prepared.Scope.Session, selected, other); err != nil || otherScope != scope {
		t.Fatal("dynamic markers changed the static profile", err)
	}
}

func TestTADWExplicitCacheUnsupportedEndpointAndLegacy(t *testing.T) {
	for _, tc := range []struct{ protocol, model, endpoint string }{
		{"openai-responses", "gpt-5.6-luna", "https://gateway.example/v1/responses"},
		{"openai-responses", "gpt-5.5-pro", "https://api.openai.com/v1/responses"},
		{"openai-chat-completions", "gpt-6.2-astra", "https://gateway.example/v1/chat/completions"},
		{"openai-chat-completions", "gpt-5.4", "https://api.openai.com/v1/chat/completions"},
		{"openai-chat-completions", "gpt-5.6-luna", "https://api.openai.com/v1/responses"},
		{config.AgentProviderProtocolOpenAICodex, "gpt-6.2-astra", config.OpenAICodexEndpoint},
	} {
		selected, prepared, transcript, limits, hints := tadwProviderFixture(t, tc.protocol)
		selected.Model, selected.Endpoint = tc.model, &tc.endpoint
		prepared.Scope = tadwPreparationScope(t, prepared.Scope.Session, selected, false)
		err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints, transcript.History[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		_, wire := tadwProviderUserMessages(t, request) // Existing wire uses string content.
		if len(request.tadwBreakpoints) != 0 || wire["prompt_cache_options"] != nil {
			t.Fatal("unsupported profile got cache fields", tc)
		}
	}
	for _, tc := range []struct{ protocol, path string }{
		{"openai-responses", "/v1/responses"},
		{"openai-chat-completions", "/v1/chat/completions"},
	} {
		selected, _, _, _, _ := tadwProviderFixture(t, tc.protocol)
		endpoint := "https://api.openai.com" + tc.path
		selected.Model, selected.Endpoint = "gpt-5.6-luna", &endpoint
		err, legacy := agentProviderRequestPrepare(selected, false, []openAICompatibleMessage{{Role: "user", Content: "ordinary MCMTR"}})
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(legacy.body())
		if err != nil || strings.Contains(string(body), "prompt_cache_") {
			t.Fatal("TADW cache fields leaked into MCMTR", err)
		}
	}
}

func TestTADWExplicitCacheWithNoHistory(t *testing.T) {
	selected, prepared, transcript, limits, hints := tadwProviderFixture(t, "openai-responses")
	endpoint := "https://api.openai.com/v1/responses"
	selected.Model, selected.Endpoint = "gpt-5.6-luna", &endpoint
	prepared.Scope = tadwPreparationScope(t, prepared.Scope.Session, selected, false)
	prepared.History = tadwInitialHistory{}
	prepared.History.Window.ToolingBudgetBytes = limits.ContextMaxBytes
	transcript.History, transcript.Tooling = nil, transcript.Tooling[:1]
	err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints, "not-retained", hints.Request.Id)
	if err != nil || !slices.Equal(request.tadwBreakpoints, []string{hints.Request.Id}) {
		t.Fatal("owner should be the only breakpoint before the first tool", err, request.tadwBreakpoints)
	}
	if base := (&tadwRequestContext{request: model.SessionEventRef{Id: hints.Request.Id}}).cacheBaseID(); base != hints.Request.Id {
		t.Fatal("pinned owner should be first when history is empty", base)
	}
}
