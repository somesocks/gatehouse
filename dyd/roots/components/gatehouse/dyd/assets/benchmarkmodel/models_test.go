package benchmarkmodel_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"gatehouse/benchmarkmodel"
	"gatehouse/typed_id"
)

func TestCapturedRecordGraphRoundTrip(t *testing.T) {
	newID := func(kind string) string {
		t.Helper()
		id, err := typed_id.New(kind)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// All export identities and links are assembled without database insertion.
	execution := benchmarkmodel.Execution{
		Id: newID("bex"), ConfigPath: "tests.yaml",
		ConfigApiVersion: "v1", GatehouseVersion: "test", GatehouseFingerprint: "build-hash",
		StartedAt: "2026-10-13T12:00:00Z", Status: "running",
	}
	credentialSources := []string{"env:BENCH_API_KEY"}
	provider := benchmarkmodel.Provider{
		Id: newID("bpr"), ExecutionId: execution.Id, Alias: "provider",
		Protocol: "openai-responses", ApiKeySources: &credentialSources,
	}
	model := benchmarkmodel.Model{
		Id: newID("bmd"), ExecutionId: execution.Id, Alias: "assistant", ProviderId: provider.Id,
		Model: "test-model", Parameters: map[string]interface{}{"reasoning_effort": "low"},
	}
	prompt := benchmarkmodel.SystemPrompt{
		Id: newID("bsp"), ExecutionId: execution.Id, Alias: "baseline", Mode: "builtin",
	}
	question := benchmarkmodel.Question{
		Id: newID("bqu"), ExecutionId: execution.Id, Alias: "capabilities", Text: "What can you do?",
	}
	environment := benchmarkmodel.LispEnvironment{
		Id: newID("ben"), ExecutionId: execution.Id, Alias: "session",
	}
	workspaceID, sessionID, agentID := newID(typed_id.Workspace), newID(typed_id.Session), newID(typed_id.WorkspaceAgent)
	requestID, terminalID := newID(typed_id.SessionEvent), newID(typed_id.SessionEvent)
	run := benchmarkmodel.Run{
		Id: newID("brn"), ExecutionId: execution.Id, TestAlias: "discovery", Repetition: 1,
		ModelId: model.Id, SystemPromptId: prompt.Id, QuestionId: question.Id, LispEnvironmentId: environment.Id,
		WorkspaceId: &workspaceID, WorkspaceAgentId: &agentID, SessionId: &sessionID,
		AgentRequestId: &requestID, TerminalEventId: &terminalID, Status: "completed",
	}
	metrics := map[string]interface{}{
		"input_tokens": json.Number("9007199254740993"), "output_tokens": json.Number("0"),
		"future_metric": map[string]interface{}{"value": json.Number("17")},
	}
	events := []benchmarkmodel.SessionEvent{
		{
			Id: requestID, RunId: run.Id, Workspace: workspaceID, Session: sessionID,
			Kind: "agent.request", CreatedAt: "2026-10-13T12:00:01Z",
			Payload: map[string]interface{}{"agent": agentID},
		},
		{
			Id: terminalID, RunId: run.Id, Workspace: workspaceID, Session: sessionID,
			Parent: &requestID, AuthorAgent: &agentID, Kind: "agent.success", CreatedAt: "2026-10-13T12:00:02Z",
			Payload: map[string]interface{}{"text": "Available capabilities", "future_payload": map[string]interface{}{"items": []interface{}{"one", "two"}}},
			Metrics: &metrics,
		},
	}
	type capture struct {
		Execution   benchmarkmodel.Execution
		Provider    benchmarkmodel.Provider
		Model       benchmarkmodel.Model
		Prompt      benchmarkmodel.SystemPrompt
		Question    benchmarkmodel.Question
		Environment benchmarkmodel.LispEnvironment
		Run         benchmarkmodel.Run
		Events      []benchmarkmodel.SessionEvent
	}
	want := capture{execution, provider, model, prompt, question, environment, run, events}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got capture
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("captured identities, references, payloads, or metrics changed on round trip\ngot: %#v\nwant: %#v", got, want)
	}
}

func TestCapturedOptionalValuesRemainDistinct(t *testing.T) {
	empty, no := "", false
	history, algorithm := 64, "mcmtr"
	metrics := map[string]interface{}{}
	type capture struct {
		OmittedEnvironment benchmarkmodel.LispEnvironment
		EmptyEnvironment   benchmarkmodel.LispEnvironment
		BuiltinPrompt      benchmarkmodel.SystemPrompt
		EmptyPrompt        benchmarkmodel.SystemPrompt
		PartialModel       benchmarkmodel.Model
		UnknownMetrics     benchmarkmodel.SessionEvent
		EmptyMetrics       benchmarkmodel.SessionEvent
	}
	want := capture{
		OmittedEnvironment: benchmarkmodel.LispEnvironment{Id: "environment-omitted", ExecutionId: "execution"},
		EmptyEnvironment:   benchmarkmodel.LispEnvironment{Id: "environment-empty", ExecutionId: "execution", Prelude: &empty, ProjectLinked: &no},
		BuiltinPrompt:      benchmarkmodel.SystemPrompt{Id: "builtin", Mode: "builtin"},
		EmptyPrompt:        benchmarkmodel.SystemPrompt{Id: "empty", Mode: "replace", Text: &empty},
		PartialModel:       benchmarkmodel.Model{Id: "partial", CompactionAlgorithm: &algorithm, HistoryBytes: &history},
		UnknownMetrics:     benchmarkmodel.SessionEvent{Id: "unknown"},
		EmptyMetrics:       benchmarkmodel.SessionEvent{Id: "empty", Metrics: &metrics},
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got capture
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("omitted, empty, false, partial, or unknown values changed on round trip\ngot: %#v\nwant: %#v", got, want)
	}
}
