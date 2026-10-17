# Captured benchmark models

The captured-record schema is defined in
`libraries/gatehouse-schema/dyd/assets/benchmark/models.dhall`. The
`output=benchmark-models&renderer=go` variant generates the `benchmarkmodel` Go
package, assembled into both the Gatehouse build and test workspaces.

The record models are an independent contract from `GatehouseBenchmarkConfig`
and the operational `model` package. Configuration-to-snapshot conversion and
native-event capture belong at explicit boundaries.

## Records

| Record | Purpose |
|---|---|
| `Execution` | Invocation identity, configuration identity/version, Gatehouse build, timestamps, execution status/error |
| `Provider` | Execution-scoped provider snapshot with deferred credential source descriptors |
| `Model` | Execution-scoped model snapshot linked to its provider snapshot |
| `SystemPrompt` | Execution-scoped prompt mode and loaded text |
| `Question` | Execution-scoped loaded question and optional output/turn/tool expectations |
| `LispEnvironment` | Execution-scoped project linkage and loaded prelude |
| `Run` | One selected input combination and repetition, with lifecycle and native source references |
| `SessionEvent` | Original event identity, context, authors, payload, metrics, parent, timestamp, and owning run |

All record IDs and ID references are strings, assigned before persistence.
Session-event IDs and source identifiers retain their original native values.
Snapshot aliases are scoped by `execution_id`, so captures from successive
invocations can coexist even when aliases are reused. Each snapshot stores the
loaded input values directly.

Optional properties use pointer types in the generated Go models. Omitted
limits, compaction fields, boolean settings, and text remain distinct from
explicitly supplied values, including empty text and false booleans. No runtime
defaults are applied to snapshot records.

Model parameters and event payloads/metrics are open property maps. Native
payloads and metrics are preserved as captured data rather than tied to a fixed
statistics schema. Run status describes execution, not benchmark quality.

## In-memory relational capture

`benchmark.OpenResults(ctx)` explicitly creates a private in-memory SQLite
database from `benchmark/results.sql`. Its connection pool retains one connection
so all queries see the same database. The caller closes the database when done.
This initializer is separate from operational migrations and package startup.

The capture tables use caller-supplied text IDs. Every input snapshot references
its execution, with alias uniqueness scoped to that execution. Models reference
providers from the same execution; runs reference selected snapshots from their
own execution. Different executions can therefore retain changed definitions
under the same aliases.

Native event identifiers, source identifiers, payloads, timestamps, authors, and
metrics are stored as captured. Run request/terminal links and event parent links
are constrained to the same run. Event/run links are deferred until transaction
commit, allowing an event tree to be inserted before its run or in child-first
order.

JSON objects hold parameters and payloads/metrics; credential-source
arrays contain descriptors. Optional generated fields map to SQL `NULL`, while
empty strings and false booleans are stored as explicit values. The capture
database identifies this schema with SQLite `user_version = 3`. Version 3 adds
question expectations, loaded execution inputs/planned counts, artifact contents,
and SQL measurement/grading views. Version-1 and version-2 files are not compatible
append destinations and are left untouched.

## Record writer

The benchmark package provides typed `InsertExecution`, `InsertProvider`,
`InsertModel`, `InsertSystemPrompt`, `InsertQuestion`, `InsertLispEnvironment`,
`InsertRun`, and `InsertSessionEvent` functions. Each accepts a context, the
caller's `*sql.Tx`, and a generated `benchmarkmodel` record. IDs and references
are supplied by the caller; inserts report duplicate IDs instead of replacing
previous captures.

The caller begins a transaction, inserts the input snapshots or run/event batch,
and commits it once all references are present. On any writer error, the caller
rolls back. Deferred constraints are checked at commit. JSON encoding errors and
SQL errors include the record identity and preserve their underlying error.

`UpdateExecution` changes status, finish time, and error. `UpdateRun` changes
status, start/finish times, error, and native source references, addressing the
run by both its ID and owning execution. Configuration/build identity and input
selections remain those originally inserted. Missing updates return an error
wrapping `sql.ErrNoRows`. These updates can share a transaction with newly
captured events.

## SQLite file output

`benchmark.ExportResults(ctx, capture, path, appendOutput)` writes the complete
captured dataset to a standalone SQLite file. The capture remains caller-owned.
Without append, the destination must be new. With append, a missing destination
is created, or records are inserted into an existing version-3 capture database.
Existing output must provide the capture tables and columns.

The export reads one source transaction and writes one destination transaction.
All captured IDs, optional SQL values, and JSON text are copied directly. Appends
use plain inserts; duplicate identities fail the complete append instead of
replacing records. Existing data survives a failed append. A failed new export
removes the file it created.

The file uses only the analysis schema and can be opened by ordinary SQLite
tools. File output is an explicit operation, separate from capture initialization
and normal Gatehouse serving.

## Preparation

`benchmark.Prepare(config, path, version, fingerprint)` consumes a validated,
file-resolved configuration and creates one `Prepared` execution. It copies the
configuration, assigns capture IDs, preserves declared settings and credential
descriptors, and resolves provider aliases into snapshot IDs. Editing the caller's
configuration afterward does not change the prepared execution.

The execution and five snapshot collections are available as generated
`benchmarkmodel` records. `Prepared.Insert(ctx, tx)` persists them in the caller's
transaction through the record writer. Preparation itself does not open a capture
database or contact a provider.

`Prepared.Runs()` lazily yields `(benchmarkmodel.Run, error)` pairs in test and
selection/repetition order. Each requested record gets a new ID and the selected
snapshot references, starts as `planned`, and omits native identifiers and timing
fields. Callers retain each yielded record for insertion and lifecycle updates;
enumerating the iterator again creates new run IDs. Stopping iteration does not
materialize remaining combinations. `RunCount` retains the validated matrix count.
