package benchmark

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"gatehouse/benchmarkmodel"
	"gatehouse/benchmarkschema"
	"gatehouse/typed_id"
)

// Prepared owns the captured input records for one execution. IDs exist before
// persistence, and run records are generated only as the iterator is consumed.
type Prepared struct {
	Execution        benchmarkmodel.Execution
	Providers        []benchmarkmodel.Provider
	Models           []benchmarkmodel.Model
	SystemPrompts    []benchmarkmodel.SystemPrompt
	Questions        []benchmarkmodel.Question
	LispEnvironments []benchmarkmodel.LispEnvironment
	RunCount         uint64

	configuration  Config
	modelIDs       map[string]string
	promptIDs      map[string]string
	questionIDs    map[string]string
	environmentIDs map[string]string
}

// Prepare consumes a validated, file-resolved configuration. It assigns capture
// identities and copies declared values without reading credentials, applying
// operational defaults, opening a database, or starting an agent.
func Prepare(configuration Config, path, version, fingerprint string) (error, *Prepared) {
	// Freeze the document, including nested parameter values and matrix slices,
	// so later edits to the caller's configuration do not change this execution.
	encoded, err := json.Marshal(configuration.Document)
	if err != nil {
		return fmt.Errorf("copy benchmark configuration: %w", err), nil
	}
	var document benchmarkschema.GatehouseBenchmarkConfig
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("copy benchmark configuration: %w", err), nil
	}
	executionID, err := typed_id.New("bex")
	if err != nil {
		return err, nil
	}
	prepared := &Prepared{
		Execution: benchmarkmodel.Execution{
			Id: executionID, ConfigPath: path, ConfigApiVersion: document.ApiVersion,
			GatehouseVersion: version, GatehouseFingerprint: fingerprint,
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "running",
		},
		RunCount:      configuration.RunCount,
		configuration: Config{Document: document, RunCount: configuration.RunCount, Sources: map[string]Source{}},
		modelIDs:      map[string]string{}, promptIDs: map[string]string{},
		questionIDs: map[string]string{}, environmentIDs: map[string]string{},
	}
	for key, source := range configuration.Sources {
		prepared.configuration.Sources[key] = source
	}
	providerIDs := map[string]string{}
	for _, definition := range document.AgentProviders {
		id, err := typed_id.New("bpr")
		if err != nil {
			return err, nil
		}
		record := benchmarkmodel.Provider{
			Id: id, ExecutionId: executionID, Alias: definition.Alias, Protocol: definition.Protocol,
			Endpoint: definition.Endpoint, Enabled: definition.Enabled,
		}
		if definition.ApiKey != nil {
			record.ApiKeySources, record.Keychain = &definition.ApiKey.Sources, definition.ApiKey.Keychain
		}
		prepared.Providers = append(prepared.Providers, record)
		providerIDs[record.Alias] = record.Id
	}
	for _, definition := range document.AgentModels {
		id, err := typed_id.New("bmd")
		if err != nil {
			return err, nil
		}
		providerID, exists := providerIDs[definition.Provider]
		if !exists {
			return fmt.Errorf("prepare model %q: unknown provider %q", definition.Alias, definition.Provider), nil
		}
		parameters, ok := definition.Parameters.(map[string]any)
		if !ok {
			return fmt.Errorf("prepare model %q: parameters must be an object", definition.Alias), nil
		}
		record := benchmarkmodel.Model{
			Id: id, ExecutionId: executionID, Alias: definition.Alias, ProviderId: providerID,
			Model: definition.Model, Parameters: parameters, MaxTurns: definition.MaxTurns,
			MaxOutputTokens: definition.MaxOutputTokens, Enabled: definition.Enabled,
		}
		if definition.Compaction != nil {
			record.CompactionAlgorithm = &definition.Compaction.Algorithm
			record.HistoryBytes, record.BufferBytes = definition.Compaction.HistoryBytes, definition.Compaction.BufferBytes
		}
		prepared.Models = append(prepared.Models, record)
		prepared.modelIDs[record.Alias] = record.Id
	}
	for _, definition := range document.SystemPrompts {
		id, err := typed_id.New("bsp")
		if err != nil {
			return err, nil
		}
		record := benchmarkmodel.SystemPrompt{
			Id: id, ExecutionId: executionID, Alias: definition.Alias, Mode: definition.Mode, Text: definition.Text,
		}
		prepared.SystemPrompts = append(prepared.SystemPrompts, record)
		prepared.promptIDs[record.Alias] = record.Id
	}
	for _, definition := range document.Questions {
		id, err := typed_id.New("bqu")
		if err != nil {
			return err, nil
		}
		record := benchmarkmodel.Question{Id: id, ExecutionId: executionID, Alias: definition.Alias, Text: definition.Text}
		if definition.Expected != nil {
			record.ExpectedOutput = definition.Expected.Output
			record.ExpectedMaxTurns = definition.Expected.MaxTurns
			record.ExpectedMaxToolFailures = definition.Expected.MaxToolFailures
			record.ExpectedMaxToolCalls = definition.Expected.MaxToolCalls
		}
		prepared.Questions = append(prepared.Questions, record)
		prepared.questionIDs[record.Alias] = record.Id
	}
	for _, definition := range document.LispEnvironments {
		id, err := typed_id.New("ben")
		if err != nil {
			return err, nil
		}
		record := benchmarkmodel.LispEnvironment{
			Id: id, ExecutionId: executionID, Alias: definition.Alias, Prelude: definition.Prelude, ProjectLinked: definition.ProjectLinked,
		}
		prepared.LispEnvironments = append(prepared.LispEnvironments, record)
		prepared.environmentIDs[record.Alias] = record.Id
	}
	return nil, prepared
}

// Runs yields new planned records in the validated selection/repetition order.
// Each yielded record receives a fresh ID; callers retain that record when
// persisting it and updating its lifecycle. ID-generation errors end iteration.
func (prepared *Prepared) Runs() iter.Seq2[benchmarkmodel.Run, error] {
	return func(yield func(benchmarkmodel.Run, error) bool) {
		for selection := range prepared.configuration.Runs() {
			id, err := typed_id.New("brn")
			if err != nil {
				yield(benchmarkmodel.Run{}, fmt.Errorf("prepare benchmark run: %w", err))
				return
			}
			record := benchmarkmodel.Run{
				Id: id, ExecutionId: prepared.Execution.Id, TestAlias: selection.TestAlias,
				ModelId: prepared.modelIDs[selection.ModelAlias], SystemPromptId: prepared.promptIDs[selection.SystemPromptAlias],
				QuestionId: prepared.questionIDs[selection.QuestionAlias], LispEnvironmentId: prepared.environmentIDs[selection.EnvironmentAlias],
				Repetition: selection.Repetition, Status: "planned",
			}
			if !yield(record, nil) {
				return
			}
		}
	}
}

// Insert stores the execution and input snapshots in the caller's transaction.
// Run rows are inserted separately as they are generated and scheduled.
func (prepared *Prepared) Insert(ctx context.Context, tx *sql.Tx) error {
	if err := InsertExecution(ctx, tx, prepared.Execution); err != nil {
		return err
	}
	document, err := json.Marshal(prepared.configuration.Document)
	if err != nil {
		return err
	}
	sources, err := json.Marshal(prepared.configuration.Sources)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_inputs (execution_id, run_count, configuration, sources) VALUES (?, ?, ?, ?)`,
		prepared.Execution.Id, fmt.Sprint(prepared.RunCount), string(document), string(sources)); err != nil {
		return fmt.Errorf("insert execution inputs: %w", err)
	}
	for _, record := range prepared.Providers {
		if err := InsertProvider(ctx, tx, record); err != nil {
			return err
		}
	}
	for _, record := range prepared.Models {
		if err := InsertModel(ctx, tx, record); err != nil {
			return err
		}
	}
	for _, record := range prepared.SystemPrompts {
		if err := InsertSystemPrompt(ctx, tx, record); err != nil {
			return err
		}
	}
	for _, record := range prepared.Questions {
		if err := InsertQuestion(ctx, tx, record); err != nil {
			return err
		}
	}
	for _, record := range prepared.LispEnvironments {
		if err := InsertLispEnvironment(ctx, tx, record); err != nil {
			return err
		}
	}
	return nil
}
