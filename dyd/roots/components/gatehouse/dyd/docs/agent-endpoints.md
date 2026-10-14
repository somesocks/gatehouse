# Agent endpoints and live configuration

Agent providers configure `endpoint`, the complete HTTP(S) request URL. The
protocol determines the payload format, not the URL path. For example:

```yaml
agent_providers:
  - alias: openai
    revision: 1
    protocol: openai-responses
    endpoint: https://api.openai.com/v1/responses
    api_key:
      sources: [env:OPENAI_API_KEY]
```

Chat Completions uses a complete endpoint such as
`https://api.openai.com/v1/chat/completions`. Custom proxy paths, encoded path
segments, trailing slashes, and query parameters are passed through unchanged.
Benchmark providers additionally accept `endpoint: env:BENCH_OPENAI_ENDPOINT`.

## Live deployment upgrade

SQLite and PostgreSQL migration 52 renames the operational provider column and
converts existing base URLs to complete endpoints, inserting the former protocol
route before any query or fragment. It preserves provider IDs, revisions,
credentials, and enabled state. It runs once, including for disabled providers;
existing migration and baseline checksums remain valid.

Update deployment configuration and API clients from `base_url` to `endpoint`,
including the full route in each value, when deploying this version. Do not
increment a provider's configuration revision solely for this migration: its
stored URL is converted automatically. Future endpoint changes use normal
provider revision/update semantics.

## Durable rounds

`thinking.request` contains the normal event identity, parent request, author
agent, and timestamp, with only `turn` and optional `previous` in its payload.
`previous` references the preceding committed round outcome. The principal and
original message are read through the parent request, and tool offsets are
derived from committed round history.

Each provider attempt reconstructs conversation input from canonical session
events. Historical requests remain bounded by the owning agent request; its
active subtree is additionally bounded to events before this thinking request.
Compaction watermarks are cached per thinking request, with profile and boundary
checks, so later rounds cannot overwrite the cache for an earlier round.

The enabled agent's current model/provider binding is resolved for each attempt,
including after any rate-limit wait. The system prompt is rendered from current
agent configuration and builtin prompt text. Model parameters, endpoint, and
credentials come from that current binding. The assembled wire request exists
only in memory. Committed tool/final decisions still win during replay.

Rate-limit policy is read inside an always-present claim checkpoint. Completed
claims replay their recorded reservation and wait; new claims read current
policy, using an estimate of the complete transient request including the current
prompt and tools. Claim and settlement checkpoints no-op internally when no
reservation applies. An actual output reservation bounds the output token limit;
input-only reservations do not freeze it. Settlement uses committed usage and
retains the estimate for dimensions with unknown usage. Bucket accounting and
checkpoint results are separate from session event payloads.

SQLite and PostgreSQL migration 53 remove `provider_request` and `round_input`
from existing thinking-request payloads. Event identities, parents, authors,
timestamps, metrics, turn numbers, and predecessor references are preserved.
There is no runtime snapshot decoder or legacy-control flag in the event format.
