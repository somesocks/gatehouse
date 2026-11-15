# TADW initial-history adapter

`agent/history_tadw.go` connects the SQLite/PostgreSQL cursor, file-metadata projection,
TADW renderer and pure history-window policy. The [normal reply runtime](context-algorithms.md)
uses it when the selected model specifies TADW; MCMTR remains the default.
The [request checkpoint helper](tadw-history-checkpoint.md) wraps this adapter
with a single ordered-ID DBOS checkpoint and replay rehydration.

## Inputs and output

`loadTADWHistory(ctx, store, anchor, cachedBaseIDs, config)` accepts the triggering
event reference, caller-supplied cache hints and the existing three byte limits:

```text
0 < history_min_bytes <= history_max_bytes <= context_max_bytes
```

The caller remains responsible for authorization and scoping hints to the same
conversation/model/rendering profile. A base match is an alignment hint, not a
claim of provider KV reuse. Invalid configuration is rejected before opening a
transaction.

The returned `tadwInitialHistory` contains:

- `Window`: selected IDs, base and exact byte/tooling accounting from the policy.
- `History`: just those selected, already-rendered fragments in final prompt
  order, reusable directly as `tadwTranscript.History` in ordinary worker memory.

The adapter retains no raw event payloads in its result. The candidate fragments
not selected by the policy are released, rather than retained through a subslice
backing array. The adapter itself persists nothing. The request checkpoint helper
persists only ordered IDs, never this rendered buffer.

## Read and cutoff sequence

1. Open a read-only, repeatable-read transaction and its lazy history cursor.
2. Skip kinds omitted by the renderer before attempting file hydration. The
   adapter and renderer share one allowlist, including cancellation-only agent
   failure filtering. Such nodes have no fragment, quota charge or selected ID.
3. Hydrate canonical attachment IDs and output-file references using that same
   transaction, then render the event with the production renderer.
4. Compare the exact UTF-8 fragment size, including XML escaping, metadata and
   its trailing newline, against the remaining history maximum. Accept whole
   events only. Stop at the first crossing event: no splitting, skipping it or
   searching older events to fill spare bytes. An oversized first event yields
   empty history. Subtract-before-add accounting avoids int64 overflow.
5. On exact fit, stop without asking the cursor for another event. Otherwise,
   reaching a cutoff may require hydrating/rendering one rejected candidate.
   The cursor's separately bounded metadata lookahead still applies.
6. Reverse the entire accepted fragment list **once**, then call
   `selectTADWHistoryWindow` with the supplied hints and configuration.
7. Keep only the selected suffix; do not render it a second time. Close cursor
   resources and release the transaction on success, cancellation or failure.

`collectTADWHistory` is the internal streaming collector, separated for focused
boundary/error tests. Errors return no partial history. Both database backends
use the same collector, renderer, exact byte limits and selected-tree ordering.

## Metadata without file contents

`database.SessionEventFilesHydrateInTransaction` takes the already-owned
`*sql.Tx`. Unlike the existing public hydration entrypoints it does not request
another pooled connection or begin another transaction. This matters because
Gatehouse's SQLite pool has one connection, already reserved by the history
transaction.

The helper reads only scoped file ID/name/media-type/size metadata, not content,
provider credentials, object chunks or fingerprints. Stored sizes stay int64,
without conversion through floating point. Canonical references are replaced
only in a transient payload projection; the input is not mutated. Repeated IDs
within one event share a metadata lookup while retaining their original order.

Disabled files retain historical metadata and remain renderable, as with
existing output-file metadata hydration; metadata access does not grant body
read permission. Missing/invalid references or unavailable size/name metadata
are errors, not silently dropped attachments. Non-file events require no file
queries. Existing hydration helpers and their behavior are unchanged.

All event and file reads share a transaction snapshot. PostgreSQL uses repeatable
read so subsequent event/file queries retain the first read's snapshot, like
SQLite's read transaction. This does **not** provide
durable request-creation visibility or freeze mutable file metadata across a
later recovery transaction. Those integration concerns remain deferred.

## Validation

Tests cover whole-event byte boundaries, UTF-8 and XML escaping, exact-fit early
stop, a crossing event without interior skipping, reversal and partial subtrees,
empty history, skipped kinds before hydration, supplied cache hints, candidate
trimming, cancellation and errors without partial results.

SQLite integration fixtures include real traversal with both attachment and
output-file references. They have no file-content tables or storage providers,
use the single-connection store, and verify byte accounting charges only the
rendered references rather than file contents. Corrupt later payloads remain
unread after an exact fit or an oversized first event. Database tests cover
canonical-input ownership, precise integer sizes, disabled files, scope
collisions, missing/invalid references and concurrent file-metadata changes
inside versus outside the held read transaction.
