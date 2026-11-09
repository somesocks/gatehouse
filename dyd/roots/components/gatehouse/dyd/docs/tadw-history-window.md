# TADW history-window policy

`agent/context_tadw.go` implements the pure policy for choosing a history window
and aligning its first event with a known cross-request cache base. It uses the
exact weights from the standalone TADW renderer, not stored payload JSON sizes.
It is not wired into the runtime or active compaction profiles yet. Retrieval,
cache-table reads/writes, DBOS checkpoints and tooling compaction remain separate.

## Inputs and limits

`selectTADWHistoryWindow` accepts:

- An already bounded list of `tadwRenderedEvent` values in final prompt order:
  old edge first, recent edge last. The policy does not infer order from IDs,
  sort events, or reconstruct the tree.
- Known cache-base event IDs, scoped by the caller to the appropriate
  conversation/model/rendering profile. Stale, duplicate and empty hints are
  harmless; only IDs present in the candidate history can match.
- `tadwWindowConfig`, with the agreed byte parameters:

```text
0 < history_min_bytes <= history_max_bytes <= context_max_bytes
```

`context_max_bytes` is the usable transcript budget, not a token limit. There
are no defaults, ratios, separate refill thresholds, or tooling-size settings
in this component. All limits are signed int64 values, including INT64_MAX.
For rollover headroom use `history_min_bytes < history_max_bytes`; for guaranteed
initial tooling space use `history_max_bytes < context_max_bytes`. Equality is
allowed but removes the corresponding headroom.

The caller must supply **at most history_max_bytes** of rendered candidate
history. Over-budget candidates are rejected rather than silently clipped.
This function must not be fed an eagerly materialized whole session as a
substitute for bounded retrieval. It also rejects duplicate/missing candidate
IDs and empty fragments. Non-renderable events should already have been omitted
by the renderer.

## Selection

The chosen history is always a contiguous suffix of the supplied candidate.
There is no event splitting, body compaction, interior removal or reordering.

1. Compute each potential base's retained suffix size.
2. Choose the **oldest eligible cached base**, retaining the most history, if
   its suffix is at least history_min_bytes. Cache-hint order does not matter.
3. If no base matches, establish a new one by retaining the smallest recent
   suffix that meets history_min_bytes. Include the entire event crossing that
   threshold: this is a lower-bound target, not a cap.
4. With less than history_min_bytes available, retain everything. The first
   event can still match a cache hint; later hints cannot shrink short history.
5. Empty history has no base and leaves the entire context budget to tooling.

This uses **the same minimum** for cache eligibility and rollover. A coarse,
large crossing event may leave little or no rollover headroom; the policy does
not violate the minimum or skip events to compensate.

For example, with history max 8 equal-sized events and history min 4, a cache
miss retains events 4–7 of the candidate 0–7. That base remains usable as events
8–11 arrive: the retained history grows from 4 to 8 events without changing its
start. Once event 12 pushes that base outside the bounded candidate, a miss
establishes a new base at event 9, retaining 9–12. It does not creep forward one
event on each request.

## Output

`tadwHistoryWindow` contains:

- `EventIDs`: an independently owned, ordered ID array, with no XML/payloads.
- `BaseEventID`: its first ID, or empty for empty history.
- `HistoryBytes`: exact retained rendered bytes, including event separators.
- `ToolingBudgetBytes`: context_max_bytes minus actual HistoryBytes.
- `CacheBaseMatched`: whether a known base ID matched. This is an alignment
  hint, **not a claim that the provider reused KV state**; actual reuse requires
  the serialized prefix to match.

The function does not mutate its inputs or persist anything. The future DBOS
initial-window checkpoint can persist the selected ordered IDs, not rendered
chat. Lookup state is bounded by candidate event count; time is linear in the
candidate count plus the number of cache hints. Budget checks subtract before
adding so oversized input cannot overflow byte accounting.

## Tests

`agent/context_tadw_test.go` covers:

- Cache hit/miss, oldest-match preference and stale/duplicate hints.
- Exact minimum, +/-1-byte eligibility, whole-event rounding and short history.
- Empty history, equal limits, zero remaining tooling and int64 limits.
- Renderer-backed Unicode/XML/file-metadata weights, including separators.
- A multi-request append/rollover simulation with stable rendered prefixes.
- Input/result ownership, invalid candidates/configuration, and an independent
  exhaustive-suffix oracle over 1,000 deterministic randomized fixtures.
