package benchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	"gatehouse/benchmarkmodel"
	"gatehouse/typed_id"
)

func openTestResults(t *testing.T) *sql.DB {
	t.Helper()
	err, db := OpenResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type resultsFixture struct {
	execution benchmarkmodel.Execution
	provider  benchmarkmodel.Provider
	model     benchmarkmodel.Model
	prompt    benchmarkmodel.SystemPrompt
	question  benchmarkmodel.Question
	env       benchmarkmodel.LispEnvironment
	run       benchmarkmodel.Run
	events    []benchmarkmodel.SessionEvent
}

func newResultsFixture(t *testing.T, question string) resultsFixture {
	t.Helper()
	id := func(kind string) string {
		t.Helper()
		value, err := typed_id.New(kind)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	empty, no, algorithm, history := "", false, "mcmtr", 64
	endpoint := "https://provider.invalid/v1"
	credentials := []string{"env:BENCH_API_KEY"}
	f := resultsFixture{
		execution: benchmarkmodel.Execution{
			Id: id("bex"), ConfigPath: "tests.yaml", ConfigApiVersion: "v1",
			GatehouseVersion: "test", GatehouseFingerprint: "build", StartedAt: "2026-10-13T12:00:00Z", Status: "running",
		},
	}
	f.provider = benchmarkmodel.Provider{
		Id: id("bpr"), ExecutionId: f.execution.Id, Alias: "provider", Protocol: "openai-responses",
		BaseUrl: &endpoint, ApiKeySources: &credentials,
	}
	f.model = benchmarkmodel.Model{
		Id: id("bmd"), ExecutionId: f.execution.Id, ProviderId: f.provider.Id, Alias: "assistant", Model: "test-model",
		Parameters: map[string]interface{}{"reasoning_effort": "low"}, CompactionAlgorithm: &algorithm, HistoryBytes: &history,
	}
	f.prompt = benchmarkmodel.SystemPrompt{
		Id: id("bsp"), ExecutionId: f.execution.Id, Alias: "baseline", Mode: "append", Text: &empty,
	}
	f.question = benchmarkmodel.Question{
		Id: id("bqu"), ExecutionId: f.execution.Id, Alias: "capabilities", Text: question,
	}
	f.env = benchmarkmodel.LispEnvironment{
		Id: id("ben"), ExecutionId: f.execution.Id, Alias: "session", Prelude: &empty, ProjectLinked: &no,
	}
	workspace, session, principal, agent := id(typed_id.Workspace), id(typed_id.Session), id(typed_id.Principal), id(typed_id.WorkspaceAgent)
	message, request, tool, toolResult, terminal := id(typed_id.SessionEvent), id(typed_id.SessionEvent), id(typed_id.SessionEvent), id(typed_id.SessionEvent), id(typed_id.SessionEvent)
	started, finished := "2026-10-13T12:00:00Z", "2026-10-13T12:00:04Z"
	f.run = benchmarkmodel.Run{
		Id: id("brn"), ExecutionId: f.execution.Id, TestAlias: "discovery", Repetition: 1,
		ModelId: f.model.Id, SystemPromptId: f.prompt.Id, QuestionId: f.question.Id, LispEnvironmentId: f.env.Id,
		WorkspaceId: &workspace, SessionId: &session, WorkspaceAgentId: &agent,
		AgentRequestId: &request, TerminalEventId: &terminal, StartedAt: &started, FinishedAt: &finished, Status: "completed",
	}
	metrics := map[string]interface{}{"input_tokens": json.Number("9007199254740993"), "output_tokens": json.Number("0"), "future": map[string]interface{}{"known": true}}
	f.events = []benchmarkmodel.SessionEvent{
		{Id: message, RunId: f.run.Id, Workspace: workspace, Session: session, Kind: "message.text", AuthorPrincipal: &principal,
			Payload: map[string]interface{}{"text": question}, CreatedAt: "2026-10-13T12:00:00Z"},
		{Id: request, RunId: f.run.Id, Workspace: workspace, Session: session, Parent: &message, Kind: "agent.request", AuthorPrincipal: &principal,
			Payload: map[string]interface{}{"agent": agent}, CreatedAt: "2026-10-13T12:00:01Z"},
		{Id: tool, RunId: f.run.Id, Workspace: workspace, Session: session, Parent: &request, Kind: "tool.request", AuthorAgent: &agent,
			Payload: map[string]interface{}{"name": "lisp", "code": "; inspect capabilities\n(help/env)\n"}, CreatedAt: "2026-10-13T12:00:02Z"},
		{Id: toolResult, RunId: f.run.Id, Workspace: workspace, Session: session, Parent: &tool, Kind: "tool.success", AuthorAgent: &agent,
			Payload: map[string]interface{}{"output": "(help/env session/files/list)"}, CreatedAt: "2026-10-13T12:00:03Z"},
		{Id: terminal, RunId: f.run.Id, Workspace: workspace, Session: session, Parent: &request, Kind: "agent.success", AuthorAgent: &agent,
			Payload: map[string]interface{}{"text": "Capabilities", "future": []interface{}{"one", "two"}}, Metrics: &metrics, CreatedAt: "2026-10-13T12:00:04Z"},
	}
	return f
}

func resultJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func execResult(t *testing.T, tx *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

// These test inserts exercise the storage contract directly with generated
// records. The production record writer is a subsequent implementation slice.
func insertResultInputs(t *testing.T, tx *sql.Tx, f resultsFixture) {
	t.Helper()
	e := f.execution
	execResult(t, tx, `INSERT INTO executions
		(id, config_path, config_api_version, gatehouse_version, gatehouse_fingerprint, started_at, finished_at, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Id, e.ConfigPath, e.ConfigApiVersion, e.GatehouseVersion, e.GatehouseFingerprint, e.StartedAt, e.FinishedAt, e.Status, e.Error)
	p := f.provider
	var credentials any
	if p.ApiKeySources != nil {
		credentials = resultJSON(t, *p.ApiKeySources)
	}
	execResult(t, tx, `INSERT INTO providers
		(id, execution_id, alias, protocol, base_url, api_key_sources, keychain, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Id, p.ExecutionId, p.Alias, p.Protocol, p.BaseUrl, credentials, p.Keychain, p.Enabled)
	m := f.model
	execResult(t, tx, `INSERT INTO models
		(id, execution_id, alias, provider_id, model, parameters, max_turns, max_output_tokens, compaction_algorithm, history_bytes, buffer_bytes, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.Id, m.ExecutionId, m.Alias, m.ProviderId, m.Model, resultJSON(t, m.Parameters), m.MaxTurns, m.MaxOutputTokens,
		m.CompactionAlgorithm, m.HistoryBytes, m.BufferBytes, m.Enabled)
	s := f.prompt
	execResult(t, tx, `INSERT INTO system_prompts (id, execution_id, alias, mode, text) VALUES (?, ?, ?, ?, ?)`,
		s.Id, s.ExecutionId, s.Alias, s.Mode, s.Text)
	q := f.question
	execResult(t, tx, `INSERT INTO questions (id, execution_id, alias, text) VALUES (?, ?, ?, ?)`,
		q.Id, q.ExecutionId, q.Alias, q.Text)
	env := f.env
	execResult(t, tx, `INSERT INTO lisp_environments (id, execution_id, alias, project_linked, prelude) VALUES (?, ?, ?, ?, ?)`,
		env.Id, env.ExecutionId, env.Alias, env.ProjectLinked, env.Prelude)
}

func insertResultRun(t *testing.T, tx *sql.Tx, r benchmarkmodel.Run) {
	t.Helper()
	execResult(t, tx, `INSERT INTO runs
		(id, execution_id, test_alias, model_id, system_prompt_id, question_id, lisp_environment_id, repetition,
		 workspace_id, workspace_agent_id, session_id, agent_request_id, terminal_event_id, started_at, finished_at, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Id, r.ExecutionId, r.TestAlias, r.ModelId, r.SystemPromptId, r.QuestionId, r.LispEnvironmentId, r.Repetition,
		r.WorkspaceId, r.WorkspaceAgentId, r.SessionId, r.AgentRequestId, r.TerminalEventId, r.StartedAt, r.FinishedAt, r.Status, r.Error)
}

func insertResultEvent(t *testing.T, tx *sql.Tx, e benchmarkmodel.SessionEvent) {
	t.Helper()
	var metrics any
	if e.Metrics != nil {
		metrics = resultJSON(t, *e.Metrics)
	}
	execResult(t, tx, `INSERT INTO session_events
		(id, run_id, workspace, session, parent, kind, author_principal, author_agent, author_gateway, payload, created_at, metrics)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Id, e.RunId, e.Workspace, e.Session, e.Parent, e.Kind, e.AuthorPrincipal, e.AuthorAgent, e.AuthorGateway,
		resultJSON(t, e.Payload), e.CreatedAt, metrics)
}

func insertResultFixture(t *testing.T, db *sql.DB, f resultsFixture) {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insertResultInputs(t, tx, f)
	// Children come before parents, and every event comes before its run.
	for index := len(f.events) - 1; index >= 0; index-- {
		insertResultEvent(t, tx, f.events[index])
	}
	insertResultRun(t, tx, f.run)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenResultsCreatesIsolatedSchema(t *testing.T) {
	first, second := openTestResults(t), openTestResults(t)
	var foreignKeys, version int
	if err := first.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err := first.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 || version != 1 || first.Stats().MaxOpenConnections != 1 {
		t.Fatalf("unexpected database settings: foreign_keys=%d version=%d pool=%#v", foreignKeys, version, first.Stats())
	}
	rows, err := first.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	want := []string{"executions", "lisp_environments", "models", "providers", "questions", "runs", "session_events", "system_prompts"}
	if !reflect.DeepEqual(tables, want) {
		t.Fatalf("tables = %v, want %v", tables, want)
	}
	insertResultFixture(t, first, newResultsFixture(t, "first database"))
	var count int
	if err := second.QueryRow(`SELECT COUNT(*) FROM executions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("separate in-memory database contains %d executions", count)
	}
}

func TestResultsPreserveExecutionSnapshotsAndEventTrees(t *testing.T) {
	db := openTestResults(t)
	first, second := newResultsFixture(t, "original question"), newResultsFixture(t, "changed question")
	second.env.Prelude, second.env.ProjectLinked, second.prompt.Text = nil, nil, nil
	second.prompt.Mode = "builtin"
	second.model.CompactionAlgorithm, second.model.HistoryBytes = nil, nil
	second.provider.ApiKeySources = nil
	second.provider.BaseUrl = nil
	second.provider.Protocol = "builtin"
	second.model.Model = "dummy.fixed-reply"
	second.model.Parameters = map[string]interface{}{"text": "reply"}
	second.events[len(second.events)-1].Metrics = nil
	insertResultFixture(t, db, first)
	insertResultFixture(t, db, second)
	for _, f := range []resultsFixture{first, second} {
		var executionID, modelID, providerID, promptID, questionID, environmentID, text, terminalID string
		if err := db.QueryRow(`SELECT x.id, m.id, p.id, s.id, q.id, e.id, q.text, t.id
			FROM runs r JOIN executions x ON x.id = r.execution_id
			JOIN models m ON m.execution_id = r.execution_id AND m.id = r.model_id
			JOIN providers p ON p.execution_id = m.execution_id AND p.id = m.provider_id
			JOIN system_prompts s ON s.execution_id = r.execution_id AND s.id = r.system_prompt_id
			JOIN questions q ON q.execution_id = r.execution_id AND q.id = r.question_id
			JOIN lisp_environments e ON e.execution_id = r.execution_id AND e.id = r.lisp_environment_id
			JOIN session_events t ON t.run_id = r.id AND t.id = r.terminal_event_id
			WHERE r.id = ?`, f.run.Id).Scan(&executionID, &modelID, &providerID, &promptID, &questionID, &environmentID, &text, &terminalID); err != nil {
			t.Fatal(err)
		}
		if executionID != f.execution.Id || modelID != f.model.Id || providerID != f.provider.Id || promptID != f.prompt.Id || questionID != f.question.Id || environmentID != f.env.Id || text != f.question.Text || terminalID != *f.run.TerminalEventId {
			t.Fatalf("snapshot/run/event join changed supplied identities or content for %s", f.execution.Id)
		}
		for _, event := range f.events {
			var payload, workspace, session, kind, createdAt string
			var parent, principal, agent, gateway, metrics sql.NullString
			if err := db.QueryRow(`SELECT workspace, session, kind, parent, author_principal, author_agent, author_gateway, payload, created_at, metrics
				FROM session_events WHERE run_id = ? AND id = ?`, f.run.Id, event.Id).
				Scan(&workspace, &session, &kind, &parent, &principal, &agent, &gateway, &payload, &createdAt, &metrics); err != nil {
				t.Fatal(err)
			}
			if workspace != event.Workspace || session != event.Session || kind != event.Kind || createdAt != event.CreatedAt || payload != resultJSON(t, event.Payload) {
				t.Fatalf("native event data changed for %s", event.Id)
			}
			for _, author := range []struct {
				got  sql.NullString
				want *string
			}{{parent, event.Parent}, {principal, event.AuthorPrincipal}, {agent, event.AuthorAgent}, {gateway, event.AuthorGateway}} {
				if author.got.Valid != (author.want != nil) || (author.want != nil && author.got.String != *author.want) {
					t.Fatalf("native event reference changed for %s", event.Id)
				}
			}
			if metrics.Valid != (event.Metrics != nil) || (event.Metrics != nil && metrics.String != resultJSON(t, *event.Metrics)) {
				t.Fatalf("native metrics changed for %s", event.Id)
			}
		}
	}
	var emptyPrelude, emptyPrompt sql.NullString
	var explicitFalse sql.NullInt64
	if err := db.QueryRow(`SELECT prelude, project_linked FROM lisp_environments WHERE id = ?`, first.env.Id).Scan(&emptyPrelude, &explicitFalse); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT text FROM system_prompts WHERE id = ?`, first.prompt.Id).Scan(&emptyPrompt); err != nil {
		t.Fatal(err)
	}
	if !emptyPrelude.Valid || emptyPrelude.String != "" || !emptyPrompt.Valid || emptyPrompt.String != "" || !explicitFalse.Valid || explicitFalse.Int64 != 0 {
		t.Fatal("explicit empty strings/false must survive persistence")
	}
	var nullPrelude, nullPrompt sql.NullString
	var nullLinked sql.NullInt64
	if err := db.QueryRow(`SELECT prelude, project_linked FROM lisp_environments WHERE id = ?`, second.env.Id).Scan(&nullPrelude, &nullLinked); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT text FROM system_prompts WHERE id = ?`, second.prompt.Id).Scan(&nullPrompt); err != nil {
		t.Fatal(err)
	}
	if nullPrelude.Valid || nullPrompt.Valid || nullLinked.Valid {
		t.Fatal("omitted settings must remain SQL NULL")
	}
	var algorithm sql.NullString
	var history, buffer, turns, tokens, enabled sql.NullInt64
	if err := db.QueryRow(`SELECT compaction_algorithm, history_bytes, buffer_bytes, max_turns, max_output_tokens, enabled FROM models WHERE id = ?`, first.model.Id).
		Scan(&algorithm, &history, &buffer, &turns, &tokens, &enabled); err != nil {
		t.Fatal(err)
	}
	if !algorithm.Valid || algorithm.String != "mcmtr" || !history.Valid || history.Int64 != 64 || buffer.Valid || turns.Valid || tokens.Valid || enabled.Valid {
		t.Fatal("partial compaction or omitted model settings were defaulted")
	}
	var parameters, credentials string
	if err := db.QueryRow(`SELECT parameters FROM models WHERE id = ?`, first.model.Id).Scan(&parameters); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT api_key_sources FROM providers WHERE id = ?`, first.provider.Id).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if parameters != resultJSON(t, first.model.Parameters) || credentials != resultJSON(t, *first.provider.ApiKeySources) {
		t.Fatal("JSON parameters or credential descriptors changed")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("captured graph contains a foreign-key violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestResultsRejectCrossExecutionLinksAndDuplicateTrials(t *testing.T) {
	db := openTestResults(t)
	first, second := newResultsFixture(t, "first"), newResultsFixture(t, "second")
	insertResultFixture(t, db, first)
	insertResultFixture(t, db, second)
	for _, column := range []string{"model_id", "system_prompt_id", "question_id", "lisp_environment_id"} {
		t.Run(column, func(t *testing.T) {
			value := map[string]string{"model_id": second.model.Id, "system_prompt_id": second.prompt.Id, "question_id": second.question.Id, "lisp_environment_id": second.env.Id}[column]
			if _, err := db.Exec(`UPDATE runs SET `+column+` = ? WHERE id = ?`, value, first.run.Id); err == nil {
				t.Fatal("run accepted an input from another execution")
			}
		})
	}
	if _, err := db.Exec(`UPDATE models SET provider_id = ? WHERE id = ?`, second.provider.Id, first.model.Id); err == nil {
		t.Fatal("model accepted a provider from another execution")
	}
	if _, err := db.Exec(`INSERT INTO questions (id, execution_id, alias, text) VALUES (?, ?, ?, ?)`, "duplicate-question", first.execution.Id, first.question.Alias, "duplicate"); err == nil {
		t.Fatal("duplicate alias accepted within one execution")
	}
	if _, err := db.Exec(`INSERT INTO runs (id, execution_id, test_alias, model_id, system_prompt_id, question_id, lisp_environment_id, repetition, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'planned')`, "duplicate-run", first.execution.Id, first.run.TestAlias, first.model.Id, first.prompt.Id, first.question.Id, first.env.Id, first.run.Repetition); err == nil {
		t.Fatal("duplicate combination/repetition accepted")
	}
}

func TestResultsRejectCrossRunEventReferences(t *testing.T) {
	for _, field := range []string{"parent", "agent_request_id", "terminal_event_id"} {
		t.Run(field, func(t *testing.T) {
			db := openTestResults(t)
			first, second := newResultsFixture(t, "first"), newResultsFixture(t, "second")
			insertResultFixture(t, db, first)
			insertResultFixture(t, db, second)
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if field == "parent" {
				execResult(t, tx, `UPDATE session_events SET parent = ? WHERE id = ?`, first.run.AgentRequestId, second.events[len(second.events)-1].Id)
			} else {
				execResult(t, tx, `UPDATE runs SET `+field+` = ? WHERE id = ?`, first.run.TerminalEventId, second.run.Id)
			}
			if err := tx.Commit(); err == nil {
				t.Fatal("event reference accepted from another run")
			}
		})
	}
}
