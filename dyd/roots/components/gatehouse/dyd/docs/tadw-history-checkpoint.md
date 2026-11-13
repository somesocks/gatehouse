# TADW request-history checkpoint and recovery

`agent/history_tadw_checkpoint.go` adds an **inactive** request-entry helper over
[the history adapter](tadw-history-adapter.md). No existing reply, round, provider
or MCMTR execution path calls it. There are no new configuration defaults,
indexes, cache tables, workflow registrations or tooling-compaction rules.

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

Keep that result in worker memory and reuse its `History` fragments throughout
execution, with growing tooling represented separately in `tadwTranscript`.
Do not pass it as a durable thinking workflow input or return it as a durable
step/workflow result. Calling this helper independently inside every round would
create multiple selection checkpoints and is **not** its intended lifecycle.

This increment does not wire history into the current reference-valued child
round workflows or implement cross-worker memory sharing. That integration must
preserve the single request checkpoint rather than copy its IDs into every
child input/result.

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
authoritative; XML is rebuilt in ordinary memory after a restart. Read snapshots
are local to each load/rehydration transaction. Mutable file metadata can change
the rebuilt fragments, exact byte count and tooling budget. IDs alone do **not**
guarantee byte-identical recovery or provider KV reuse. Rendering-profile and
configuration consistency remain the caller's responsibility.

The event/file read transaction and the subsequent DBOS checkpoint are separate.
A crash before the checkpoint commits can rerun selection; this is not a durable
request-creation visibility snapshot. Freezing historical metadata/visibility,
scoped cache-base persistence, provider cache controls and runtime activation
remain separate increments. The explicitly deferred execution-sibling traversal
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
at checkpoint insertion to prove the fresh caller reuses its existing buffer;
that metadata is restored before restart. Three simulated rounds reuse worker
history while tooling grows. DBOS records contain exactly one ID-list selection
checkpoint and boolean-only thinking results; a scan of all DBOS tables rejects
payload, rendered-history and tooling markers. This tests the component contract,
not activation of the production round/provider lifecycle.
