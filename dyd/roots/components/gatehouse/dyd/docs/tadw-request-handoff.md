# Inactive TADW request-context handoff

`assets/agent/context_tadw_handoff.go` connects the standalone TADW components
across reference-valued DBOS children. **No production workflow registers or
calls it. Live MCMTR, admission, cancellation, provider transport and publication
are unchanged.** Tests register their own owner, coordinator and round workflows;
their provider is simulated, not an HTTP send.

## Durable shape

A request has a dedicated history-owner child, with a deterministic workflow ID
containing the workspace, session and owning `agent.request` IDs. Its input can be
that request reference. The caller resolves the existing selected workspace agent
and triggering anchor; `Prepare` verifies that the persisted request targets that
agent and is a direct child of that anchor.

The owner calls the existing `prepareTADWHistory` checkpoint once. Its step result
is the ordered history ID list, including an empty selection. Its **workflow
result is only `tadwHistoryHandoff`**:

- owning request and triggering anchor references;
- DBOS workflow ID and step ID of the history decision;
- one SHA-256 compatibility hash over a version salt, selected workspace-agent
  reference, derived history-cache scope/profile, and byte limits.

The hash contains no raw settings, credentials, endpoint or prompt. It is an
internal consistency check, not a new actor/profile resource or provider cache
receipt. It preserves the owner's effective configuration contract after the
owner result is committed. New context/provider preparation rejects changed
settings or limits rather than silently rekeying or reselecting. Recovery before
that result commits still has the existing history-checkpoint semantics; this is
not a durable snapshot of initial event visibility or configuration lookup.

Each round receives that history handoff and its immediate predecessor's
`tadwToolingHandoff`. A tooling handoff contains only request/thinking event
references and the DBOS checkpoint locator. The boundary string itself is stored
once, in that attempt's ordinary `gatehouse.tadw-tooling-window` step, even when
unchanged. It is not copied into subsequent inputs or workflow results.

A coordinator may return/await a small result containing the thinking-outcome
reference and tooling locator. It must never serialize `tadwRoundContext`,
`tadwPreparedHistory`, windows, rendered strings, provider bodies, or credentials.
There is no additional application checkpoint table or schema migration.

## DBOS v1.4.0 access constraints

The public SDK's `GetWorkflowSteps` has two important behaviors:

1. With a workflow context, it runs a `DBOS.getWorkflowSteps` step and persists
   the listing—including loaded outputs. Calling it normally from every round
   would copy the initial ID list into every child's log.
2. `WithStepsOffset` is a **row ordinal**, not a function-ID predicate. There is
   no public single-operation lookup by function ID.

The worker takes the non-workflow DBOS root context at construction, rejects a
workflow-bound context, and uses the public listing API through a derived root
context. Cancellation and deadlines come from the child through standard context
cancellation, but **workflow/step values do not**. Reads therefore create no DBOS
operation and return only worker-local data. No direct query of SDK system-table
schemas is used in production code.

Before publishing a locator, `capture` requests metadata only through the current
step and verifies IDs `0..N` are all committed, with no holes. It checks the
current name and success too. A dense committed prefix makes step ID `N` a stable
row ordinal: no pending lower-ID operation can later insert before it. Sparse
logs or unfinished earlier `dbos.Go` steps fail explicitly. Capturing is
O(prefix steps) metadata work, not a general arbitrary-log lookup. The tested
owner has one step; the round prefix is admission followed by tooling. Reads use
`LIMIT 1 OFFSET N` and verify exact ID, name and success, never guessing another
row, searching for a similarly named step or selecting fresh history.

The reader uses the SDK's default JSON listing-output contract (JSON text for
non-nil outputs, nil for an empty/nil ID-list result). This is not a general
custom-serializer adapter. Missing, failed, mismatched or malformed checkpoints
fail; no fallback to cache hints or traversal occurs. Owner and predecessor DBOS
records must remain retained while dependents need them. Deletion/forking and
layout/version changes require explicit lifecycle handling before activation.

## Worker-local reuse and recovery

One `tadwHandoffWorker` is shared by children on that worker. Fresh owner execution
seeds the memo from the already-rendered selection **after** its checkpoint
succeeds. A cold worker reads the original ID list and uses
`rehydrateTADWHistory` for exact scoped membership/order. It never invokes history
selection or hint lookup. Historical XML/file metadata are live on cold recovery,
not a persisted byte snapshot; missing/nonrenderable/overbudget events fail.

The memo shares immutable XML strings, clones caller-owned slices, and coalesces
concurrent cold reads. A cancelled reader does not poison a healthy waiter or a
later retry. A fixed internal 32-entry LRU bounds retained memo entries on each
worker; eviction only causes exact-ID rehydration. This is not another transcript
retention setting. In-flight callers can keep their entry after eviction, so
active memory also depends on admitted concurrency and configured history size.
`Release` eagerly removes local entries on terminal paths. Remote-worker cleanup
never depends on the parent running on that worker: their idle entries remain
bounded by the LRU. Idle entries have no TTL.

## Child admission, decisions and provider replay

The handoff expects the existing child layout to start with the reference-valued
`gatehouse.agent-round-prepare` checkpoint at step 0. It reads that committed
thinking reference to bind both current and predecessor locators to the right
attempt. It does not rerun admission. Fresh selection also verifies the canonical
thinking-success predecessor chain, request ownership, author and scope, so a
locator from another admitted round cannot reset the boundary.

The surrounding child remains responsible for cancellation checks and its
existing authorization/admission decision. Call `Round` once at a stable position
outside the provider-retry loop. It checkpoints only the tooling boundary. Fresh
selection reuses history and rendered tooling locally; replay reads the current
attempt's own decision, not a mutable latest boundary or a newer attempt's suffix.
A fresh next attempt reads its immediate predecessor's recorded decision and
applies the existing monotonic rollover policy.

**Call the returned context's `Prepare(stepCtx)` inside the provider step
callback.** On first actual execution it assembles the request from fresh buffers,
or strictly reconstructs the recorded boundary if recovering. Sequential retries
reuse successful preparation. It is not a concurrent provider-preparation API.
When DBOS skips an already completed provider step, no XML reconstruction or
provider assembly is required, even if the child crashed before returning its
outcome. Completed child workflows likewise do not reenter preparation. No actual
provider call or hint publication is performed by the helper.

## Evidence and remaining integration

Tests use real child workflows, reference-only workflow results, and subprocess
crashes with pending parent/child states. They cover:

- a second, independent worker memo cold-reading the owner's checkpoint;
- five thinking attempts and at least two tooling rollovers;
- reconstructing an earlier attempt after a later rollover;
- a child crash after the tooling decision but before provider execution;
- a child crash after provider completion but before its workflow result;
- exact history/boundary recovery without traversal indexes or a hint table;
- failures for missing selected history, missing retained boundary and retained
  payload growth, without reselection/roll-forward;
- completed provider work not repeated, including successful recovery with
  subsequently deleted history when no new provider work is needed;
- fresh-buffer trigger probes, sequential provider retry, cancellation/deadlines,
  concurrent memo access, bounded LRU/release, empty histories, configuration drift,
  malformed/missing/failed/wrong-name checkpoints and sparse-step refusal;
- one durable initial ID list, one string decision per attempt, no listing steps,
  and no rendered transcript or private transcript payload in DBOS storage.

The two memo instances in the fresh subprocess prove storage-only handoff without
shared history memory. After the original process exits, the restarted subprocess
uses a different DBOS executor ID and the public `ResumeWorkflows` API to re-enqueue
the pending parent/child. Tests assert that the interrupted child is now owned by
that new executor, with all memo state lost. This tests explicit failover after a
confirmed worker death, not failure detection or simultaneous multi-host queue
deployment. The test admission is a reference-valued
stand-in using the existing checkpoint name, not a second implementation of live
authorization. Existing live lifecycle tests remain separate regression coverage.

Treat the selected configuration as immutable during each preparation and retry
sequence. The compatibility check is not a transport lock: a sender must still use
the same route/credentials/configuration after the body is prepared.

Actual runtime registration/coordinator wiring, transport and route consistency,
post-use hint publication, real multi-worker lifecycle/retention configuration,
and explicit TADW activation remain later increments. No deferred history-query
or ignored-file-field issue is changed here.
