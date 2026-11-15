# TADW tooling windows and context assembly

These components assemble TADW context in the normal reply/round runtime.
The model selects the [context algorithm](context-algorithms.md); MCMTR remains
the default. Window selection changes prompt membership, not source events.

## Fixed history and growing tooling

The assembler accepts `tadwInitialHistory` as working data for one assembly call.
The history anchor is the triggering event, **not** the new `agent.request`.
Tooling belongs to that new request and is bounded by its current thinking event.

The assembler validates the supplied history's fragment IDs/order, uniqueness,
base and exact byte accounting. It does not traverse, reselect, hydrate or render
history again. Configuration must satisfy the existing three-limit invariant:

```text
0 < history_min_bytes <= history_max_bytes <= context_max_bytes
B = context_max_bytes - actual_history_bytes
```

Stored window accounting must agree. These checks are not a complete
configuration/rendering-profile fingerprint; the caller owns that consistency.

The prompt contains fixed history, then the owning `agent.request`, then retained
tool events in ascending `(created_at, id)` order. The request is pinned first,
even if inconsistent stored timestamps place a tool before it in raw query order.
Actual parent IDs are preserved when parents lie outside the window. There is no
extra envelope, sorting of request/result blocks, or splitting of XML fragments.
History slice elements are copied for ownership; immutable XML strings are reused.
The [provider-input adapter](tadw-provider-input.md) places this
assembled transcript in one user message without additional XML or native tool
conversation blocks.

## High-tide policy: drop the old prefix, no lower tier

`loadTADWToolingWindow(..., from)` is fresh selection. `from` is the prior window's
inclusive first retained **tool event** ID; empty means nothing has been dropped.
The owning request is always retained independently of this boundary.

1. Read/render the prior retained suffix plus new eligible tooling. If it fits
   within `B`, keep `from` unchanged. Exact fit does not trigger rollover.
2. Only on a byte overflow, reopen a newest-first cursor **in the same read
   transaction**. Do not try to truncate the forward collector's accepted prefix:
   it stopped before reaching the newest events.
3. Keep the owning request and the newest tool event, then add whole older tool
   events until their combined bytes reach `floor(B / 2)`. This fixed 50% soft
   target includes the request; it is not 50% of the entire context.
4. Include the event crossing the soft target if it fits within `B`. If it would
   exceed the hard maximum, stop before it, even if that leaves less than half.
   Never skip it to search older events for a better fit. Event granularity can
   also leave more than half, or even the entire hard budget, retained.
5. Reverse the accepted tool suffix once and prepend the pinned request. Advance
   `from` to the oldest retained tool event. Subsequent growth starts there;
   shrinking data or spare space does not refill previously dropped tooling.

The pure `selectTADWToolingRollover` policy takes rendered fragments through a
newest-first callback. It performs no DB/DBOS calls. Subtract-before-add comparisons
are safe with `INT64_MAX` limits. `tadwToolingWindow` holds the boundary and rendered
transcript in worker memory, not durable serialized context.

The owning request and newest tool event must fit together. Otherwise return
`*tadwToolingBudgetError`, not a successful empty/partial window. An oversized
older event may be dropped by rollover if the newest suffix fits. Hydration,
decoding, rendering and source failures are errors, not compaction signals. Only
a budget overflow triggers selection. Selection is at whole-event boundaries,
not paired request/result or batch boundaries; an outcome can retain a parent
reference to a dropped request. There are no stubs, summaries or lower-tier text.

## Strict rendering versus selection

- `loadTADWContext(...)` strictly renders all eligible tooling; it still errors on
  overflow and does not invoke rollover implicitly.
- `loadTADWContextFrom(..., from)` strictly renders a recorded retained window.
  Missing, foreign, omitted-kind or future boundaries fail explicitly. If changed
  data no longer fits, it errors rather than advancing the recorded boundary.
- `loadTADWToolingWindow(..., from)` may advance the prior boundary on fresh
  execution, using the policy above.

All failures return an empty result. The typed budget error reports the hard
budget, bytes accepted before crossing, crossing ID and fragment size—not the
size of all remaining tooling. Rendering charges exact UTF-8 XML bytes, including
escaping, file metadata, separators and trailing newlines. File contents are not
loaded. Empty tool output is a real self-closing event; cancelled tool failures
still render. A zero tooling budget cannot fit the owning request.

## Request-only database reader

`database.SessionTADWToolingCursor` operates on a caller-owned live transaction.
SQLite and PostgreSQL share this API and recursive subtree-selection semantics.
Normal readers use read-only repeatable-read transactions for stable event/file
metadata across successive reads. PostgreSQL streams IDs through a transaction
cursor named `gatehouse_tadw_tooling`, closing each fetched result before
event/file point reads on the same connection; SQLite uses its existing
streaming rows. PostgreSQL cursor names are connection-local. Close the forward
cursor before opening the reverse rollover cursor on the same transaction.

It validates the persisted request target and the thinking request's kind, agent
author, direct parent, scope and exclusive ID boundary. The recursive walk:

- Traverses only the owning request subtree, before `thinking.id`.
- Does not cross nested `agent.request` nodes, even for the same agent, or
  foreign-agent-authored subtrees. Principal-authored bookkeeping may be crossed.
- Emits only the owning request, `tool.request`, `tool.success` and `tool.failure`.
  Thinking, approval, input, cancel, message, agent-outcome and unknown payloads
  are not emitted/decoded, except the thinking validation read.

Optional `TADWToolingReadOptions{FromEventID, Reverse}` controls suffix selection
and direction. A retained boundary must itself be an eligible tool event in the
owned subtree before the thinking boundary. Its `(created_at, id)` position is
resolved using metadata, without decoding its payload. The cutoff filters emitted
IDs, **not recursion**: a retained result can have an older, dropped ancestor.
The request bypasses the cutoff. Default cursor order is ascending; reverse order
is descending. `RequestEvent()` exposes the already-validated root for pinning.

Payloads are point-read only on `Next`, apart from request/thinking validation;
the validated root is reused. Payloads before an already-recorded retained
boundary are not decoded or hydrated. The forward fit check may read events that
a new rollover subsequently drops, and reverse selection may render one older
hard-limit-crossing event before rejecting it.
However, SQLite may walk/sort current-request metadata before producing IDs.
Boundary validation also walks owned metadata, and rollover opens another cursor.
This is not the bounded-frontier history algorithm or an accepted-byte bound on
metadata work. Scoped child-index and ID seeks are used; no new indexes or payload
weight indexes are introduced.

Event reads and canonical file-reference hydration use the same transaction.
Hydration reads only file ID/name/media type/int64 size metadata, not contents,
providers or credentials. Disabled historical files remain renderable. Cursors
close on exhaustion/error; early consumers must close them before ending their
transaction. Closing a cursor never ends the caller's transaction. Assemblers
release their owned read transaction and the store's sole connection on every
return path, including cancellation.

## DBOS: one small boundary decision per thinking attempt

`checkpointTADWTooling(..., from)` belongs once per thinking attempt at a stable
workflow position. The caller carries the returned boundary to the next attempt.

- The named DBOS step `gatehouse.tadw-tooling-window` returns **only the boundary
  string**, including an empty/unchanged decision. No rendered context, tooling
  payloads or repeated initial-history ID lists are checkpointed.
- Fresh execution selects a window and reuses its rendered buffer after the step
  succeeds. It does not rehydrate after committing the boundary.
- Replay gets that attempt's recorded boundary and strictly renders from it.
  It does not rerun selection or substitute a later attempt's boundary.
- Record a decision on every attempt, even if unchanged. Conditionally adding a
  durable step after an uncheckpointed overflow could change step order on replay.

DBOS persists this request-private state in its own database. No separate
application table is needed. The initial-history IDs are still checkpointed once
per request. Rendered history/context belongs to one selection or assembly call.
The [request workflow](tadw-request-context.md) retains its ID slice and assigns
each returned boundary to its loop-local state. The runtime boundary step returns
only that string; fresh provider steps render the recorded window on demand.

## Durability and remaining boundaries

Read snapshots are local to each selection/replay transaction, not durable
insertion-visibility snapshots. The exclusive thinking-ID cutoff does not exclude
late insertions of older IDs. Existing payload/metadata edits can change rebuilt
XML, byte counts, or even a boundary's position. History membership/order are
fixed, not historical XML: each actual provider attempt renders the recorded IDs
from authoritative storage. IDs/boundaries do not guarantee byte-identical
reconstruction or KV reuse.
A replay window that no longer fits fails explicitly instead of reselecting.

A crash before a DBOS decision commits may repeat fresh selection. Preserving
historical visibility/metadata, shared-hint publication and provider cache controls
remain separate concerns. The previously deferred
history-traversal and ignored-file-field hydration issues are unchanged.

## Validation

Tests cover 50% rounding above/below the target, request cost, newest-event
protection, exact-fit growth, monotonic boundary advancement, no refill, dropped
payload exclusion, scope/ownership/kind validation, ties, reverse iteration,
omitted ancestors, frozen history, empty output/history, `INT64_MAX`, cancellation,
connection release and exact UTF-8 accounting. The pure policy is checked against
an independent suffix oracle over 1,000 randomized cases.

Real subprocess crash/recovery tests cross two rollovers and replay earlier
attempts before completing a later one. They verify per-attempt boundaries,
unchanged fresh/recovered XML when data is unchanged, dropped corrupt payloads
remaining unread, explicit failure for missing boundaries, and failure rather
than reselection after a retained window grows. A metadata-change trigger at
checkpoint insertion proves fresh execution reuses its existing buffer. Durable
records are inspected for one initial-history ID list, boundary strings only,
and boolean-only simulated thinking results; a scan of DBOS tables rejects
history/tooling payload and XML markers.
