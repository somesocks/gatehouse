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
| `Question` | Execution-scoped loaded question |
| `LispEnvironment` | Execution-scoped project linkage and loaded prelude |
| `Run` | One selected input combination and repetition, with lifecycle and native source references |
| `SessionEvent` | Original event identity, context, authors, payload, metrics, parent, timestamp, and owning run |

All record IDs and ID references are strings, assigned before persistence.
Session-event IDs and source identifiers retain their original native values.
Snapshot aliases are scoped by `execution_id`, so captures from successive
invocations can coexist even when aliases are reused. Each snapshot retains a
content hash and a `sources` map; the `Source` component records a file location
and optional original reference for roles such as `definition`, `text`, or
`prelude`.

Optional properties use pointer types in the generated Go models. Omitted
limits, compaction fields, boolean settings, and text remain distinct from
explicitly supplied values, including empty text and false booleans. No runtime
defaults are applied to snapshot records.

Model parameters and event payloads/metrics are open property maps. Native
payloads and metrics are preserved as captured data rather than tied to a fixed
statistics schema. Run status describes execution, not benchmark quality.
