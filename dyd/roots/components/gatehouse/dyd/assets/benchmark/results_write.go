package benchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"gatehouse/benchmarkmodel"
)

// InsertExecution stores the supplied record in the caller's transaction. All
// insert functions preserve caller-assigned identities and use plain INSERTs.
// The caller commits or rolls back the complete record batch.
func InsertExecution(ctx context.Context, tx *sql.Tx, e benchmarkmodel.Execution) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO executions
		(id, config_path, config_api_version, gatehouse_version, gatehouse_fingerprint, started_at, finished_at, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Id, e.ConfigPath, e.ConfigApiVersion, e.GatehouseVersion, e.GatehouseFingerprint, e.StartedAt, e.FinishedAt, e.Status, e.Error)
	if err != nil {
		return fmt.Errorf("insert benchmark execution %q: %w", e.Id, err)
	}
	return nil
}

func InsertProvider(ctx context.Context, tx *sql.Tx, p benchmarkmodel.Provider) error {
	var credentials any
	if p.ApiKeySources != nil {
		encoded, err := json.Marshal(*p.ApiKeySources)
		if err != nil {
			return fmt.Errorf("encode benchmark provider %q credential sources: %w", p.Id, err)
		}
		credentials = string(encoded)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO providers
		(id, execution_id, alias, protocol, endpoint, api_key_sources, keychain, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Id, p.ExecutionId, p.Alias, p.Protocol, p.Endpoint, credentials, p.Keychain, p.Enabled)
	if err != nil {
		return fmt.Errorf("insert benchmark provider %q: %w", p.Id, err)
	}
	return nil
}

func InsertModel(ctx context.Context, tx *sql.Tx, m benchmarkmodel.Model) error {
	parameters, err := json.Marshal(m.Parameters)
	if err != nil {
		return fmt.Errorf("encode benchmark model %q parameters: %w", m.Id, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO models
		(id, execution_id, alias, provider_id, model, parameters, max_turns, max_output_tokens, compaction_algorithm, history_bytes, buffer_bytes, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.Id, m.ExecutionId, m.Alias, m.ProviderId, m.Model, string(parameters), m.MaxTurns, m.MaxOutputTokens,
		m.CompactionAlgorithm, m.HistoryBytes, m.BufferBytes, m.Enabled)
	if err != nil {
		return fmt.Errorf("insert benchmark model %q: %w", m.Id, err)
	}
	return nil
}

func InsertSystemPrompt(ctx context.Context, tx *sql.Tx, p benchmarkmodel.SystemPrompt) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO system_prompts (id, execution_id, alias, mode, text) VALUES (?, ?, ?, ?, ?)`,
		p.Id, p.ExecutionId, p.Alias, p.Mode, p.Text)
	if err != nil {
		return fmt.Errorf("insert benchmark system prompt %q: %w", p.Id, err)
	}
	return nil
}

func InsertQuestion(ctx context.Context, tx *sql.Tx, q benchmarkmodel.Question) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO questions
		(id, execution_id, alias, text, expected_output, expected_max_turns, expected_max_tool_failures, expected_max_tool_calls)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		q.Id, q.ExecutionId, q.Alias, q.Text, q.ExpectedOutput, q.ExpectedMaxTurns, q.ExpectedMaxToolFailures, q.ExpectedMaxToolCalls)
	if err != nil {
		return fmt.Errorf("insert benchmark question %q: %w", q.Id, err)
	}
	return nil
}

func InsertLispEnvironment(ctx context.Context, tx *sql.Tx, e benchmarkmodel.LispEnvironment) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO lisp_environments (id, execution_id, alias, project_linked, prelude) VALUES (?, ?, ?, ?, ?)`,
		e.Id, e.ExecutionId, e.Alias, e.ProjectLinked, e.Prelude)
	if err != nil {
		return fmt.Errorf("insert benchmark Lisp environment %q: %w", e.Id, err)
	}
	return nil
}

func InsertRun(ctx context.Context, tx *sql.Tx, r benchmarkmodel.Run) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO runs
		(id, execution_id, test_alias, model_id, system_prompt_id, question_id, lisp_environment_id, repetition,
		 workspace_id, workspace_agent_id, session_id, agent_request_id, terminal_event_id, started_at, finished_at, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Id, r.ExecutionId, r.TestAlias, r.ModelId, r.SystemPromptId, r.QuestionId, r.LispEnvironmentId, r.Repetition,
		r.WorkspaceId, r.WorkspaceAgentId, r.SessionId, r.AgentRequestId, r.TerminalEventId, r.StartedAt, r.FinishedAt, r.Status, r.Error)
	if err != nil {
		return fmt.Errorf("insert benchmark run %q: %w", r.Id, err)
	}
	return nil
}

func InsertSessionEvent(ctx context.Context, tx *sql.Tx, e benchmarkmodel.SessionEvent) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("encode benchmark event %q payload: %w", e.Id, err)
	}
	var metrics any
	if e.Metrics != nil {
		encoded, err := json.Marshal(*e.Metrics)
		if err != nil {
			return fmt.Errorf("encode benchmark event %q metrics: %w", e.Id, err)
		}
		metrics = string(encoded)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO session_events
		(id, run_id, workspace, session, parent, kind, author_principal, author_agent, author_gateway, payload, created_at, metrics)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Id, e.RunId, e.Workspace, e.Session, e.Parent, e.Kind, e.AuthorPrincipal, e.AuthorAgent, e.AuthorGateway,
		string(payload), e.CreatedAt, metrics)
	if err != nil {
		return fmt.Errorf("insert benchmark event %q: %w", e.Id, err)
	}
	return nil
}

// UpdateExecution changes lifecycle fields only. Configuration and build identity
// remain those originally inserted. A missing identity returns sql.ErrNoRows.
func UpdateExecution(ctx context.Context, tx *sql.Tx, e benchmarkmodel.Execution) error {
	result, err := tx.ExecContext(ctx, `UPDATE executions SET finished_at = ?, status = ?, error = ? WHERE id = ?`,
		e.FinishedAt, e.Status, e.Error, e.Id)
	if err != nil {
		return fmt.Errorf("update benchmark execution %q: %w", e.Id, err)
	}
	return requireResultRow(result, "execution", e.Id)
}

// UpdateRun changes lifecycle fields and native source references only. It
// addresses the run within its execution without retargeting selected inputs.
func UpdateRun(ctx context.Context, tx *sql.Tx, r benchmarkmodel.Run) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET
		workspace_id = ?, workspace_agent_id = ?, session_id = ?, agent_request_id = ?, terminal_event_id = ?,
		started_at = ?, finished_at = ?, status = ?, error = ? WHERE id = ? AND execution_id = ?`,
		r.WorkspaceId, r.WorkspaceAgentId, r.SessionId, r.AgentRequestId, r.TerminalEventId,
		r.StartedAt, r.FinishedAt, r.Status, r.Error, r.Id, r.ExecutionId)
	if err != nil {
		return fmt.Errorf("update benchmark run %q: %w", r.Id, err)
	}
	return requireResultRow(result, "run", r.Id)
}

func requireResultRow(result sql.Result, kind, id string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update benchmark %s %q row count: %w", kind, id, err)
	}
	if count == 0 {
		return fmt.Errorf("update benchmark %s %q: %w", kind, id, sql.ErrNoRows)
	}
	return nil
}
