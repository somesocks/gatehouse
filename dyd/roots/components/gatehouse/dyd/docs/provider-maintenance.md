# Agent provider maintenance

Provider maintenance is dispatched by one application-wide DBOS schedule,
`gatehouse.agent-provider-maintenance`, using `0 0 * * * *` (hourly, UTC).
The daemon registers the workflows before DBOS launch and applies the schedule
after launch. Reapplying it is idempotent across processes and restarts.

Each tick checkpoints only the enabled provider IDs and opaque fingerprints,
then enqueues a child check per provider. A child's workflow ID is the provider
ID plus scheduled tick. Repeated dispatch of that tick reuses the same job.
The queue has partition concurrency one, keyed by provider ID, across executors.
Providers remain independent; no database transaction spans an upstream request.
Historical missed hours are not backfilled. Interrupted jobs are recovered by
DBOS; the next cron tick examines current provider state.

## Protocol handlers

- **Codex:** skip refresh when known expiry is more than an hour plus a five-minute
  safety buffer away. Otherwise refresh. Unknown expiry refreshes each sweep.
- **OpenAI Responses / Chat Completions:** verify the configured encrypted static
  key can be decrypted and is nonempty. This reports `credentials_available`, not
  upstream reachability or model health. No `/models` or health URL is inferred,
  and no inference request is generated for maintenance.
- **Builtin:** report ready without external work.

The check workflow records only a non-secret result: provider ID, state, and an
allowlisted reason. Credential bundles and ciphertext snapshots
are never workflow inputs, outputs, events, or checkpoints. Unknown protocols
report unsupported rather than guessing a check mechanism.

## Refresh and failure policy

The Codex handler reads/decrypts the current bundle only when refresh is needed.
Before making an upstream request, it selects and unwraps the current `default`
system key and retains that key/reference for the duration of the operation.
An unavailable destination completes as `unavailable` with reason
`encryption_key_unavailable`, without exchanging or changing the credential.
It then uses the refresh-token grant at the fixed OpenAI token endpoint,
validates the account, and encrypts the replacement with the pinned system key,
preserving the credential's associated-data context.
An omitted replacement refresh token preserves the previous token. Missing
expiry stays unknown: neither the previous access-token expiry nor a fabricated
one-hour timestamp is assigned to the new token. `expires_at` is omitted from
credential JSON and public metadata when unknown.

A successful exchange retries saving the **same** encrypted bundle for up to ten
seconds if database persistence fails. It does not repeat the token POST.
The ciphertext and its key reference stay paired across retries. Retiring that
version during an in-flight refresh does not block persistence: the wrapped key
record is retained for historical decryption, and only new operations select a
different enabled version. The operation's local unwrapped-key copy is cleared
on completion.
Transient upstream failures complete as `retry` and are retried on the next
hourly sweep. Known permanent refresh rejections invalidate only the matching
original ciphertext, clear account metadata, and require reconnect. A stale
rejection cannot clear a newer connection.

Maintenance adds no provider fields, credential bookkeeping, or schema migration.
It uses the existing encrypted credential as the conditional-write value:
`WHERE credential = <original ciphertext>`, together with the protocol and
keychain reference. Reconnect and disconnect replace/clear that value;
metadata-only edits preserve it. Runtime refresh/invalidation leaves configuration
revision unchanged. Credential writes and provider activity are atomic.
An already-started refresh may finish
persisting on a disabled provider; it never re-enables it, and disabled providers
are excluded from future sweeps.

The dispatch snapshot carries a SHA-256 fingerprint of the existing ciphertext
and encryption context, computed in memory, never stored on the provider.
Recovery after credential commit but before the DBOS step checkpoint sees a
different fingerprint and skips the old exchange, including for credentials
with unknown expiry. A fresh encryption nonce changes the ciphertext even if
OpenAI returns the same token values. OpenAI rotation and
local persistence are not one atomic transaction: a crash after remote rotation
but before local commit can still require reconnect.

There are no provider next-due fields, custom refresh leases/heartbeats, or rapid
refresh retry loops. DBOS owns scheduling, deduplication, recovery, and concurrency.
`Runtime.CheckNow` is the entry point for a future inference expiry/401 recovery:
it uses the same queue, deduplicates by provider and caller request ID, and can
be awaited for status before rereading authorization from the current provider.

The pinned DBOS 1.4.0 manual `TriggerSchedule` helper performs a nested pool read
inside a transaction, which blocks with the daemon's single-connection SQLite
pool. The hourly cron uses a different enqueue path. Integration tests exercise
native scheduled enqueueing through a one-tick backfill; immediate application
checks use `CheckNow` rather than that SDK helper.
