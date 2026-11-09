# TADW SQLite history cursor

`database/session_tadw.go` adds a standalone, lazy history traversal component.
It is **not wired into agent execution**. Existing MCMTR retrieval and rendering
remain unchanged. The byte-window policy is described in
[tadw-history-window.md](tadw-history-window.md).

## API and transaction ownership

`Store.SessionTADWHistoryCursor(ctx, tx, anchor)` returns `(error, cursor)`.
The anchor is the event triggering execution, not the `agent.request` itself.
Only SQLite stores are supported; PostgreSQL is explicitly rejected.

The caller supplies a live `*sql.Tx` from the same store. Construction performs
no query. Use `Next()`, `Event()`, and `Err()` like a database rows cursor, and
always defer `Close()` when stopping early. Exhaustion, cancellation, and errors
close cursor resources automatically. Closing the cursor never commits or rolls
back the caller's transaction.

Each value is a `TADWHistoryEvent`:

- `Event`: one decoded `model.SessionEvent`, with its actual parent, authors,
  payload and metrics. An outcome is its own event, not a nullable request field.
- `OwnerID`: diagnostic selection provenance. Consumers do not need to regroup
  events by this ID.

All selection, selected-child queries, and payload reads use the supplied
transaction. Further file-metadata hydration must also use that transaction,
not acquire another connection from the store's single-connection SQLite pool.
The cursor does not hydrate attachments/results or read file contents.

**This is a read-transaction snapshot, not a durable request-creation snapshot.**
Visibility begins with the transaction's first read, which may precede this
cursor or occur on its first `Next`. Reopening a transaction later can observe
new eligible events or outcomes. Request-boundary reconstruction and durable
initial-window ID checkpointing remain future integration work.

## Selection and order

- Visit the anchor and successive parents as branch roots.
- At each level, exhaust eligible siblings newest-first, then ascend. Eligibility
  is `created_at <= branch_root.created_at`; equal-time siblings with higher IDs
  remain eligible. The full thread order is `(created_at, id)`.
- For each non-root owner other than `agent.request`, select its oldest three
  direct children by `(created_at ASC, id ASC)`. Branch roots skip these previews.
- Select `agent.success`/`agent.failure` children of selected requests separately;
  these do not consume preview slots. Pending requests remain real nodes without
  placeholder outcomes. Multiple terminal outcomes are an integrity error.
- An outcome already on the anchor ancestry retains its own owner block and is
  not emitted twice when its ancestor request is expanded.

The complete event stream is **reverse prompt order**: visit selected children
newest-first and descendants-before-parent. Child selection is separate from the
generic selected-tree walker: future selected tool/result subtrees need not
change the output representation. No such additional child-selection policy is
activated.

Reverse the entire accepted event list **once** to obtain final prompt order:
older history first, with parent-first DFS and selected children ordered by
`(created_at ASC, id ASC)`. No regrouping, block-wise reversal or global timestamp
sort is needed. Request outcomes stay next to their request in final prompt
order; their completion timestamps do not reorder requests.

A byte cutoff may stop inside a selected subtree. In that case the accepted
history is still a suffix of the full prompt-order stream. A selected descendant
may retain a parent reference whose event lies outside that suffix. Do not force
that parent into the window or skip the crossing event to fill spare space.

## Bounded work and indexes

The seed query is a recursive successor cursor with split timestamp/ID seeks,
not a complete history query followed by sorting and truncation. Explicit
`CROSS JOIN` fences keep the current recursive row before node/parent ID lookups;
otherwise SQLite can scan a whole scope through the thread index before testing
those IDs. This uses existing ID indexes, not an extra covering index. Four-owner
frontiers bound request-child selection to twelve keys. A valid frontier has at
most 28 selected metadata nodes, plus one owner lookahead. `Next` fetches only
its own event payload; there is no payload prefetch. ID guards grow with the
consumed prefix and encountered ancestry, not the whole session.

SQLite migration V070 adds exactly two indexes:

- `gatehouse_session_events_thread_order`: parent and thread-order seeks.
- `gatehouse_session_events_agent_terminals`: a covering partial index restricted
  to agent outcomes, for request-child lookups.

Outcome queries explicitly require the partial index. Without that requirement,
SQLite can choose an ordered thread scan and filter execution children despite
`LIMIT 2`. The limit detects duplicate outcomes; it is not a substitute for an
appropriate access path. The migration must be applied before using this API.

There is **no additional ID-metadata covering index or payload-size index**.
Existing ID indexes handle metadata and payload point lookups. Inline records
are expected to stay small, with large content represented by file references.

No byte quota is implemented in this database cursor. The next adapter must
hydrate file metadata, render exact UTF-8 bytes, stop consumption at the history
maximum, establish final prompt order, and call the pure window policy. It must
not materialize the whole session first.

## Validation

Tests cover scope collisions, missing anchors, parentless siblings, timestamp
and ID ties, oldest-three membership, separate outcomes and pending requests,
ancestral outcomes, generic selected tools/results, invalid data, cancellation,
resource/transaction ownership, read-transaction consistency, every early-stop
prefix, whole-list reversal into final prompt order, query-plan shape (including
actual node/parent ID seeks), and deterministic randomized parent-map oracles.

Small-record fixtures with 10,000 tied siblings, a 10,000-node ancestry, and
10,000 execution children verify bounded logical work on early close, including
lookup-candidate probes rather than only emitted-node counts. A negative
control removes the partial index and its enforcement to demonstrate the
otherwise hidden execution-child scan. These counters are not physical I/O or
end-to-end throughput measurements. A full-schema migration test verifies both
index definitions, repeat migration safety, preservation of existing events,
and cursor operation against the migrated production schema.
