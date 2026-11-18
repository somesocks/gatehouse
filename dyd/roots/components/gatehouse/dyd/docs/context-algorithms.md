# Agent context algorithms

The selected agent model's `compaction` configuration chooses context preparation
within the normal reply/round runtime. Admission, participant and authorization
checks, cancellation, provider transport, rate limits, tools and outcome persistence
are shared. MCMTR remains the default. TADW supports SQLite and PostgreSQL and requires an
external provider; builtin models use MCMTR.

## Configuration

`libraries/gatehouse-schema/dyd/assets/config/document.dhall` defines
`AgentModelCompaction` as a `oneOf` of references to named `MCMTRConfig` and
`TADWConfig` records. The flat wire discriminator is `algorithm`.

Omitted compaction selects the existing model-specific MCMTR defaults. Its fields
remain optional in file configuration:

```yaml
compaction:
  algorithm: mcmtr
  history_bytes: 98304
  buffer_bytes: 49152
```

MCMTR requires positive capacities and `buffer_bytes <= floor(history_bytes / 2)`.
The administration API accepts an effective JSON configuration with both limits
explicit, as before.

TADW requires all three limits:

```yaml
compaction:
  algorithm: tadw
  context_max_bytes: 262144
  history_max_bytes: 131072
  history_min_bytes: 65536
```

They satisfy:

```text
0 < history_min_bytes <= history_max_bytes <= context_max_bytes
```

The branches have separate field sets. The resolver supplies MCMTR defaults,
validates the selected branch, and serializes effective configuration as flat JSON
in the existing model `compaction` column. No database schema change is required.
Database model create/update validates the same effective limits.

TADW counts exact rendered UTF-8 history/tooling bytes and its trailing
request-orientation hints. These are not token limits or a bound on the entire
JSON request, system prompt or tool schemas.

## Request selection and replay

The first `gatehouse.agent-round-prepare` transaction records the algorithm and,
for TADW, its limits and effective-template compatibility hash alongside the
admitted thinking-event reference. That workflow owns the full round/tool loop
for either algorithm. It retains the returned choice and follows it rather than
selecting again from today's model setting.

Existing reference-only admissions retain the original MCMTR single-round
execution and checkpoint sequence. A new provider preparation requires the selected
algorithm to match the recorded choice. TADW additionally requires matching limits
and an effective provider template matching the recorded compatibility hash. Drift fails preparation;
it does not switch algorithms or select different history. A previously committed
provider decision replays independently of current preparation configuration.

## TADW context lifecycle

The request's first admitted workflow checkpoints the ordered selected history IDs
once and retains that returned slice throughout its loop. Each thinking round has
an admission and a tooling-boundary decision, including unchanged/empty boundaries.

The loop-local context holds history IDs, references, limits and its boundary.
Rendering reads those IDs and that attempt's boundary from authoritative event
storage. Payloads and file metadata are live for each assembly, while membership,
order and boundary decisions remain fixed. History and tooling transactions close
before HTTP or durable waiting.

The shared provider path assembles a request to estimate its rate-limit debit,
releases that body before any wait, then resolves the selected binding and
assembles the actual request after waiting. Each HTTP retry follows the same
preparation path. Bodies and rendered fragments belong to their assembly/attempt;
DBOS stores initial IDs and small decisions/references, not transcript bodies.

TADW presents one user message containing history XML, retained tooling XML and a
trailing `<hints>` block identifying the agent, message to answer and tooling
start. The hints' exact bytes are reserved before tooling selection and rendering;
they are derived from the owning request, not checkpointed or stored as shared
cache-alignment hints. Native Lisp tools and final-response schemas remain declared normally.
Incoming tool decisions share general validation with MCMTR; only MCMTR applies
its native-tool high-tier buffer rule. TADW enforces its history/tooling budgets
when assembling the next request.

See [request context](tadw-request-context.md),
[tooling policy](tadw-tooling-context.md) and
[provider assembly](tadw-provider-input.md) for component contracts.

## Provider-decision recovery

Transient estimation and provider execution run inside separate steps, on either
side of the existing claim/wait sequence. Their results are readiness/token counts
and an outcome reference, not request/response bodies. DBOS replay returns these results
without rendering or sending again.

Before execution, a step reserves the thinking outcome's event reference. Provider
execution atomically stores that outcome and all associated application events.
If application persistence commits before DBOS checkpoints the execute step, a
repeated callback finds the reserved outcome and returns it before preparation or
HTTP. This also applies to retry outcomes. Settlement and retry sleeps remain
ordinary declared workflow steps.

HTTP can repeat after interruption before application persistence commits. A
committed application decision is recovered without sending again. This is not
an exactly-once network protocol. No database transaction spans HTTP.

## Validation and remaining work

Tests exercise configuration unions and effective serialization, database model
creation/update and reconciliation, Chat Completions, Responses and Codex HTTP,
multiple tool rounds, rollover, HTTP retries, rate-limit waiting, cancellation,
admission/revocation and subprocess failover. Recovery tests cover interrupted
preparation, uncommitted decisions, committed decisions, algorithm changes and
legacy admission results. Durable-state checks verify one initial history list,
one boundary per thinking attempt and transcript-free workflow inputs/results.
Existing MCMTR, database, migration and benchmark suites remain regression coverage.

Compatible shared hints are read only during fresh initial selection. Successful
fresh provider turns publish or refresh their used history base on a best-effort
basis after their application decisions commit. Recovery does not backfill a hint
lost in that gap, and completed-attempt replay does not touch hint recency. See
[runtime publication](context-cache-hints.md#runtime-publication). Durable
visibility snapshots, provider cache controls, execution-sibling traversal and
ignored-file-field hydration retain their existing limitations.
