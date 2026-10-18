# Running benchmarks

```sh
gatehouse benchmark run --config=tests.yaml
gatehouse benchmark run --config=tests.yaml --parallel=8 --report=results --format=text
gatehouse benchmark run --config=tests.yaml --report=results --format=json > results.json
gatehouse benchmark run --config=tests.yaml --report=full --format=jsonl > results.jsonl
gatehouse benchmark run --config=tests.yaml --report=full --format=sqlite > results.sqlite
gatehouse benchmark run --config=tests.yaml --report=full --format=sqlite --output=results.sqlite
```

All value-bearing flags use `--name=value`. `--config` is required. `--report`
defaults to `summary` and accepts `summary`, `results`, or `full`. `--format`
defaults to `text` and accepts `text`, `json`, `jsonl`, or `sqlite`. SQLite requires
`--report=full`. Invalid combinations and invalid input are rejected before model
requests. `--output=<path>` writes the report to a file, creating or truncating it.
The default, also available explicitly as `--output=-`, writes to stdout for shell
redirection. Output files are opened before execution; an unavailable destination
fails before model requests. Progress and runtime logging go to stderr. Diagnostics
follow their configured destinations; the command does not override them or check
whether they share an output with the report. `--parallel` is a positive integer
limiting active benchmark runs; it defaults to **8**. Use `--parallel=1` for serial
execution. This is independent of Dryad's similarly named option.

## Execution and isolation

The runner expands the validated matrix lazily through bounded workers, using
Gatehouse's normal session-reply runtime. One execution shares an ephemeral
operational database, keyring, and DBOS runtime, initialized and shut down once.
Selected providers and models are registered once; each model/prompt/environment
combination reuses a workspace and agent binding with the stable alias `assistant`.
Every run gets a fresh named session and (when selected) a fresh linked project,
so conversation history and project state do not leak between runs. Replacement
prompts are immutable per-binding settings, allowing different prompt modes to
run concurrently. Naming sessions avoids unrelated automatic-naming model calls.
Lisp evaluation, provider calls/retries, limits, tool execution, and native event
creation use the same runtime as ordinary Gatehouse sessions.

Only selected providers' credentials are resolved, once per execution, from their
declared sources. Credentials are encrypted in disposable runtime state using
execution-local random keychain passphrases. Captured provider snapshots contain
credential source descriptors. The benchmark requires no daemon configuration or
interactive keychain passphrase.

Prompt modes `builtin`, `append`, and `replace` are applied literally. Omitted
preludes use the normal Gatehouse prelude; explicitly empty preludes evaluate the
agent's program directly. Custom preludes are complete wrappers that normally end
with `(eval program)`.

Runtime defaults are applied at the execution boundary: 127 tool-using turns,
16,000 output tokens, and MCMTR history/buffer capacities of 96/48 KiB. A supplied
history without a buffer uses the smaller of 48 KiB and half that history. A
supplied buffer without a history uses at least twice that buffer. Invalid effective
capacities fail before model requests. Input snapshots preserve the original
declarations, including omitted settings.

Individual agent failures are captured and execution continues with later runs.
Infrastructure failures stop the execution. Input and approval requests fail the
unattended run through native cancellation, rather than waiting for a human.
SIGINT/SIGTERM cancel active work through the same native cancellation path. Partial
captures are finalized and reported using an independent cleanup context. On
interruption or infrastructure failure, workers stop taking new runs and active
sessions are drained/cancelled before the shared runtime shuts down. Per-run
timing starts when a worker takes a run, excluding time waiting for a worker slot;
shared initialization/shutdown remains part of overall execution elapsed time.
Both native cancellation outcomes (`cancel.success` and `cancel.failure` when
completion wins the race) end the cancellation wait, retaining the recorded agent
terminal event.

Configured diagnostic signals (USR1, USR2, HUP) are serviced during the command and
emit to their configured destinations. They are separate from SIGINT/SIGTERM,
which interrupt execution.

## Expectations

Questions may specify any nonempty subset of:

```yaml
questions:
  - alias: rome-temperature
    text: "What temperature is forecast for Rome? Return only the number."
    expected:
      output: "24"
      max_turns: 3
      max_tool_failures: 0
      max_tool_calls: 3
```

`output` is a string compared to the final answer after trimming surrounding
Unicode whitespace on both sides. Limits are nonnegative and inclusive. Omitted
checks are skipped; explicit zero remains a real expectation. Expectations are
captured as input data and are not included in the agent's question.

Turns match the engine's tool-using round count. A batch of several tool calls
uses one turn; the final-answer round and provider retries use no extra turns.
Each native tool invocation counts as one tool call, independently of the number
of Lisp functions it invokes. Tool-failure events count even if the agent recovers.
Expectations grade observed behavior; model execution limits are configured
separately.

## Results database and grading

A second, private in-memory SQLite database is the source of truth for all reports.
The runner always captures the complete dataset, regardless of report selection:

- execution, original loaded configuration/source references, and planned run count;
- provider/model/prompt/question/environment snapshots, including expectations;
- individual runs, original native events, payloads, metrics, and source IDs;
- session-file metadata and file contents, including spilled tool/provider output.

Files with unfinished storage objects retain metadata and a null content field.
Completed artifacts have binary contents. The operational database is closed after
capture; saved results retain the files needed to inspect the events.

SQLite schema version 3 supplies `run_measurements`, `run_checks`, `run_results`,
and `execution_summary` views. These derive grading and reporting directly from
captured inputs and events. `run_checks` exposes each optional check independently.
`run_results.grade` is `passed`, `failed`, `ungraded`, `error`, `cancelled`, or
`pending`. Run lifecycle status is separate: a completed run can fail expectations.
Completed runs without expectations are ungraded.

Provider usage remains null when unavailable. Known zero usage stays zero.
Aggregates include provider-outcome counts, the number with metrics, and per-field
observation counts, so partially reported usage remains distinguishable from complete measurements. Provider retries
are captured as native thinking updates; tool counts come from native tool events.

## Output

- `summary`: aggregate grades/statuses/measurements and model/prompt/environment groups.
- `results`: summary plus every individual run's answer, checks, grade, and measurements.
- `full`: results plus every capture table and the SQL schema/grading views.

JSON is one document. JSONL uses typed records (`schema`, capture table names,
`result`, `group`, `summary`); full captures start with the schema and finish with
summary records. Artifact bytes use base64 in JSON/JSONL. Full text output is an
indented complete representation; summary/results text output uses readable tables.
Text group rows include execution and test identity. Individual text results show
each supplied expectation beside its observed value and SQL check outcome; omitted
checks are labelled unchecked. Token and provider-time sums include their observation
counts, for example `10 (1/2 outcomes reported)`, so partial totals are explicit in
text as well as JSON/JSONL groups.

SQLite output exports a finalized database to a private temporary file, copies it
to the selected report output, and cleans up. Its views can be queried with ordinary SQLite tools, and
its original data can be regraded without more model requests. SQLite files cannot
be combined using shell append redirection.

Exit status is 0 when execution and all supplied expectations succeed (ungraded
completed runs are allowed), 1 for execution/grading/report failures, 130 for
SIGINT, and 143 for SIGTERM. Interrupted executions still attempt to emit their
partial report.
The first recorded interruption retains exit status 130/143 even if reporting or
closing its output fails; those failures are still written to stderr.
