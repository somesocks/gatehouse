package benchmark

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"sync"
	"time"

	"gatehouse/agent"
	"gatehouse/benchmarkmodel"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/storage"
	"gatehouse/task"
	"gatehouse/typed_id"
)

// Execute always returns the complete capture once it has been opened, including
// on execution failure or cancellation. The caller owns and closes that database.
// Runs share an execution-local native database and runtime, with fresh sessions.
func Execute(ctx context.Context, configuration Config, path, version, fingerprint string, progress io.Writer) (error, *sql.DB) {
	return ExecuteWithOptions(ctx, configuration, path, version, fingerprint, progress, ExecutionOptions{Parallel: DefaultParallel})
}

const DefaultParallel = 8

type ExecutionOptions struct {
	Parallel int
}

func ExecuteWithOptions(ctx context.Context, configuration Config, path, version, fingerprint string, progress io.Writer, options ExecutionOptions) (error, *sql.DB) {
	if options.Parallel < 1 {
		return fmt.Errorf("benchmark parallelism must be a positive integer"), nil
	}
	if progress == nil {
		progress = io.Discard
	}
	progress = &lockedWriter{writer: progress}
	err, prepared := Prepare(configuration, path, version, fingerprint)
	if err != nil {
		return err, nil
	}
	err, capture := OpenResults(ctx)
	if err != nil {
		return err, nil
	}
	if err := resultTransaction(ctx, capture, func(tx *sql.Tx) error { return prepared.Insert(ctx, tx) }); err != nil {
		return err, capture
	}
	finish := func(cause error) (error, *sql.DB) {
		prepared.Execution.Status = "completed"
		if cause != nil {
			prepared.Execution.Status = "failed"
			if ctx.Err() != nil {
				prepared.Execution.Status = "cancelled"
			}
			prepared.Execution.Error = stringPointer(cause.Error())
		}
		prepared.Execution.FinishedAt = stringPointer(timestamp())
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := resultTransaction(cleanup, capture, func(tx *sql.Tx) error { return UpdateExecution(cleanup, tx, prepared.Execution) })
		return errors.Join(cause, err), capture
	}
	providers := map[string]benchmarkmodel.Provider{}
	models := map[string]benchmarkmodel.Model{}
	prompts := map[string]benchmarkmodel.SystemPrompt{}
	questions := map[string]benchmarkmodel.Question{}
	environments := map[string]benchmarkmodel.LispEnvironment{}
	for _, provider := range prepared.Providers {
		providers[provider.Id] = provider
	}
	for _, selected := range prepared.Models {
		models[selected.Id] = selected
	}
	for _, prompt := range prepared.SystemPrompts {
		prompts[prompt.Id] = prompt
	}
	for _, question := range prepared.Questions {
		questions[question.Id] = question
	}
	for _, environment := range prepared.LispEnvironments {
		environments[environment.Id] = environment
	}

	// Resolve only selected providers, once. Credentials never enter capture data.
	selectedAliases := map[string]bool{}
	for _, test := range prepared.configuration.Document.Tests {
		for _, alias := range test.Matrix.Models {
			selectedAliases[alias] = true
		}
	}
	keys := map[string][]byte{}
	defer func() {
		for _, key := range keys {
			clear(key)
		}
	}()
	resolver := agent.NewAPIKeySourceResolver()
	for _, selected := range prepared.Models {
		if !selectedAliases[selected.Alias] {
			continue
		}
		if _, err := runtimeModel(selected); err != nil {
			return finish(err)
		}
		provider := providers[selected.ProviderId]
		if provider.Protocol == "builtin" || keys[provider.Id] != nil {
			continue
		}
		sources := []config.AgentProviderAPIKeySource{}
		if provider.ApiKeySources != nil {
			for _, source := range *provider.ApiKeySources {
				sources = append(sources, config.AgentProviderAPIKeySource(source))
			}
		}
		err, key := resolveAPIKey(ctx, resolver, provider.Alias, sources)
		if err != nil {
			return finish(err)
		}
		keys[provider.Id] = key
	}
	err, native := newNativeRuntime(ctx, prepared, providers, models, prompts, environments, keys, progress)
	if err != nil {
		return finish(err)
	}
	workContext, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	next, stopIterator := iter.Pull2(prepared.Runs())
	defer stopIterator()
	var scheduling sync.Mutex
	worker := func(ctx context.Context, _ int) (error, bool) {
		failed := false
		for {
			scheduling.Lock()
			if ctx.Err() != nil {
				scheduling.Unlock()
				return nil, failed
			}
			run, preparationErr, ok := next()
			scheduling.Unlock()
			if preparationErr != nil {
				stop(preparationErr)
				return preparationErr, failed
			}
			if !ok {
				return nil, failed
			}
			run.Status, run.StartedAt = "running", stringPointer(timestamp())
			if err := resultTransaction(ctx, capture, func(tx *sql.Tx) error { return InsertRun(ctx, tx, run) }); err != nil {
				stop(err)
				return err, failed
			}
			selected := models[run.ModelId]
			fmt.Fprintf(progress, "benchmark %s: %s / %s / %s / %s / repetition %d\n", run.Id, run.TestAlias, selected.Alias, questions[run.QuestionId].Alias, environments[run.LispEnvironmentId].Alias, run.Repetition)
			infrastructureErr := native.executeRun(ctx, capture, &run, questions[run.QuestionId], environments[run.LispEnvironmentId])
			if infrastructureErr != nil {
				stop(infrastructureErr)
			}
			if run.Status == "running" {
				run.Status = "failed"
				if ctx.Err() != nil {
					run.Status = "cancelled"
				}
				if infrastructureErr != nil {
					run.Error = stringPointer(infrastructureErr.Error())
				}
			}
			run.FinishedAt = stringPointer(timestamp())
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := resultTransaction(cleanup, capture, func(tx *sql.Tx) error { return UpdateRun(cleanup, tx, run) })
			cancel()
			if err := errors.Join(infrastructureErr, err); err != nil {
				stop(err)
				return err, failed
			}
			failed = failed || run.Status != "completed"
		}
	}
	// ParallelMap runs a fixed number of worker loops; the matrix stays lazy.
	workers := make([]int, int(min(uint64(options.Parallel), prepared.RunCount)))
	err, failed := task.ParallelMap(worker)(task.WithConcurrency(workContext, options.Parallel), workers)
	executionErr := errors.Join(err, context.Cause(workContext))
	for _, failure := range failed {
		if failure {
			executionErr = errors.Join(executionErr, fmt.Errorf("one or more benchmark runs did not complete"))
			break
		}
	}
	return finish(errors.Join(executionErr, native.close()))
}

type lockedWriter struct {
	mutex  sync.Mutex
	writer io.Writer
}

func (writer *lockedWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.writer.Write(data)
}

func runtimeModel(selected benchmarkmodel.Model) (database.SystemAgentModelInput, error) {
	parameters, err := json.Marshal(selected.Parameters)
	if err != nil {
		return database.SystemAgentModelInput{}, err
	}
	maxTurns, maxTokens := config.DefaultAgentModelMaxTurns, config.DefaultAgentModelMaxOutputTokens
	if selected.MaxTurns != nil {
		maxTurns = *selected.MaxTurns
	}
	if selected.MaxOutputTokens != nil {
		maxTokens = *selected.MaxOutputTokens
	}
	history, buffer := config.DefaultAgentModelHistoryBytes, config.DefaultAgentModelBufferBytes
	if selected.HistoryBytes != nil {
		history = *selected.HistoryBytes
	}
	if selected.BufferBytes != nil {
		buffer = *selected.BufferBytes
	}
	if selected.HistoryBytes != nil && selected.BufferBytes == nil {
		buffer = min(buffer, history/2)
	}
	if selected.BufferBytes != nil && selected.HistoryBytes == nil {
		if buffer > int(^uint(0)>>1)/2 {
			return database.SystemAgentModelInput{}, fmt.Errorf("model %q: compaction buffer is too large", selected.Alias)
		}
		history = max(history, 2*buffer)
	}
	if maxTurns <= 0 || maxTokens <= 0 || buffer <= 0 || history <= 0 || buffer > history/2 {
		return database.SystemAgentModelInput{}, fmt.Errorf("model %q: invalid effective execution limits", selected.Alias)
	}
	compaction, err := json.Marshal(config.AgentModelCompaction{Algorithm: "mcmtr", HistoryBytes: history, BufferBytes: buffer})
	if err != nil {
		return database.SystemAgentModelInput{}, err
	}
	return database.SystemAgentModelInput{Alias: "assistant", Model: selected.Model, Parameters: string(parameters), Compaction: string(compaction), MaxTurns: maxTurns, MaxOutputTokens: maxTokens, Enabled: true}, nil
}

func resolveAPIKey(ctx context.Context, resolver *agent.APIKeySourceResolver, alias string, sources []config.AgentProviderAPIKeySource) (error, []byte) {
	type resolution struct {
		err error
		key []byte
	}
	ready := make(chan resolution)
	go func() {
		err, key := resolver.Resolve(alias, sources)
		select {
		case ready <- resolution{err, key}:
		case <-ctx.Done():
			clear(key)
		}
	}()
	select {
	case result := <-ready:
		return result.err, result.key
	case <-ctx.Done():
		return ctx.Err(), nil
	}
}

func (native *nativeRuntime) executeRun(ctx context.Context, capture *sql.DB, run *benchmarkmodel.Run, question benchmarkmodel.Question, environment benchmarkmodel.LispEnvironment) (resultErr error) {
	store, keyring, runtime := native.store, native.keyring, native.runtime
	binding := native.bindings[nativeSelection{run.ModelId, run.SystemPromptId, run.LispEnvironmentId}]
	workspace, principal := binding.workspace, native.principal
	var capturedSession model.SessionRef
	seen := map[string]bool{}
	defer func() {
		// All per-session capture finishes before the shared runtime is closed.
		if capturedSession.Id != "" {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err, events := store.SessionEventsGet(cleanup, capturedSession)
			if err == nil {
				err = captureEvents(cleanup, capture, run, events, seen)
			}
			if err == nil {
				err = captureArtifacts(capture, store, keyring, run, capturedSession)
			}
			cancel()
			resultErr = errors.Join(resultErr, err)
		}
	}()
	sessionID, err := typed_id.New(typed_id.Session)
	if err != nil {
		return err
	}
	session := model.Session{Ref: model.SessionRef{Workspace: workspace, Id: sessionID}, Name: stringPointer("Benchmark"), AuthorPrincipal: &principal, Enabled: true}
	if environment.ProjectLinked != nil && *environment.ProjectLinked {
		id, err := typed_id.New(typed_id.Project)
		if err != nil {
			return err
		}
		project := model.Project{Ref: model.ProjectRef{Workspace: workspace, Id: id}, Enabled: true}
		if err, _ := store.ProjectsCreate(ctx, project, principal, nil); err != nil {
			return err
		}
		session.Project = &project.Ref
	}
	if err, _ := store.SessionsCreate(ctx, session, principal); err != nil {
		return err
	}
	run.WorkspaceId, run.WorkspaceAgentId, run.SessionId = &workspace.Id, &binding.agentID, &sessionID
	capturedSession = session.Ref
	messageID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		return err
	}
	author := model.Principal{Ref: principal, Enabled: true}
	if err, _ := runtime.CreateMessage(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session.Ref, Id: messageID}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &author,
		Payload: map[string]interface{}{"text": question.Text, "agents": []string{binding.agentID}}}); err != nil {
		return err
	}
	var stopping error
	var cancellation *model.SessionEvent
	var stopAt time.Time
	for {
		poll, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err, events := store.SessionEventsGet(poll, session.Ref)
		if err == nil {
			err = captureEvents(poll, capture, run, events, seen)
		}
		cancel()
		if err != nil {
			return err
		}
		var request, terminal *model.SessionEvent
		cancellationFinished := nativeCancellationComplete(events, cancellation)
		for index := range events {
			event := &events[index]
			if event.Kind == model.SessionEventKindAgentRequest {
				request = event
			}
			if event.Kind == model.SessionEventKindAgentSuccess || event.Kind == model.SessionEventKindAgentFailure {
				terminal = event
			}
			if stopping == nil && (event.Kind == model.SessionEventKindInputRequest || event.Kind == model.SessionEventKindApprovalRequest) {
				stopping = fmt.Errorf("benchmark requires unattended execution; agent requested input or approval")
			}
		}
		if terminal != nil && cancellationFinished {
			run.TerminalEventId = &terminal.Ref.Id
			run.Status = "completed"
			if terminal.Kind == model.SessionEventKindAgentFailure {
				run.Status, run.Error = "failed", stringPointer(fmt.Sprintf("agent failed: %v", terminal.Payload["code"]))
			}
			if stopping != nil {
				run.Status, run.Error = "failed", stringPointer(stopping.Error())
				if ctx.Err() != nil {
					run.Status = "cancelled"
				}
			}
			return nil
		}
		if ctx.Err() != nil && stopping == nil {
			stopping = ctx.Err()
		}
		if stopping != nil && stopAt.IsZero() {
			stopAt = time.Now().Add(10 * time.Second)
		}
		if stopping != nil && cancellation == nil && request != nil {
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				return err
			}
			event := model.SessionEvent{Ref: model.SessionEventRef{Session: session.Ref, Id: id}, Parent: &request.Ref,
				Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: &author, Payload: map[string]interface{}{}}
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err, _ = runtime.CancelReply(cleanup, event)
			cancel()
			if err != nil && !errors.Is(err, database.ErrSessionReplyAlreadyCompleted) {
				return err
			}
			if err == nil {
				cancellation = &event
			}
		}
		if !stopAt.IsZero() && time.Now().After(stopAt) {
			return fmt.Errorf("waiting for native benchmark cancellation: %w", stopping)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func nativeCancellationComplete(events []model.SessionEvent, cancellation *model.SessionEvent) bool {
	if cancellation == nil {
		return true
	}
	for _, event := range events {
		if (event.Kind == model.SessionEventKindCancelSuccess || event.Kind == model.SessionEventKindCancelFailure) && event.Parent != nil && *event.Parent == cancellation.Ref {
			return true
		}
	}
	return false
}

func captureEvents(ctx context.Context, capture *sql.DB, run *benchmarkmodel.Run, events []model.SessionEvent, seen map[string]bool) error {
	err := resultTransaction(ctx, capture, func(tx *sql.Tx) error {
		for _, event := range events {
			if event.Kind == model.SessionEventKindAgentRequest {
				run.AgentRequestId = stringPointer(event.Ref.Id)
			}
			if seen[event.Ref.Id] {
				continue
			}
			record := benchmarkmodel.SessionEvent{Id: event.Ref.Id, RunId: run.Id, Workspace: event.Ref.Session.Workspace.Id, Session: event.Ref.Session.Id,
				Kind: event.Kind, Payload: event.Payload, CreatedAt: event.CreatedAt}
			if event.Parent != nil {
				record.Parent = stringPointer(event.Parent.Id)
			}
			if event.AuthorPrincipal != nil {
				record.AuthorPrincipal = stringPointer(event.AuthorPrincipal.Ref.Id)
			}
			if event.AuthorAgent != nil {
				record.AuthorAgent = stringPointer(event.AuthorAgent.Id)
			}
			if event.AuthorGateway != nil {
				record.AuthorGateway = stringPointer(event.AuthorGateway.Id)
			}
			if event.Metrics != nil {
				encoded, err := json.Marshal(event.Metrics)
				if err != nil {
					return err
				}
				var metrics map[string]interface{}
				decoder := json.NewDecoder(bytes.NewReader(encoded))
				decoder.UseNumber()
				if err := decoder.Decode(&metrics); err != nil {
					return err
				}
				record.Metrics = &metrics
			}
			if err := InsertSessionEvent(ctx, tx, record); err != nil {
				return err
			}
		}
		return UpdateRun(ctx, tx, *run)
	})
	if err == nil {
		for _, event := range events {
			seen[event.Ref.Id] = true
		}
	}
	return err
}

func captureArtifacts(capture *sql.DB, store *database.Store, keyring *keychain.Keyring, run *benchmarkmodel.Run, session model.SessionRef) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files, err := resultRows(ctx, store.DB, `SELECT files.*, objects.state AS object_state
        FROM gatehouse_session_files files JOIN gatehouse_storage_objects objects ON objects.id = files.storage_object
        WHERE files.workspace = ? AND files.session = ? ORDER BY files.id`, session.Workspace.Id, session.Id)
	if err != nil {
		return err
	}
	client := storage.NewClient(store, keyring)
	for _, file := range files {
		object, ok := file["storage_object"].(string)
		if !ok {
			return fmt.Errorf("capture artifact: missing storage object")
		}
		var content []byte
		if file["object_state"] == "success" {
			err, reader := client.Get(ctx, object, 0)
			if err != nil {
				return err
			}
			if reader == nil {
				return fmt.Errorf("capture artifact: storage object %q is unavailable", object)
			}
			content, err = io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		metadata, err := json.Marshal(file)
		if err != nil {
			return err
		}
		if _, err := capture.ExecContext(ctx, `INSERT INTO artifacts (run_id, id, metadata, content) VALUES (?, ?, ?, ?)`, run.Id, file["id"], string(metadata), content); err != nil {
			return err
		}
	}
	return nil
}

func resultTransaction(ctx context.Context, db *sql.DB, action func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := action(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func timestamp() string                  { return time.Now().UTC().Format(time.RFC3339Nano) }
func stringPointer(value string) *string { return &value }
