package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func tadwProviderFixture(t *testing.T, protocol string) (*database.WorkspaceAgentModel, tadwPreparedHistory, tadwTranscript, tadwWindowConfig) {
	t.Helper()
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	selected := tadwPreparationTestAgent(session)
	selected.Protocol = protocol
	if protocol == config.AgentProviderProtocolOpenAICodex {
		endpoint := config.OpenAICodexEndpoint
		selected.Endpoint = &endpoint
	}
	historyEvent := tadwTestEvent("message.text", map[string]interface{}{"text": "café<&\nこんにちは"})
	historyEvent.Ref = model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_%026d", 1)}
	historyEvent.Parent = nil
	history := tadwAdapterRender(t, []model.SessionEvent{historyEvent})
	request := tadwTestEvent("agent.request", map[string]interface{}{"agent": selected.Ref.Id})
	request.Ref = model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_%026d", 10)}
	request.Parent = &historyEvent.Ref
	tool := tadwTestEvent("tool.request", map[string]interface{}{"name": "lisp", "code": "(string/concat \"café<&\" \"!\")", "reason": "résumé<&"})
	tool.Ref = model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_%026d", 20)}
	tool.Parent = &request.Ref
	outcome := tadwTestEvent("tool.success", map[string]interface{}{"output": ""})
	outcome.Ref = model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_%026d", 30)}
	outcome.Parent = &tool.Ref
	tooling := tadwAdapterRender(t, []model.SessionEvent{request, tool, outcome})
	initial := tadwInitialHistory{History: history, Window: tadwHistoryWindow{EventIDs: []string{history[0].ID}, BaseEventID: history[0].ID, HistoryBytes: tadwRenderedBytes(history)}}
	initial, cfg := tadwTideBudget(initial, tadwRenderedBytes(tooling)+100)
	prepared := tadwPreparedHistory{Agent: selected.Ref, Scope: tadwPreparationScope(t, session, selected, false), History: initial}
	return selected, prepared, tadwTranscript{History: slices.Clone(history), Tooling: tooling}, cfg
}

// Inspect the marshalled body, not just the intermediate structs. JSON escaping
// is transport encoding only; decoded user content must be the exact XML bytes.
func tadwProviderUserContent(t *testing.T, request agentProviderRequest) (string, map[string]json.RawMessage) {
	t.Helper()
	encoded, err := json.Marshal(request.body())
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	var messages []struct {
		Role       string          `json:"role"`
		Content    string          `json:"content"`
		ToolCalls  json.RawMessage `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
		Type       string          `json:"type"`
	}
	key := "input"
	if request.Chat != nil {
		key = "messages"
	}
	if err := json.Unmarshal(wire[key], &messages); err != nil {
		t.Fatal(err)
	}
	users := 0
	content := ""
	for index, message := range messages {
		if message.Role == "user" {
			users++
			content = message.Content
			if index != len(messages)-1 {
				t.Fatal("user input is not last")
			}
		} else if request.Chat == nil || message.Role != "system" {
			t.Fatal("unexpected provider role", message.Role)
		}
		if len(message.ToolCalls) != 0 || message.ToolCallID != "" || message.Type != "" {
			t.Fatal("XML was expanded into native tool conversation")
		}
	}
	if users != 1 {
		t.Fatal("want exactly one user message", users)
	}
	return content, wire
}

func TestTADWProviderRequestWireBodiesAndOwnership(t *testing.T) {
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		t.Run(protocol, func(t *testing.T) {
			selected, prepared, transcript, cfg := tadwProviderFixture(t, protocol)
			before, _ := json.Marshal(struct {
				Prepared   tadwPreparedHistory
				Transcript tadwTranscript
				Selected   *database.WorkspaceAgentModel
			}{prepared, transcript, selected})
			err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, cfg)
			if err != nil {
				t.Fatal(err)
			}
			content, wire := tadwProviderUserContent(t, request)
			if content != transcript.String() || int64(len(content)) != transcript.Bytes() || !strings.Contains(content, "café&lt;&amp;") || !strings.HasSuffix(content, "/>\n") {
				t.Fatal("content/bytes changed", content)
			}
			// The existing preparer is the oracle for all protocol-specific static fields.
			err, want := agentProviderRequestPrepare(selected, false, []openAICompatibleMessage{{Role: "user", Content: transcript.String()}})
			if err != nil || !reflect.DeepEqual(request, want) {
				t.Fatal("provider settings drifted", err)
			}
			static := request
			if request.Chat != nil {
				copy := *request.Chat
				copy.Messages = slices.Clone(copy.Messages[:len(copy.Messages)-1])
				if len(copy.Messages) == 0 {
					copy.Messages = nil
				}
				static.Chat = &copy
			} else {
				copy := *request.Responses
				copy.Input = []json.RawMessage{}
				static.Responses = &copy
			}
			profile, err := tadwPreparationProfileFingerprint(static)
			if err != nil || profile != prepared.Scope.Profile {
				t.Fatal("keys do not describe actual static wire fields", err)
			}
			if protocol == config.AgentProviderProtocolOpenAICodex {
				if _, ok := wire["max_output_tokens"]; ok {
					t.Fatal("Codex got public API token limit")
				}
				if string(wire["stream"]) != "true" || string(wire["store"]) != "false" || string(wire["tool_choice"]) != `"auto"` {
					t.Fatal("missing Codex settings")
				}
			}
			encoded, _ := json.Marshal(wire)
			if strings.Contains(string(encoded), *selected.APIKey) {
				t.Fatal("credential in provider body")
			}
			after, _ := json.Marshal(struct {
				Prepared   tadwPreparedHistory
				Transcript tadwTranscript
				Selected   *database.WorkspaceAgentModel
			}{prepared, transcript, selected})
			if string(before) != string(after) {
				t.Fatal("mutated input")
			}
			// A built body owns its content: later slice edits do not rewrite it.
			original := content
			transcript.Tooling[0].XML = "mutated"
			prepared.History.History[0].XML = "mutated"
			if got, _ := tadwProviderUserContent(t, request); got != original {
				t.Fatal("request aliases context")
			}
		})
	}
}

func TestTADWProviderRequestEmptyHistoryAndPromptReplacement(t *testing.T) {
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		for _, replacement := range []string{"", "override only"} {
			selected, prepared, transcript, cfg := tadwProviderFixture(t, protocol)
			selected.SystemPromptReplacement = &replacement
			prepared.Scope = tadwPreparationScope(t, prepared.Scope.Session, selected, true)
			prepared.History = tadwInitialHistory{}
			cfg = tadwWindowConfig{ContextMaxBytes: 10000, HistoryMaxBytes: 5000, HistoryMinBytes: 1}
			prepared.History.Window.ToolingBudgetBytes = cfg.ContextMaxBytes
			transcript.History = nil
			err, request := agentProviderRequestPrepareTADW(selected, true, prepared, transcript, cfg)
			if err != nil {
				t.Fatal(err)
			}
			content, _ := tadwProviderUserContent(t, request)
			if content != transcript.String() {
				t.Fatal("empty history changed tooling")
			}
			if request.Chat != nil {
				want := 1
				if replacement != "" {
					want = 2
				}
				if len(request.Chat.Messages) != want || (want == 2 && request.Chat.Messages[0].Content != replacement) {
					t.Fatal("wrong replacement messages")
				}
			} else if request.Responses.Instructions != replacement {
				t.Fatal("wrong replacement instructions")
			}
		}
	}
}

func TestTADWProviderRequestRejectsConfigurationDrift(t *testing.T) {
	changes := []func(*database.WorkspaceAgentModel){
		func(s *database.WorkspaceAgentModel) { s.Model = "different" },
		func(s *database.WorkspaceAgentModel) { *s.Endpoint = "https://another.invalid" },
		func(s *database.WorkspaceAgentModel) { *s.APIKey = "different-credential" },
		func(s *database.WorkspaceAgentModel) { *s.SystemPrompt = "different prompt" },
		func(s *database.WorkspaceAgentModel) { s.Protocol = "openai-chat-completions" },
		func(s *database.WorkspaceAgentModel) { s.Parameters = `{"reasoning_effort":"high"}` },
		func(s *database.WorkspaceAgentModel) { s.Ref.Id = fmt.Sprintf("wag_%026d", 2) },
	}
	for i, change := range changes {
		selected, prepared, transcript, cfg := tadwProviderFixture(t, "openai-responses")
		change(selected)
		err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, cfg)
		if err == nil || !strings.Contains(err.Error(), "does not match") || !reflect.DeepEqual(request, agentProviderRequest{}) {
			t.Fatal(i, err)
		}
	}
	selected, prepared, transcript, cfg := tadwProviderFixture(t, "openai-responses")
	if err, _ := agentProviderRequestPrepareTADW(selected, true, prepared, transcript, cfg); err == nil {
		t.Fatal("project prompt drift accepted")
	}
	if err, _ := agentProviderRequestPrepareTADW(nil, false, prepared, transcript, cfg); err == nil {
		t.Fatal("nil selection")
	}
}

func TestTADWProviderRequestContextValidationAndBudget(t *testing.T) {
	for _, name := range []string{"scope", "agent", "history-content", "history-order", "history-ids", "accounting", "history-utf8", "tool-utf8", "empty-tool-id", "empty-tool-xml", "duplicate", "overlap", "no-tooling", "overflow", "invalid-limits", "exact", "max-int"} {
		t.Run(name, func(t *testing.T) {
			selected, prepared, transcript, cfg := tadwProviderFixture(t, "openai-responses")
			// Clone to distinguish accidental mutation of both sides of the comparison.
			transcript.History = slices.Clone(transcript.History)
			switch name {
			case "scope":
				prepared.Scope.Profile = "wrong"
			case "agent":
				prepared.Agent.Id = "wrong"
			case "history-content":
				transcript.History[0].XML += "changed"
			case "history-order":
				transcript.History = append(transcript.History, transcript.Tooling[1])
			case "history-ids":
				prepared.History.Window.EventIDs = []string{"wrong"}
			case "accounting":
				prepared.History.Window.HistoryBytes++
			case "history-utf8":
				prepared.History.History[0].XML = "\xff"
				transcript.History[0].XML = "\xff"
				prepared.History.Window.HistoryBytes = 1
				prepared.History.Window.ToolingBudgetBytes = cfg.ContextMaxBytes - 1
			case "tool-utf8":
				transcript.Tooling[0].XML = "\xff"
			case "empty-tool-id":
				transcript.Tooling[0].ID = ""
			case "empty-tool-xml":
				transcript.Tooling[0].XML = ""
			case "duplicate":
				transcript.Tooling[1].ID = transcript.Tooling[0].ID
			case "overlap":
				transcript.Tooling[0].ID = transcript.History[0].ID
			case "no-tooling":
				transcript.Tooling = nil
			case "overflow":
				prepared.History, cfg = tadwTideBudget(prepared.History, transcript.ToolingBytes()-1)
			case "invalid-limits":
				cfg = tadwWindowConfig{}
			case "exact":
				prepared.History, cfg = tadwTideBudget(prepared.History, transcript.ToolingBytes())
			case "max-int":
				cfg.ContextMaxBytes = math.MaxInt64
				prepared.History.Window.ToolingBudgetBytes = cfg.ContextMaxBytes - prepared.History.Window.HistoryBytes
			}
			err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, cfg)
			if name == "exact" || name == "max-int" {
				if err != nil {
					t.Fatal(err)
				}
				content, _ := tadwProviderUserContent(t, request)
				if content != transcript.String() {
					t.Fatal("truncated context")
				}
				return
			}
			if err == nil || !reflect.DeepEqual(request, agentProviderRequest{}) {
				t.Fatal("expected error without partial request", err)
			}
			if name == "overflow" {
				var budget *tadwToolingBudgetError
				if !errors.As(err, &budget) {
					t.Fatal(err)
				}
			}
		})
	}
}

func tadwProviderDatabaseAgent(t *testing.T, store *database.Store, request model.SessionEventRef) *database.WorkspaceAgentModel {
	t.Helper()
	selected := tadwPreparationTestAgent(request.Session)
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET author_agent=? WHERE author_agent='agent'`, selected.Ref.Id); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"agent": selected.Ref.Id})
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=? WHERE id=?`, string(payload), request.Id); err != nil {
		t.Fatal(err)
	}
	return selected
}

func TestTADWProviderRequestGrowingPrefixRolloverAndReconstruction(t *testing.T) {
	for _, protocol := range []string{"openai-chat-completions", "openai-responses", config.AgentProviderProtocolOpenAICodex} {
		t.Run(protocol, func(t *testing.T) {
			store, initial, cfg, request, _ := tadwToolingDB(t)
			selected := tadwProviderDatabaseAgent(t, store, request)
			selected.Protocol = protocol
			if protocol == config.AgentProviderProtocolOpenAICodex {
				endpoint := config.OpenAICodexEndpoint
				selected.Endpoint = &endpoint
			}
			full, err := loadTADWContext(context.Background(), store, initial, request, selected.Ref, tadwToolingThinking(request, 40), cfg)
			if err != nil {
				t.Fatal(err)
			}
			initial, cfg = tadwTideBudget(initial, full.ToolingBytes())
			prepared := tadwPreparedHistory{Agent: selected.Ref, Scope: tadwPreparationScope(t, request.Session, selected, false), History: initial}
			from, prior := "", ""
			var early agentProviderRequest
			historyXML := (tadwTranscript{History: initial.History}).String()
			for _, attempt := range []int{11, 40, 60} {
				window, err := loadTADWToolingWindow(context.Background(), store, initial, request, selected.Ref, tadwToolingThinking(request, attempt), cfg, from)
				if err != nil {
					t.Fatal(err)
				}
				err, wire := agentProviderRequestPrepareTADW(selected, false, prepared, window.Transcript, cfg)
				if err != nil {
					t.Fatal(err)
				}
				content, _ := tadwProviderUserContent(t, wire)
				if !strings.HasPrefix(content, historyXML) || content != window.Transcript.String() {
					t.Fatal("history or whole-event order changed")
				}
				if attempt == 40 {
					if from != window.FromEventID || !strings.HasPrefix(content, prior) {
						t.Fatal("fitting growth lost its prefix")
					}
					early = wire
				}
				if attempt == 60 && (window.FromEventID == "" || window.FromEventID == from || strings.HasPrefix(content, prior)) {
					t.Fatal("fixture did not roll")
				}
				from, prior = window.FromEventID, content
			}
			// Reconstruct an earlier thinking attempt after a later rollover. Its own
			// empty boundary still yields the old full tooling, not the latest suffix.
			restored, err := loadTADWContextFrom(context.Background(), store, initial, request, selected.Ref, tadwToolingThinking(request, 40), cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			err, replayed := agentProviderRequestPrepareTADW(selected, false, prepared, restored, cfg)
			a, _ := json.Marshal(early.body())
			b, _ := json.Marshal(replayed.body())
			if err != nil || string(a) != string(b) {
				t.Fatal("earlier wire body changed after rollover", err)
			}
		})
	}
}

func TestTADWProviderRequestDBOSStateRemainsMinimal(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "tadw-provider-test-worker")
	store, initial, cfg, request, _ := tadwToolingDB(t)
	selected := tadwProviderDatabaseAgent(t, store, request)
	full, err := loadTADWContext(context.Background(), store, initial, request, selected.Ref, tadwToolingThinking(request, 40), cfg)
	if err != nil {
		t.Fatal(err)
	}
	budget := full.ToolingBytes()
	ctx, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "tadw-provider-test", ApplicationVersion: "tadw-provider-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(ctx, 5*time.Second)
	var expected []tadwTideExpectation
	workflow := func(ctx dbos.Context, request model.SessionEventRef) (bool, error) {
		history, err := checkpointTADWHistory(ctx, store, tadwToolingThinking(request, 5), nil, cfg)
		if err != nil {
			return false, err
		}
		history, limits := tadwTideBudget(history, budget)
		scope, err := tadwHistoryCacheScopeForAgent(request.Session, selected, false)
		if err != nil {
			return false, err
		}
		prepared := tadwPreparedHistory{Agent: selected.Ref, Scope: scope, History: history}
		from := ""
		for _, attempt := range []int{11, 40, 60} {
			window, err := checkpointTADWTooling(ctx, store, history, request, selected.Ref, tadwToolingThinking(request, attempt), limits, from)
			if err != nil {
				return false, err
			}
			err, wire := agentProviderRequestPrepareTADW(selected, false, prepared, window.Transcript, limits)
			if err != nil {
				return false, err
			}
			// Only a boolean is durable; the actual request/body remains a closure local.
			if _, err := dbos.RunAsStep(ctx, func(context.Context) (bool, error) {
				encoded, err := json.Marshal(wire.body())
				return len(encoded) > 0 && err == nil, err
			}, dbos.WithStepName("test.tide-thinking")); err != nil {
				return false, err
			}
			expected = append(expected, tadwTideExpectation{From: window.FromEventID, XML: window.Transcript.String()})
			from = window.FromEventID
		}
		return true, nil
	}
	dbos.RegisterWorkflow(ctx, workflow, dbos.WithWorkflowName("test.tadw-provider"))
	if err := testdbos.Launch(ctx); err != nil {
		t.Fatal(err)
	}
	handle, err := dbos.RunWorkflow(ctx, workflow, request, dbos.WithWorkflowID("tadw-tide-request"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	assertTADWTideStorage(t, store, initial.Window.EventIDs, expected)
	assertTADWDBOSPayloadAbsent(t, store)
	var tables int
	if err := store.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='gatehouse_context_cache_hints'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("assembly created shared cache state", tables, err)
	}
}
