# Shared context-cache alignment hints

SQLite and PostgreSQL V071 add `gatehouse_context_cache_hints`, a small shared application
store for **alignment hints**, not request recovery state. The TADW path in the
[normal reply runtime](context-algorithms.md) reads compatible hints during fresh
history selection and publishes/refreshes them after successful provider use.
Both database backends use the same storage API and hint semantics. Provider cache
controls remain separate work.

## Scope and schema

A scope consists of:

- Workspace and session.
- `cache_scope`: a caller-supplied, non-secret provider/model/cache-namespace key.
  This is deliberately **not** the workspace-agent binding referenced by MCMTR's
  existing `gatehouse_agent_contexts.model` column. Different agents can share
  hints only when their cache scope and complete profile actually agree.
- `algorithm`: a versioned algorithm identifier.
- `profile`: a caller-supplied compatible prompt/rendering fingerprint.

The caller must account for provider routing/cache namespaces, system prompts,
tools, rendering and other relevant differences. The generic store does not
generate or prove the completeness of those keys. Do not put credentials in them.
The [selected-agent TADW preparation adapter](tadw-history-preparation.md)
now derives keys from resolved workspace-agent configuration and the actual static
provider request, without changing the storage API or adding another identity.

Rows add a stable `hint_key`, small JSON-object `hint`, and integer `last_used_at`
(Unix microseconds). The scoped key is the primary key. A separate index is:

```sql
(workspace, session, cache_scope, algorithm, profile,
 last_used_at DESC, hint_key DESC)
```

The table references the session with `ON DELETE CASCADE`. It deliberately has no
foreign key to an event inside algorithm-specific JSON: stale references are
allowed and must not block event maintenance. Existing tables and event indexes
are untouched. MCMTR's checkpoint table is not replaced or reinterpreted.

## Bounded indexed APIs

`database.ContextCacheHintsList(ctx, scope)` reads at most **32** rows through the
scope/recency index, with deterministic key-descending ties. It does not search
JSON, join events, scan other scopes, or sort a session's history. The bound is
an internal constant, not a new user setting.

`database.ContextCacheHintPut(ctx, scope, key, hint)` validates scope/key strings
(nonblank, valid UTF-8, at most 1,024 bytes) and a valid UTF-8 JSON object of at
most **4,096 bytes**. Adapters own semantic validation and versioning; the generic
store does not interpret boundaries or allowlist algorithm-specific fields.
Adapters must publish only small references/decisions, never transcript payloads.

Publication upserts one key and prunes that same scope to 32 rows in a single
transaction. SQLite starts with a write, avoiding a deferred read-to-write
snapshot upgrade. PostgreSQL uses a read-committed transaction and first locks the
session row with `FOR NO KEY UPDATE`; each statement after that lock observes the
preceding publisher's commit. This serializes publishers before upsert/pruning, so concurrent
writers cannot each prune an incomplete view and exceed the scope cap. Other
sessions remain independent. Existing timestamps use SQLite `MAX` or PostgreSQL
`GREATEST` so a stale wall clock cannot move a key's recency backward. This is wall-clock recency with deterministic ties, not a globally
ordered provider-use log. The most recent payload written for a key wins; callers
should assign distinct stable keys to distinct hints. A future-skewed timestamp
can bias eviction, but hints remain optional suggestions, not correctness state.

Reads/writes close resources and release their connection on success/error.
Publication failures roll back both upsert and pruning. Store/connection failures
are returned, not disguised as cache misses. Retention is bounded **per scope**,
not globally across an unlimited number of profiles/model scopes; obsolete-scope
cleanup beyond session deletion is separate work.

## TADW adapter

`agent/history_tadw_cache.go` defines the `tadw-history-v1` algorithm. Its hint is:

```json
{"version":1,"base_event_id":"sev_..."}
```

The key equals the base ID. `loadTADWHistoryCacheHints` decodes at most 32 records,
ignoring unrecognized/malformed semantic payloads, invalid IDs, unsupported
versions and key/base mismatches. A valid but stale/out-of-branch ID is passed to
the existing bounded-history selector, which intersects hints with its candidate
without point-reading hinted events. The selector still picks the **oldest
eligible base**, not the most recently published hint. Misses use normal window
selection. Eviction can reduce alignment opportunities, not break correctness.

`checkpointTADWHistoryWithCacheHints` validates the scope against the anchor and
uses the existing `gatehouse.tadw-initial-history` step:

1. **Fresh callback only:** list/decode hints, closing that query before opening
   the history transaction. Hints and event selection need not share a snapshot;
   hints are merely suggestions and eligibility is decided against the actual
   bounded history candidate.
2. Select/render history with the normal same-transaction event/file reads.
3. Checkpoint **only the ordered selected event IDs**, then reuse the fresh
   worker-local rendered buffer.
4. On replay, skip the entire callback—including hint lookup—and rehydrate the
   recorded IDs. Changed hints, deleted hints or even an unavailable hints table
   cannot cause reselection or affect this recovery path.

The original caller-supplied-hints checkpoint helper remains available and keeps
its original step name/output and recovery behavior. No additional hint lookup
step, repeated history list, profile, cache payload or XML is persisted in DBOS.

`publishTADWHistoryCacheHint` is an **explicit** publication API. The caller must
invoke it only after a provider request actually used that base, not merely after
selection/checkpointing. It accepts only a typed base event reference in the same
session and stores the ID/version, without reading payloads. The reference may
subsequently become stale. Neither history helper automatically publishes or
touches hints. A used prefix is not proof that a provider retained or reused its
KV cache.

## Runtime publication

The fresh `gatehouse.agent-provider-execute` callback publishes after receiving a
successfully parsed final reply or tool-call turn, checking cancellation, and
committing its application decision. It publishes no hint for failed preparation,
estimation, rate-limit waiting, HTTP errors (including retryable 429s), invalid
responses or empty fixed history. A successful retry publishes only after its own
provider use and application-decision commit.

The base is the first ID in that request's selected history. Publication identity
is local to the execution callback: it combines the selected provider/credential
snapshot with the actual sent request's static wire fields, excluding only the
conversation. Effective output-cap changes are included where the protocol sends
them. Model/binding changes during HTTP do not move the hint into a different
namespace. The namespace and profile use the same derivation as selection;
publication does not reread configuration or retain request bodies.

Publication occurs outside the application-decision transaction, after its
successful commit. Artifact or application-persistence failures before that commit
do not publish a hint. A crash after the decision commits but before publication
may lose the optional hint; the committed decision still prevents another provider
send on recovery. Recovery does not backfill the missing hint. Storage errors are
logged and ignored, without failing the provider outcome. Hints remain alignment
suggestions, not recovery checkpoints.

Completed execution steps replay their small result directly. Repeated callbacks
that find their reserved application outcome return before publication. Neither
path inserts a hint nor refreshes `last_used_at`; recovery does not turn an old
provider use into new recency. There is no additional DBOS step, hint receipt or
transcript data in workflow state.

## MCMTR compatibility and boundaries

The generic store can hold versioned MCMTR stream-boundary objects under its own
algorithm/profile namespace. A future MCMTR adapter must validate which stream
references are compatible; it must not blindly import another request's tooling
state. JSON storage alone does not supply MCMTR selection/recovery semantics.
No MCMTR consumer, state migration or behavior change is implemented here.

TADW initial-history IDs and per-thinking tooling boundaries remain request-private
DBOS decisions. The shared table is disposable cross-request hint storage, not a
replacement for either checkpoint. It does not freeze historical visibility or
metadata, derive configuration fingerprints itself, or activate TADW.

## Validation

Database tests cover isolation across every scope dimension, ordering/ties,
deduplication/update, fixed-cap pruning, timestamp non-regression, concurrent
publishers on separate connections, rollback of an upsert after injected pruning
failure, validation/cancellation, missing sessions/tables and connection release.
SQLite `EXPLAIN QUERY PLAN` must use the scoped recency index without a temporary
sort. PostgreSQL server-independent tests exercise parameter binding, session-lock
ordering, `GREATEST`, scoped prune queries, commit/rollback behavior and migration
registration/schema generation. They do not simulate PostgreSQL's actual lock
manager or execute its DDL.

Migration tests upgrade V070 data, run migration twice, preserve existing schema
objects/events, exercise the real API and constraints, verify index shape and
session cascade. Fresh and older-baseline upgrade paths must converge.

Adapter tests cover explicit-only publication, oldest-eligible selection, stale
and malformed hints, algorithm/profile isolation and validation. Real subprocess
crash/recovery tests change hints or drop the table after the ID checkpoint,
remove traversal indexes, and require unchanged recovered history. They retain
the original fresh-buffer metadata trigger and DBOS ID-only/payload-absence
assertions.

Normal-runtime tests cover publication/touch after final and tool replies,
successful retries, no publication before the decision commits or after failed
calls/application persistence, best-effort write failure, empty history, MCMTR
isolation, configuration drift during HTTP, actual wire output caps and consumption
by a later request. Subprocess tests verify unchanged hint recency/write counts
for both completed-step replay and the application-commit/step-checkpoint gap.
A crash during hint publication leaves a committed application outcome, and
recovery neither repeats HTTP nor backfills the missing hint.
