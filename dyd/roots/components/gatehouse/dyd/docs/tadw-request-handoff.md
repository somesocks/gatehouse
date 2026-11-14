# Inactive TADW request-context handoff

`assets/agent/context_tadw_handoff.go` connects the standalone TADW components
across reference-valued DBOS children. **No production workflow registers or
calls it. Live MCMTR, admission, cancellation, provider transport and publication
are unchanged.** Tests register their own owner, coordinator and round workflows;
their provider is simulated, not an HTTP send.

## Request and attempt state

The request's history is an ordered list of **event IDs**. Its initial membership
and order are checkpointed once. Each thinking attempt records one tooling-boundary
decision. Rendered history, tooling XML and provider bodies are call-local working
data for selection or provider assembly.

`tadwHandoffWorker` supplies the DBOS root context and database store used to read
checkpoints and events. `tadwRoundContext` holds the references, ordered IDs, limits
and recorded boundary used by the provider loop.

## Durable shape

A request has a history-owner child, with a deterministic workflow ID containing
the workspace, session and owning `agent.request` IDs. Its input can be that
request reference. The caller resolves the existing selected workspace agent and
triggering anchor; `Prepare` verifies that the persisted request targets that
agent and is a direct child of that anchor.

The owner calls the existing `prepareTADWHistory` checkpoint once. Selection
renders temporarily for exact byte accounting, then the helper discards those
fragments. The step result is **only the ordered history ID list**, including an
empty selection. The workflow result is **only `tadwHistoryHandoff`**:

- owning request and triggering anchor references;
- DBOS workflow ID and step ID of the history decision;
- one SHA-256 compatibility hash over a version salt, selected workspace-agent
  reference, derived history-cache scope/profile, and byte limits.

The hash contains no raw settings, credentials, endpoint or prompt. It is an
internal consistency check, not a new actor/profile resource or provider cache
receipt. New context/provider preparation rejects changed settings or limits
rather than silently rekeying or reselecting. This is not a durable snapshot of
initial event visibility or configuration lookup.

Each round receives that history handoff and its immediate predecessor's
`tadwToolingHandoff`. A tooling handoff contains only request/thinking event
references and the DBOS checkpoint locator. The boundary string is stored once
in that attempt's `gatehouse.tadw-tooling-window` step, even when unchanged. It is
not copied into subsequent child inputs or workflow results.

Child workflows return small outcome references and tooling locators, not the
loop-local history-ID list, XML, provider bodies or credentials. There is no
additional application checkpoint table or schema migration.

## DBOS v1.4.0 access constraints

The public SDK's `GetWorkflowSteps` has two important behaviors:

1. With a workflow context, it checkpoints the listing, including loaded outputs.
   Calling it normally from each child would duplicate the initial ID list.
2. `WithStepsOffset` is a **row ordinal**, not a function-ID predicate. There is
   no public single-operation lookup by function ID.

The helper takes a non-workflow DBOS root context and rejects a workflow-bound
context. Reads use the public listing API through a derived root context. Child
cancellation/deadlines are propagated, but workflow/step values are not. Reads
therefore create no DBOS operation. Production code does not query SDK table
schemas directly.

Before publishing a locator, `capture` requests metadata only and verifies IDs
`0..N` are all committed, with no holes. It checks the current name and success
too. This makes step ID `N` a stable row ordinal; no pending lower-ID operation
can insert before it. Sparse logs or unfinished earlier `dbos.Go` steps fail
explicitly. Capturing is O(prefix steps) metadata work. The tested owner has one
step; the round prefix is admission followed by tooling. Output reads use
`LIMIT 1 OFFSET N` and verify exact ID, name and success, never guessing another
row or selecting fresh history.

The reader uses the SDK's default JSON listing-output contract, not a general
custom-serializer adapter. Missing, failed, mismatched or malformed checkpoints
fail. Owner and predecessor records must remain retained while dependents need
them. Deletion/forking and layout/version changes require explicit lifecycle
handling before activation.

## On-demand rendering in the loop

`historyIDs` reads the original ordered ID list from its checkpoint. Each caller
owns its returned slice. Event payloads and file metadata are read during rendering.

The handoff expects the existing child layout to start with the reference-valued
`gatehouse.agent-round-prepare` checkpoint at step 0. It reads that committed
thinking reference to bind current and predecessor locators to the right attempt.
It does not rerun admission. Fresh tooling selection verifies the canonical
thinking-success predecessor chain, request ownership, author and scope.

Call `Round` once at a stable position outside the provider-retry loop. The fresh
decision callback renders history/tooling to apply exact byte limits and rollover
policy, then returns **only the boundary**. Replay reads the attempt's own recorded
decision rather than a mutable latest watermark or a newer attempt's suffix.

**Call `worker.Render(stepCtx, round, selected, projectChat)` inside the provider
step callback for every actual attempt, including retries.** It fetches and
renders precisely the recorded history IDs and tooling boundary, and assembles a
provider request owned by that attempt. Each retry assembles its own request from
the same recorded IDs and boundary. DBOS records the decisions and references
described above.

If DBOS skips an already completed provider step, `Render` is not called. No XML
reconstruction or provider assembly is required, even if the child crashed before
returning its outcome. No actual provider call or hint publication is performed
by the helper.

Event storage remains authoritative on **every** render. Membership/order stay
fixed, but payloads and file metadata can change the XML, byte count and remaining
tooling budget between attempts. Missing/nonrenderable/overbudget data fail;
rendering never silently trims, reselects or advances the recorded boundary.
Read snapshots are local, not a durable visibility snapshot or proof of provider
KV reuse. The selected configuration must be immutable during each call, and an
actual sender must use matching route/credentials/configuration.

## Validation and remaining integration

Tests cover real child workflows and explicit failover to another DBOS executor
through the public `ResumeWorkflows` API after a subprocess crash. They check
initial-ID-only durability, five thinking attempts, two tooling rollovers,
earlier-window reconstruction, provider retry, cancellation/deadlines, empty and
malformed checkpoints, configuration drift, and sparse-step refusal.

Rendering tests change history payloads and history/tooling file metadata after
the boundary checkpoint and between calls on the **same round**. Each provider
attempt sees authoritative data while preserving the recorded IDs and boundary.
Additional cases check rendering failures for missing selected history. Crash
cases cover missing history, missing boundaries, retained-data growth, and a
completed provider step that must not render or execute again. Storage assertions
reject rendered transcripts/private payloads and repeated initial ID lists.

This is not failure detection or simultaneous multi-host deployment coverage.
Test admission is a reference-valued stand-in using the existing checkpoint name,
not a second implementation of authorization. Actual runtime algorithm selection,
transport wiring, post-use hint publication and explicit activation remain later
increments. No deferred history-query or ignored-file-field issue is changed.
