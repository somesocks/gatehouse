PRAGMA user_version = 3;

CREATE TABLE executions (
    id TEXT PRIMARY KEY NOT NULL,
    config_path TEXT NOT NULL,
    config_api_version TEXT NOT NULL,
    gatehouse_version TEXT NOT NULL,
    gatehouse_fingerprint TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'cancelled')),
    error TEXT
) STRICT;

CREATE TABLE execution_inputs (
    execution_id TEXT PRIMARY KEY NOT NULL REFERENCES executions(id),
    run_count TEXT NOT NULL,
    configuration TEXT NOT NULL CHECK (json_valid(configuration)),
    sources TEXT NOT NULL CHECK (json_valid(sources))
) STRICT;

CREATE TABLE providers (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    alias TEXT NOT NULL,
    protocol TEXT NOT NULL,
    endpoint TEXT,
    api_key_sources TEXT CHECK (api_key_sources IS NULL OR (json_valid(api_key_sources) AND json_type(api_key_sources) = 'array')),
    keychain TEXT,
    enabled INTEGER CHECK (enabled IS NULL OR enabled IN (0, 1)),
    UNIQUE (execution_id, alias),
    UNIQUE (execution_id, id)
) STRICT;

CREATE TABLE models (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    alias TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    parameters TEXT NOT NULL CHECK (json_valid(parameters)) CHECK (json_type(parameters) = 'object'),
    max_turns INTEGER,
    max_output_tokens INTEGER,
    compaction_algorithm TEXT,
    history_bytes INTEGER,
    buffer_bytes INTEGER,
    enabled INTEGER CHECK (enabled IS NULL OR enabled IN (0, 1)),
    UNIQUE (execution_id, alias),
    UNIQUE (execution_id, id),
    FOREIGN KEY (execution_id, provider_id) REFERENCES providers(execution_id, id)
) STRICT;

CREATE TABLE system_prompts (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    alias TEXT NOT NULL,
    mode TEXT NOT NULL,
    text TEXT,
    UNIQUE (execution_id, alias),
    UNIQUE (execution_id, id)
) STRICT;

CREATE TABLE questions (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    alias TEXT NOT NULL,
    text TEXT NOT NULL,
    expected_output TEXT,
    expected_max_turns INTEGER CHECK (expected_max_turns IS NULL OR expected_max_turns >= 0),
    expected_max_tool_failures INTEGER CHECK (expected_max_tool_failures IS NULL OR expected_max_tool_failures >= 0),
    expected_max_tool_calls INTEGER CHECK (expected_max_tool_calls IS NULL OR expected_max_tool_calls >= 0),
    UNIQUE (execution_id, alias),
    UNIQUE (execution_id, id)
) STRICT;

CREATE TABLE lisp_environments (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    alias TEXT NOT NULL,
    project_linked INTEGER CHECK (project_linked IS NULL OR project_linked IN (0, 1)),
    prelude TEXT,
    UNIQUE (execution_id, alias),
    UNIQUE (execution_id, id)
) STRICT;

CREATE TABLE runs (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    test_alias TEXT NOT NULL,
    model_id TEXT NOT NULL,
    system_prompt_id TEXT NOT NULL,
    question_id TEXT NOT NULL,
    lisp_environment_id TEXT NOT NULL,
    repetition INTEGER NOT NULL CHECK (repetition > 0),
    workspace_id TEXT,
    workspace_agent_id TEXT,
    session_id TEXT,
    agent_request_id TEXT,
    terminal_event_id TEXT,
    started_at TEXT,
    finished_at TEXT,
    status TEXT NOT NULL CHECK (status IN ('planned', 'running', 'completed', 'failed', 'cancelled')),
    error TEXT,
    UNIQUE (execution_id, test_alias, model_id, system_prompt_id, question_id, lisp_environment_id, repetition),
    FOREIGN KEY (execution_id, model_id) REFERENCES models(execution_id, id),
    FOREIGN KEY (execution_id, system_prompt_id) REFERENCES system_prompts(execution_id, id),
    FOREIGN KEY (execution_id, question_id) REFERENCES questions(execution_id, id),
    FOREIGN KEY (execution_id, lisp_environment_id) REFERENCES lisp_environments(execution_id, id),
    FOREIGN KEY (id, agent_request_id) REFERENCES session_events(run_id, id)
        DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (id, terminal_event_id) REFERENCES session_events(run_id, id)
        DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE TABLE session_events (
    id TEXT PRIMARY KEY NOT NULL,
    run_id TEXT NOT NULL REFERENCES runs(id) DEFERRABLE INITIALLY DEFERRED,
    workspace TEXT NOT NULL,
    session TEXT NOT NULL,
    parent TEXT,
    kind TEXT NOT NULL,
    author_principal TEXT,
    author_agent TEXT,
    author_gateway TEXT,
    payload TEXT NOT NULL CHECK (json_valid(payload)) CHECK (json_type(payload) = 'object'),
    created_at TEXT NOT NULL,
    metrics TEXT CHECK (metrics IS NULL OR (json_valid(metrics) AND json_type(metrics) = 'object')),
    UNIQUE (run_id, id),
    FOREIGN KEY (run_id, parent) REFERENCES session_events(run_id, id)
        DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE INDEX session_events_by_run_kind ON session_events(run_id, kind);
CREATE INDEX session_events_by_parent ON session_events(run_id, parent);

CREATE TABLE artifacts (
    run_id TEXT NOT NULL REFERENCES runs(id),
    id TEXT NOT NULL,
    metadata TEXT NOT NULL CHECK (json_valid(metadata)),
    content BLOB,
    PRIMARY KEY (run_id, id)
) STRICT;

CREATE VIEW run_measurements AS
SELECT r.id AS run_id,
       COUNT(CASE WHEN e.kind = 'tool.request' THEN 1 END) AS tool_calls,
       COUNT(CASE WHEN e.kind = 'tool.failure' THEN 1 END) AS tool_failures,
       COUNT(CASE WHEN e.kind = 'thinking.success'
                   AND json_array_length(e.payload, '$.tool_requests') > 0 THEN 1 END) AS turns,
       SUM(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update')
                THEN json_extract(e.metrics, '$.input_tokens') END) AS input_tokens,
       SUM(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update')
                THEN json_extract(e.metrics, '$.output_tokens') END) AS output_tokens,
       SUM(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update')
                THEN json_extract(e.metrics, '$.reasoning_tokens') END) AS reasoning_tokens,
       SUM(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update')
                THEN json_extract(e.metrics, '$.cached_input_tokens') END) AS cached_input_tokens,
       SUM(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update')
                THEN json_extract(e.metrics, '$.total_tokens') END) AS total_tokens,
       SUM(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update')
                THEN json_extract(e.metrics, '$.request_ms') END) AS request_ms,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') THEN 1 END) AS provider_outcomes,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND e.metrics IS NOT NULL THEN 1 END) AS outcomes_with_metrics,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND json_extract(e.metrics, '$.input_tokens') IS NOT NULL THEN 1 END) AS input_tokens_observations,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND json_extract(e.metrics, '$.output_tokens') IS NOT NULL THEN 1 END) AS output_tokens_observations,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND json_extract(e.metrics, '$.reasoning_tokens') IS NOT NULL THEN 1 END) AS reasoning_tokens_observations,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND json_extract(e.metrics, '$.cached_input_tokens') IS NOT NULL THEN 1 END) AS cached_input_tokens_observations,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND json_extract(e.metrics, '$.total_tokens') IS NOT NULL THEN 1 END) AS total_tokens_observations,
       COUNT(CASE WHEN e.kind IN ('thinking.success', 'thinking.failure', 'thinking.update') AND json_extract(e.metrics, '$.request_ms') IS NOT NULL THEN 1 END) AS request_ms_observations
FROM runs r LEFT JOIN session_events e ON e.run_id = r.id GROUP BY r.id;

CREATE VIEW run_checks AS
SELECT r.*, m.alias AS model_alias, m.model, m.parameters,
       p.alias AS prompt_alias, q.alias AS question_alias, l.alias AS environment_alias,
       json_extract(e.payload, '$.text') AS output,
       q.expected_output, q.expected_max_turns, q.expected_max_tool_failures, q.expected_max_tool_calls,
       a.turns, a.tool_calls, a.tool_failures, a.input_tokens, a.output_tokens, a.reasoning_tokens,
       a.cached_input_tokens, a.total_tokens, a.request_ms, a.provider_outcomes, a.outcomes_with_metrics,
       a.input_tokens_observations, a.output_tokens_observations, a.reasoning_tokens_observations,
       a.cached_input_tokens_observations, a.total_tokens_observations, a.request_ms_observations,
       CASE WHEN r.finished_at IS NOT NULL AND r.started_at IS NOT NULL
            THEN MAX(0, CAST(ROUND((julianday(r.finished_at) - julianday(r.started_at)) * 86400000) AS INTEGER)) END AS elapsed_ms,
       CASE WHEN q.expected_output IS NOT NULL THEN
            COALESCE(trim(json_extract(e.payload, '$.text'), char(9,10,11,12,13,32,133,160,5760,8192,8193,8194,8195,8196,8197,8198,8199,8200,8201,8202,8232,8233,8239,8287,12288)) =
                     trim(q.expected_output, char(9,10,11,12,13,32,133,160,5760,8192,8193,8194,8195,8196,8197,8198,8199,8200,8201,8202,8232,8233,8239,8287,12288)), 0) END AS output_passed,
       CASE WHEN q.expected_max_turns IS NOT NULL THEN a.turns <= q.expected_max_turns END AS turns_passed,
       CASE WHEN q.expected_max_tool_failures IS NOT NULL THEN a.tool_failures <= q.expected_max_tool_failures END AS tool_failures_passed,
       CASE WHEN q.expected_max_tool_calls IS NOT NULL THEN a.tool_calls <= q.expected_max_tool_calls END AS tool_calls_passed
FROM runs r JOIN models m ON m.id = r.model_id JOIN system_prompts p ON p.id = r.system_prompt_id
JOIN questions q ON q.id = r.question_id JOIN lisp_environments l ON l.id = r.lisp_environment_id
JOIN run_measurements a ON a.run_id = r.id
LEFT JOIN session_events e ON e.id = r.terminal_event_id AND e.run_id = r.id;

CREATE VIEW run_results AS
SELECT *, CASE
    WHEN status IN ('planned', 'running') THEN 'pending'
    WHEN status = 'cancelled' THEN 'cancelled'
    WHEN status <> 'completed' THEN 'error'
    WHEN expected_output IS NULL AND expected_max_turns IS NULL AND expected_max_tool_failures IS NULL AND expected_max_tool_calls IS NULL THEN 'ungraded'
    WHEN COALESCE(output_passed, 1) AND COALESCE(turns_passed, 1) AND COALESCE(tool_failures_passed, 1) AND COALESCE(tool_calls_passed, 1) THEN 'passed'
    ELSE 'failed' END AS grade
FROM run_checks;

CREATE VIEW execution_summary AS
SELECT e.id AS execution_id, e.status, i.run_count AS planned_runs,
       CASE WHEN e.finished_at IS NOT NULL THEN MAX(0, CAST(ROUND((julianday(e.finished_at) - julianday(e.started_at)) * 86400000) AS INTEGER)) END AS elapsed_ms,
       COUNT(r.id) AS materialized_runs,
       COUNT(CASE WHEN r.grade = 'passed' THEN 1 END) AS passed,
       COUNT(CASE WHEN r.grade = 'failed' THEN 1 END) AS failed,
       COUNT(CASE WHEN r.grade = 'ungraded' THEN 1 END) AS ungraded,
       COUNT(CASE WHEN r.grade = 'error' THEN 1 END) AS errors,
       COUNT(CASE WHEN r.grade = 'cancelled' THEN 1 END) AS cancelled,
       COUNT(CASE WHEN r.grade = 'pending' THEN 1 END) AS pending,
       SUM(r.input_tokens) AS input_tokens, SUM(r.output_tokens) AS output_tokens,
       SUM(r.reasoning_tokens) AS reasoning_tokens, SUM(r.cached_input_tokens) AS cached_input_tokens,
       SUM(r.total_tokens) AS total_tokens, SUM(r.request_ms) AS request_ms, SUM(r.elapsed_ms) AS run_ms,
       COALESCE(SUM(r.provider_outcomes), 0) AS provider_outcomes, COALESCE(SUM(r.outcomes_with_metrics), 0) AS outcomes_with_metrics,
       COALESCE(SUM(r.input_tokens_observations), 0) AS input_tokens_observations,
       COALESCE(SUM(r.output_tokens_observations), 0) AS output_tokens_observations,
       COALESCE(SUM(r.reasoning_tokens_observations), 0) AS reasoning_tokens_observations,
       COALESCE(SUM(r.cached_input_tokens_observations), 0) AS cached_input_tokens_observations,
       COALESCE(SUM(r.total_tokens_observations), 0) AS total_tokens_observations,
       COALESCE(SUM(r.request_ms_observations), 0) AS request_ms_observations,
       COALESCE(SUM(r.turns), 0) AS turns, COALESCE(SUM(r.tool_calls), 0) AS tool_calls,
       COALESCE(SUM(r.tool_failures), 0) AS tool_failures
FROM executions e LEFT JOIN execution_inputs i ON i.execution_id = e.id
LEFT JOIN run_results r ON r.execution_id = e.id GROUP BY e.id;
