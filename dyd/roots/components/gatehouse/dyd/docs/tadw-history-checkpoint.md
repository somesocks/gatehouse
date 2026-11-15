# TADW request-history checkpoint and recovery

`agent/history_tadw_checkpoint.go` checkpoints initial history in the TADW
history-owner workflow over [the history adapter](tadw-history-adapter.md).
The [normal reply runtime](context-algorithms.md) registers that internal child
when constructing its workflow set. The separate
[shared cache-hint store](context-cache-hints.md) supplies an optional fresh-only
hint source; it is not recovery state. The
[selected-workspace-agent adapter](tadw-history-preparation.md) derives its
compatibility keys without adding another durable checkpoint.

## One selection checkpoint per request

`checkpointTADWHistory(ctx, store, anchor, cachedBaseIDs, config)` is intended to be
called **once in the request workflow**, outside its thinking/tool loop. The
anchor is the triggering event, not the new agent request. The caller owns
admission checks, workflow identity, configuration and conversation/model/profile
scoping of cache hints.

On fresh execution:

1. A DBOS step named `gatehouse.tadw-initial-history` runs `loadTADWHistory` using
   the step's context. This selects and renders history in one read transaction.
2. The step returns **only the ordered selected event IDs**, including an empty
   selection. No event payloads, XML, byte accounting, cache hints, or repeated
   copies of the ID list are checkpointed by this component.
3. After the checkpoint succeeds, the helper returns the already-rendered
   `tadwInitialHistory` from ordinary memory. It does not fetch/render it again.

The rendered result belongs to this selection call. Request state consists of the
recorded IDs; each provider attempt renders those IDs on demand. The selection
checkpoint belongs once at request entry, outside the round loop.

The [tooling assembler](tadw-tooling-context.md) accepts rendered history for one
assembly. The [request workflow](tadw-request-context.md) consumes the ID-only
step result directly and retains it across its round loop. It rehydrates only
inside fresh boundary or provider preparation callbacks.

## Optional shared hints

`checkpointTADWHistoryWithCacheHints` uses the same named step and ID-only output,
but reads shared hints inside its fresh callback before selecting history. The
original explicit-hints API remains available. Neither helper publishes a cache
hint just because it selected a base; publication must follow actual provider
use. See [shared context-cache hints](context-cache-hints.md).

## Recovery uses recorded IDs, not selection

On DBOS replay, the step callback is skipped. Its ordered ID result is passed to
`rehydrateTADWHistory(ctx, store, session, ids, config)`:

- Validate configuration, scope, cancellation, nonempty IDs and uniqueness.
- Point-read each exact ID in its recorded order, scoped to the session and
  workspace. There is no ancestry/sibling traversal, sorting, reversal, cache
  lookup, or history-window reselection. Empty history stays empty.
- Use a single caller-owned read transaction for event and file-metadata reads.
  `SessionEventGetInSQLTransaction` reuses the ordinary scoped event decoder;
  neither it nor file hydration acquires another pooled connection.
- Require every selected event to exist, remain renderable and render
  successfully. Missing events/files, malformed payloads or newly omitted kinds
  fail recovery; they are not silently omitted or replaced.
- Recalculate exact rendered UTF-8 history bytes and remaining tooling budget.
  Exceeding `history_max_bytes` is an error, not permission to trim selected IDs.
  Exact fit is allowed and arithmetic subtracts before adding to avoid overflow.

Errors return no partial history, and read transactions release the store's sole
connection. The base is the first recorded ID. `CacheBaseMatched` is initial
selection diagnostics only: it is not persisted and is false on rehydration.
That flag must not be treated as durable evidence of a provider cache hit.

## Durability boundary

The committed ordered IDs fix membership and ordering. Event storage remains
authoritative; XML is rebuilt on demand for each provider attempt. Read snapshots
are local to each load/rehydration transaction. Mutable file metadata can change
the rebuilt fragments, exact byte count and tooling budget. IDs alone do **not**
guarantee byte-identical recovery or provider KV reuse. Rendering-profile and
configuration consistency remain the caller's responsibility.

The event/file read transaction and the subsequent DBOS checkpoint are separate.
A crash before the checkpoint commits can rerun selection; this is not a durable
request-creation visibility snapshot. Freezing historical metadata/visibility
and provider cache controls remain separate concerns.
Shared cache-base hints are available through the preparation adapter,
but are never consulted during recovery. The explicitly deferred execution-sibling traversal
and ignored-file-field hydration issues are unchanged.

## Validation

Tests cover recorded non-tree order, arbitrary subsets, file-reference rendering,
precise byte accounting, metadata changes, exact-fit and over-budget recovery,
missing/scoped events, omitted kinds, malformed payloads, missing files,
validation, cancellation, empty selections and connection release.

A subprocess test crashes a real worker after the history checkpoint and two
simulated thinking checkpoints. A second process recovers the PENDING workflow
with changed hints, both traversal indexes removed and an unrelated corrupt new
event. It verifies unchanged IDs/order/fragments, empty-history recovery, and an
explicit failure if a selected event was deleted. A trigger changes file metadata
at checkpoint insertion to verify that the fresh result reflects the selection's
read snapshot; that metadata is restored before restart. DBOS records contain
exactly one ID-list selection checkpoint and boolean-only thinking results; a
scan of all DBOS tables rejects
payload, rendered-history and tooling markers. This tests the component contract,
with normal-runtime lifecycle coverage described in [context algorithms](context-algorithms.md).
