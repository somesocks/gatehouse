# TADW request context

`assets/agent/context_tadw_request.go` manages the context owned by a request's
first admitted workflow. The same workflow runs subsequent thinking rounds and
awaits reference-valued tool children. This ownership is shared by newly admitted
MCMTR and TADW requests; only their context policies differ.

## Durable and ordinary state

The first admission transaction returns a thinking-event reference and the selected
algorithm. For TADW it also records the three byte limits and a compatibility hash
of the effective selected agent/provider template. Subsequent rounds consume this
returned choice, rather than reselecting from current model settings.

One `gatehouse.tadw-initial-history` step returns the ordered history IDs. The
workflow retains that slice and one current tooling boundary. It does not retain
rendered history, a transcript, provider requests or provider responses.

Every admitted thinking round encounters `gatehouse.tadw-tooling-window`, whose
result is its boundary string, including unchanged or empty boundaries. Fresh
execution rehydrates the fixed history and selects the boundary using exact
rendered bytes. Replay receives the recorded result directly. Assigning that
result to the loop-local boundary reconstructs the same sequence on recovery.

Child tools receive event references. Neither the history list nor a rendered
context is included in their workflow inputs or outputs.

## Provider attempts

Each provider attempt has these step boundaries:

1. `gatehouse.agent-provider-estimate` prepares a transient request and returns
   only readiness and its estimated token count. Failed preparation cannot lead
   to an unestimated provider call, even if it becomes available before execution.
2. The existing rate-limit claim, optional delay and durable sleep reserve capacity.
3. `gatehouse.agent-provider-outcome-id` reserves the thinking outcome's event
   reference. Its timestamp identifies the invocation's start, not completion.
4. `gatehouse.agent-provider-execute` prepares the current request, sends it,
   validates the response and atomically persists the complete application
   decision. Its durable result is only the reserved outcome reference.
5. The existing settlement checkpoint reconciles usage. Retry outcomes lead to a
   durable sleep and another attempt, with the same thinking round's boundary.

Request preparation rereads the exact history IDs and reconstructs that round's
recorded tooling window. Authoritative payloads and file metadata are read anew.
The selected binding/template must still match the recorded choice. Context
transactions close before waiting or HTTP; no database transaction spans a
provider call.

Completed estimate and execution steps return their recorded small results without
running preparation or HTTP. If application persistence committed but the execute
step's result was not checkpointed, the execute callback finds its reserved
thinking outcome and returns that reference before preparing a request. All
associated tool requests or final replies were stored in the same application
transaction. Repeating that step cannot publish a second decision.

An HTTP call interrupted before application persistence can repeat. This is not
an exactly-once network protocol. Files published before an interrupted application
transaction retain the existing publication lifecycle.

## Recovery and compatibility

A replay runs the declared steps and consumes their results in order. The request
workflow reconstructs IDs, boundaries, claims and outcomes from those results.
Missing selected events, invalid boundaries or incompatible fresh preparation
fail rather than silently selecting another context.

Reference-only admissions from older executions retain the original single-round
execution and checkpoint sequence. Their parent continues coordinating round
children. New admissions record the context choice and own the full request loop;
this distinction comes from the admission step's ordinary return value.

Shared alignment hints are consulted only in fresh initial selection. Successful
fresh provider execution publishes its used history base after its application
decision commits, using the selected snapshot and actual static wire fields, with
best-effort storage. A crash in this gap may lose the hint. Completed-step replay
and application-outcome recovery neither backfill hints nor refresh recency. See
[runtime publication](context-cache-hints.md#runtime-publication).
