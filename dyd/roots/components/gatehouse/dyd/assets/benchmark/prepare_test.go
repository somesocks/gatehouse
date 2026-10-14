package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gatehouse/benchmarkmodel"
	"gatehouse/typed_id"
)

func preparationConfig(t *testing.T) (Config, string) {
	t.Helper()
	directory := t.TempDir()
	files := map[string]string{
		"prompt.txt":   "Inspect tooling.\n",
		"question.txt": "What tools do you have?\n",
		"prelude.lisp": "; preserve comments\n(eval program)\n",
		"tests.yaml": `api_version: v1
agent_providers:
  - alias: openai
    protocol: openai-responses
    endpoint: https://provider.invalid/v1/responses
    api_key:
      sources: [env:BENCH_PREPARE_KEY]
  - alias: builtin
    protocol: builtin
agent_models:
  - alias: model-a
    provider: openai
    model: remote-model
    parameters: {reasoning_effort: low}
    compaction: {algorithm: mcmtr, history_bytes: 64}
  - alias: model-b
    provider: builtin
    model: dummy.fixed-reply
    parameters: {text: reply}
    max_turns: 8
    max_output_tokens: 1024
    enabled: true
system_prompts:
  - alias: baseline
    mode: builtin
  - alias: discovery
    mode: replace
    text: file:prompt.txt
  - alias: empty
    mode: append
    text: ""
questions:
  - alias: capabilities
    text: "hey, what can you do?"
  - alias: tools
    text: file:question.txt
lisp_environments:
  - alias: omitted
  - alias: empty
    project_linked: false
    prelude: ""
  - alias: project
    project_linked: true
    prelude: file:prelude.lisp
tests:
  - alias: matrix
    matrix:
      models: [model-b, model-a]
      system_prompts: [discovery, baseline]
      questions: [tools, capabilities]
      lisp_environments: [empty, omitted]
    repetitions: 2
  - alias: default-repetition
    matrix:
      models: [model-a]
      system_prompts: [empty]
      questions: [capabilities]
      lisp_environments: [project]
`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, "tests.yaml")
	err, configuration := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return configuration, path
}

func TestPreparePreservesSnapshotsAndPersistsIDs(t *testing.T) {
	configuration, path := preparationConfig(t)
	t.Setenv("BENCH_PREPARE_KEY", "must-not-be-resolved")
	err, prepared := Prepare(configuration, path, "test-version", "test-build")
	if err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid("bex", prepared.Execution.Id) || prepared.Execution.ConfigPath != path || prepared.Execution.ConfigApiVersion != "v1" || prepared.Execution.GatehouseVersion != "test-version" || prepared.Execution.GatehouseFingerprint != "test-build" || prepared.Execution.Status != "running" {
		t.Fatalf("execution metadata = %#v", prepared.Execution)
	}
	started, parseErr := time.Parse(time.RFC3339Nano, prepared.Execution.StartedAt)
	if parseErr != nil || started.Location() != time.UTC {
		t.Fatalf("execution timestamp = %q (%v)", prepared.Execution.StartedAt, parseErr)
	}
	if len(prepared.Providers) != 2 || len(prepared.Models) != 2 || len(prepared.SystemPrompts) != 3 || len(prepared.Questions) != 2 || len(prepared.LispEnvironments) != 3 || prepared.RunCount != 33 {
		t.Fatalf("unexpected prepared collection sizes or run count: %#v", prepared)
	}
	provider := prepared.Providers[0]
	if provider.ApiKeySources == nil || !reflect.DeepEqual(*provider.ApiKeySources, []string{"env:BENCH_PREPARE_KEY"}) || provider.Keychain != nil || provider.Enabled != nil || *provider.Endpoint != "https://provider.invalid/v1/responses" {
		t.Fatalf("provider settings changed: %#v", provider)
	}
	if prepared.Providers[1].ApiKeySources != nil || prepared.Providers[1].Endpoint != nil {
		t.Fatal("omitted builtin credentials or endpoint were filled")
	}
	model := prepared.Models[0]
	if model.ProviderId != provider.Id || model.MaxTurns != nil || model.MaxOutputTokens != nil || model.Enabled != nil || model.CompactionAlgorithm == nil || *model.CompactionAlgorithm != "mcmtr" || model.HistoryBytes == nil || *model.HistoryBytes != 64 || model.BufferBytes != nil || model.Parameters["reasoning_effort"] != "low" {
		t.Fatalf("model settings or provider reference changed: %#v", model)
	}
	if prepared.Models[1].ProviderId != prepared.Providers[1].Id || *prepared.Models[1].MaxTurns != 8 || *prepared.Models[1].MaxOutputTokens != 1024 || prepared.Models[1].CompactionAlgorithm != nil || prepared.Models[1].Enabled == nil || !*prepared.Models[1].Enabled {
		t.Fatal("explicit model settings were not preserved")
	}
	if prepared.SystemPrompts[0].Text != nil || *prepared.SystemPrompts[1].Text != "Inspect tooling.\n" || *prepared.SystemPrompts[2].Text != "" || prepared.Questions[1].Text != "What tools do you have?\n" {
		t.Fatal("prompt or question text changed")
	}
	if prepared.LispEnvironments[0].Prelude != nil || prepared.LispEnvironments[0].ProjectLinked != nil || prepared.LispEnvironments[1].Prelude == nil || *prepared.LispEnvironments[1].Prelude != "" || prepared.LispEnvironments[1].ProjectLinked == nil || *prepared.LispEnvironments[1].ProjectLinked || *prepared.LispEnvironments[2].Prelude != "; preserve comments\n(eval program)\n" || !*prepared.LispEnvironments[2].ProjectLinked {
		t.Fatal("omitted, empty, false, or loaded environment settings changed")
	}
	db := openTestResults(t)
	tx, txErr := db.BeginTx(context.Background(), nil)
	if txErr != nil {
		t.Fatal(txErr)
	}
	defer tx.Rollback()
	if err := prepared.Insert(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var providerID, executionID, credentialSources string
	if err := db.QueryRow(`SELECT m.provider_id, m.execution_id, p.api_key_sources FROM models m
		JOIN providers p ON p.execution_id = m.execution_id AND p.id = m.provider_id WHERE m.id = ?`, model.Id).
		Scan(&providerID, &executionID, &credentialSources); err != nil {
		t.Fatal(err)
	}
	if providerID != provider.Id || executionID != prepared.Execution.Id || credentialSources != `["env:BENCH_PREPARE_KEY"]` {
		t.Fatal("prepared snapshot IDs or credential descriptors changed on insertion")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("input insertion materialized run rows")
	}
}

func TestPrepareRunSelectionsAndIndependentExecutions(t *testing.T) {
	configuration, path := preparationConfig(t)
	err, first := Prepare(configuration, path, "version", "build")
	if err != nil {
		t.Fatal(err)
	}
	err, second := Prepare(configuration, path, "version", "build")
	if err != nil {
		t.Fatal(err)
	}
	firstRuns := make([]benchmarkmodel.Run, 0, 33)
	seen := map[string]bool{}
	for run, err := range first.Runs() {
		if err != nil {
			t.Fatal(err)
		}
		if !typed_id.Valid("brn", run.Id) || seen[run.Id] || run.ExecutionId != first.Execution.Id || run.Status != "planned" || run.StartedAt != nil || run.TerminalEventId != nil || run.SessionId != nil {
			t.Fatalf("invalid planned run: %#v", run)
		}
		seen[run.Id] = true
		firstRuns = append(firstRuns, run)
	}
	// Explicit bit positions pin nesting order independently of the iterator.
	if len(firstRuns) != 33 {
		t.Fatalf("generated %d runs, want 33", len(firstRuns))
	}
	for index, run := range firstRuns[:32] {
		model := []string{first.Models[1].Id, first.Models[0].Id}[(index/16)%2]
		prompt := []string{first.SystemPrompts[1].Id, first.SystemPrompts[0].Id}[(index/8)%2]
		question := []string{first.Questions[1].Id, first.Questions[0].Id}[(index/4)%2]
		environment := []string{first.LispEnvironments[1].Id, first.LispEnvironments[0].Id}[(index/2)%2]
		if run.TestAlias != "matrix" || run.ModelId != model || run.SystemPromptId != prompt || run.QuestionId != question || run.LispEnvironmentId != environment || run.Repetition != index%2+1 {
			t.Fatalf("run %d violates selection order/product: %#v", index, run)
		}
	}
	last := firstRuns[32]
	if last.TestAlias != "default-repetition" || last.Repetition != 1 || last.ModelId != first.Models[0].Id || last.SystemPromptId != first.SystemPrompts[2].Id || last.QuestionId != first.Questions[0].Id || last.LispEnvironmentId != first.LispEnvironments[2].Id {
		t.Fatalf("default-repetition run = %#v", last)
	}
	if first.Execution.Id == second.Execution.Id || first.Models[0].Id == second.Models[0].Id || first.Providers[0].Id == second.Providers[0].Id || second.Models[0].ProviderId != second.Providers[0].Id {
		t.Fatal("successive preparations reused identities or crossed execution references")
	}
	db := openTestResults(t)
	for _, prepared := range []*Prepared{first, second} {
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := prepared.Insert(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	tx, txErr := db.BeginTx(context.Background(), nil)
	if txErr != nil {
		t.Fatal(txErr)
	}
	defer tx.Rollback()
	for _, run := range firstRuns {
		if err := InsertRun(context.Background(), tx, run); err != nil {
			t.Fatal(err)
		}
	}
	for run, err := range second.Runs() {
		if err != nil {
			t.Fatal(err)
		}
		if err := InsertRun(context.Background(), tx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 66 {
		t.Fatalf("stored %d planned runs, want 66", count)
	}
}

func TestPrepareFreezesCallerConfiguration(t *testing.T) {
	configuration, path := preparationConfig(t)
	err, prepared := Prepare(configuration, path, "version", "build")
	if err != nil {
		t.Fatal(err)
	}
	configuration.Document.AgentModels[0].Parameters.(map[string]any)["reasoning_effort"] = "changed"
	*configuration.Document.AgentModels[0].Compaction.HistoryBytes = 999
	configuration.Document.AgentProviders[0].ApiKey.Sources[0] = "env:CHANGED"
	*configuration.Document.SystemPrompts[1].Text = "changed"
	*configuration.Document.LispEnvironments[2].Prelude = "changed"
	configuration.Document.Tests[0].Matrix.Models[0] = "unknown"
	*configuration.Document.Tests[0].Repetitions = 99
	if prepared.Models[0].Parameters["reasoning_effort"] != "low" || *prepared.Models[0].HistoryBytes != 64 || (*prepared.Providers[0].ApiKeySources)[0] != "env:BENCH_PREPARE_KEY" || *prepared.SystemPrompts[1].Text != "Inspect tooling.\n" || *prepared.LispEnvironments[2].Prelude != "; preserve comments\n(eval program)\n" {
		t.Fatal("caller edits changed prepared input snapshots")
	}
	count := 0
	for run, err := range prepared.Runs() {
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 && (run.ModelId != prepared.Models[1].Id || run.Repetition != 1) {
			t.Fatal("caller edits changed prepared selections")
		}
		count++
	}
	if count != 33 {
		t.Fatalf("caller edits changed run count to %d", count)
	}
}

func TestPrepareLargeMatrixIsLazy(t *testing.T) {
	configuration, path := preparationConfig(t)
	configuration.Document.Tests = configuration.Document.Tests[:1]
	repetitions := 1_000_000_000
	configuration.Document.Tests[0].Repetitions = &repetitions
	configuration.RunCount = 16_000_000_000
	err, prepared := Prepare(configuration, path, "version", "build")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for run, err := range prepared.Runs() {
		if err != nil {
			t.Fatal(err)
		}
		if run.Repetition != 1 || run.ModelId != prepared.Models[1].Id {
			t.Fatal("large lazy matrix yielded the wrong first record")
		}
		count++
		break
	}
	if count != 1 || prepared.RunCount != 16_000_000_000 {
		t.Fatal("large matrix did not stop after its first requested record")
	}
}

func TestPreparePreservesJSONNumbers(t *testing.T) {
	configuration, path := preparationConfig(t)
	// Preparation is a representation boundary, independent of parameter-policy
	// validation; arbitrary JSON numbers must survive its deep copy exactly.
	configuration.Document.AgentModels[0].Parameters.(map[string]any)["number"] = json.Number("9007199254740993")
	err, prepared := Prepare(configuration, path, "version", "build")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Models[0].Parameters["number"] != json.Number("9007199254740993") {
		t.Fatal("preparation rounded a JSON number")
	}
}
