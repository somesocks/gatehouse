PRAGMA user_version = 1;

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

CREATE TABLE providers (
    id TEXT PRIMARY KEY NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    alias TEXT NOT NULL,
    protocol TEXT NOT NULL,
    base_url TEXT,
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
