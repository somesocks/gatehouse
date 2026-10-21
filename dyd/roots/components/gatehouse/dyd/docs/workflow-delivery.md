# Transactional workflow delivery

Message submission is owned by `SessionEventReplyRuntime.CreateMessage`. One
database transaction writes the message, canonical recipient snapshots, automatic
chat grants, activity events, and per-bot `agent.request` events, then enqueues
the reply workflows using DBOS `WithEnqueueTransaction` before committing.

The same transaction enqueues title generation for an unnamed chat receiving a
nonempty message addressed to bots. Stable per-request reply workflow IDs and a
stable per-chat title workflow ID preserve deduplication and queue partitioning.
Human/group-only messages do not enqueue agent work.

Approval responses use `RespondToApproval`: terminal event creation and the DBOS
message to the waiting approval workflow share a transaction via
`WithSendTransaction`, matching input-response delivery. Concurrent responders
are serialized on the approval request, and a partial unique index permits only
one terminal response.

If enqueueing or sending fails, the application writes roll back with the DBOS
writes. After commit, DBOS owns execution and recovery. No application outbox,
acknowledgment, or periodic reconciliation pass is involved. HTTP submission
requires a runtime dispatcher; it does not silently accept undeliverable work.

Database-only event creation methods remain available for data operations and
test fixtures. They do not schedule workflows. Call the runtime submission
methods whenever workflow execution or delivery is required.

Migration 67 drops the legacy reply, title, and approval outbox tables directly.
