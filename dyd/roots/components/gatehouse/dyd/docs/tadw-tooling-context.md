# TADW tooling context assembly

`agent/tooling_tadw.go` assembles fixed initial history followed by the current
agent request's tooling. This is a **standalone, inactive component**. No reply,
round or provider execution path calls it; MCMTR is unchanged. There are no new
configuration defaults, indexes, DBOS checkpoints, cache tables, provider cache
controls or tooling-compaction rules.

## Assembly contract

`loadTADWContext(ctx, store, history, request, agent, thinking, config)` accepts:

- Worker-local `tadwInitialHistory` from the history adapter/checkpoint helper.
- The current `agent.request` reference, its selected agent and the current
  `thinking.request` reference. The agent request is **not** the history anchor;
  the triggering event supplied to initial-history selection remains the anchor.
- The same validated three-limit TADW configuration used for initial history.

The assembler validates the frozen history's fragment IDs/order, uniqueness,
base and exact byte accounting, without traversing, reselecting, hydrating or
rendering it again. It recomputes the budget from the actual fragment bytes:

```text
tooling_budget_bytes = context_max_bytes - actual_history_bytes
```

The stored window accounting must agree. Invalid limits, history that no longer
fits, or inconsistent worker accounting fail rather than silently changing the
history window. This is not a configuration/rendering-profile fingerprint: the
caller still owns consistency of settings that do not change these checks.

On success it returns `tadwTranscript{History, Tooling}`. History is first,
tooling second, with no envelope or extra separators. Slice elements are copied
for ownership, while immutable historical XML strings are reused. No raw events
are retained in the returned context. The assembler does not mutate its inputs.

## Request-only reader

`database.SessionTADWToolingCursor` is a SQLite-only reader on the assembler's
caller-owned read transaction. Unlike the existing MCMTR round query, it does
**not** fetch other requests, historical context or the triggering message.

Before opening its ID stream it point-reads and validates:

- A persisted `agent.request` whose target matches the supplied agent.
- A persisted `thinking.request` with that agent as author and the owning
  request as its direct parent.
- Matching workspace/session scope, valid event IDs and a thinking ID greater
  than the request ID.

Membership is the owning request plus eligible descendants. The recursive query:

- Uses `child.id < thinking.id` as an exclusive attempt boundary, matching the
  existing round-query convention. The current thinking event and later-ID
  tools/results cannot enter the context, even with backdated `created_at`.
- Does not cross nested `agent.request` nodes, including requests targeting the
  same agent. Those executions have their own request ownership.
- Does not cross foreign-agent-authored nodes, matching the existing active
  subtree's author boundary. Principal-authored intermediate bookkeeping can
  remain traversable.
- Emits only the owning `agent.request`, `tool.request`, `tool.success` and
  `tool.failure`. Intermediate thinking, approval, input, cancel, message,
  agent-outcome and unknown nodes have no tooling fragment. Actual parent IDs
  are preserved even when a bookkeeping parent is omitted.

Tooling uses ascending `(created_at, id)` prompt order. Parallel tool requests
and results can interleave chronologically; there is no request/result block
regrouping or history-style reversal. Tools with empty output remain real,
self-closing events; cancelled tool failures still render.

The recursive walk uses scoped child-index seeks and event-ID point lookups.
SQLite may collect/sort **current-request metadata** before returning IDs; this
is not the bounded-frontier history algorithm. Payloads are point-read only when
`Next` requests an emitted event, except for the request/thinking validation
reads. The validated root is reused rather than fetched twice. Omitted execution
bookkeeping and unrelated/future payloads are not decoded. Early budget failure
can avoid later payload reads, but does not bound all metadata work by accepted
bytes. No extra indexes or payload weights are introduced.

## Hydration and exact budgets

Tooling event reads and canonical file-reference hydration use the same read
transaction. Hydration requests metadata only, not file contents or providers,
and never acquires a second pooled connection. Disabled historical files remain
renderable. The returned tooling fragments use the existing TADW renderer,
including escaping, metadata and each event's trailing newline.

`collectTADWTooling` accepts prompt-order events and whole fragments only:

- Every tooling ID must be unique and disjoint from the fixed history IDs.
- The first event exceeding the remaining budget produces a
  `*tadwToolingBudgetError`, discoverable through `errors.As`.
- The error contains the budget, bytes accepted before crossing, crossing event
  ID and that fragment's bytes. It does not claim to know all remaining tooling's
  size and does not return a usable partial transcript.
- There is no splitting, skipping a crossing event, dropping older tooling,
  shrinking history, or automatic compaction.
- Exact fit succeeds **only if no further renderable tooling exists**. Unlike
  bounded history selection, merely filling the budget cannot justify stopping
  and omitting remaining execution. Detecting overflow may render one crossing
  event. A zero budget cannot accommodate the nonempty owning request fragment.
- Subtract-before-add comparisons remain safe with `INT64_MAX` limits.

Cancellation, invalid ownership, source/decoding/hydration/rendering failures and
budget errors all return an empty result. Cursors close on exhaustion/error;
early consumers must close explicitly. Closing a cursor never ends its caller's
transaction. The assembler rolls back its own read transaction and releases the
store's sole connection on every return path.

## Worker memory and durable boundaries

Call the history checkpoint helper once per request workflow. Keep that result
in ordinary worker memory, then call this assembler for each thinking attempt.
Only tooling is rebuilt; the historical fragments stay fixed. Do not pass the
transcript/history through durable thinking inputs or results. This component
creates no new DBOS operation or repeated history-ID checkpoint.

This is not yet wired into the production reference-valued child-round
workflows, nor does it provide cross-worker memory sharing. Tooling is rebuilt
from event storage per call; it is not an incremental rendered-tooling cache.
Mutable historical file metadata stays frozen in the worker's initial-history
buffer, while tooling file references reflect their own current read snapshot.
Consequently tooling XML/weights can change when metadata changes; growing event
membership alone is not a guarantee of byte-stable prefixes or provider KV reuse.

The exclusive ID boundary is not a durable insertion-visibility snapshot. Late
insertion of an older-ID record, edits to existing payloads and metadata changes
are not reconstructed as of thinking creation. Those visibility/rendering-profile
concerns, activation, scoped cache-base persistence, provider controls and tooling
compaction remain separate increments. The previously deferred history traversal
and ignored-file-field hydration issues are unchanged.

## Validation

Tests cover exact/crossing/zero budgets, UTF-8 and escaping, empty output/history,
`INT64_MAX`, skipped kinds, overlap/duplicate IDs and failures without partial
contexts. SQLite tests check same-time ordering, request and scope isolation,
nested same-agent requests, foreign-agent subtrees, omitted intermediate parents,
exclusive thinking boundaries, snapshot reads, lazy payload decoding, cancellation
and cursor/connection release. Concurrent file renaming verifies metadata and
events share the tooling transaction, including exact sizes above 2^53.

Assembly tests reuse frozen history across three attempt boundaries, verify
chronological tooling growth and repeat an earlier attempt without later events.
Changing file metadata does not rerender fixed history. A first-crossing test
leaves later corrupt payloads unread. A DBOS test combines the real initial-history
checkpoint with three ordinary-memory tooling assemblies: durable records contain
one ordered history-ID selection and boolean-only simulated thinking results; a
scan of DBOS tables rejects history/tooling payload and XML markers.
