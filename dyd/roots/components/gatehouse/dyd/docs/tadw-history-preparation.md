# TADW history preparation for a selected workspace agent

`agent/history_tadw_prepare.go` derives compatibility from an already-selected
`database.WorkspaceAgentModel` and offers call-local initial-history preparation.
The normal request workflow uses the same compatibility and selection policies
with its ID-only checkpoint. The workspace agent remains the actor; compatibility
hashes do not introduce an identity or profile resource.

## Existing actor, derived compatibility keys

The workspace-agent reference identifies the configured actor. Its ID can stay
the same when its effective provider, model or prompt changes. The shared hint
store's `cache_scope`/`profile` values are derived compatibility hashes, **not**
additional actors. The combined key must change for relevant configuration
changes; a matching key is not proof of provider KV-cache reuse.

`tadwHistoryCacheScopeForAgent(session, selected, projectChat)` validates that the
selected workspace agent belongs to the session workspace, has typed workspace-
agent/model/provider references and has the required external-provider fields.
Builtin agents have no external cache scope. Protocol/parameter validation is
shared with the provider request preparer; Codex requires its fixed endpoint.
Authorization and selection of an enabled/available agent remain the caller's
responsibility. This helper performs no additional configuration queries,
credential decryption or provider calls.

### Cache scope

`cache_scope` is SHA-256 of structured JSON containing:

- The internal preparation-version salt.
- Workspace-agent reference (including workspace).
- Selected model reference and actual wire model name.
- Provider ID, protocol and exact endpoint.
- The existing opaque `AgentProviderCredential.Fingerprint()` of the encrypted
  credential bundle and its encryption context.

This v1 scope deliberately isolates different workspace agents even if their
other settings agree. Cross-agent sharing remains an optional later refinement.
Provider/model IDs alone are not sufficient: resolved protocol/endpoint/model
values are included, so edits retaining the same IDs separate incompatible keys.

Credential fingerprints are computed without decryption using the repository's
existing opaque-fingerprint implementation. No raw credential, ciphertext,
endpoint, keychain reference, account metadata or prompt enters the stored scope
string. Key/account changes invalidate scopes. Re-encryption or credential refresh
also conservatively invalidates hints, even if the provider account is unchanged;
this trades potential cache misses for simple safe namespace separation. The
fingerprint is not a provider-supplied account/cache identity or an authentication
check, and an unobservable provider-side routing/model change cannot be detected.

### Profile

`profile` is SHA-256 of structured JSON containing:

- `tadw-history-preparation-v1`.
- The existing `tadw-history-v1` selection/hint algorithm identifier.
- `tadw-flat-xml-v1`, an internal transcript-format compatibility salt.
- `tadw-event-user-xml-v3`, the provider packaging salt for one message per
  event and a separate orientation message (see [provider input](tadw-provider-input.md)).
- The **actual static provider wire body**, obtained by preparing without a
  conversation, setting explicit cache options when supported, then calling
  `.body()`. Individual event markers are not part of this template.

The static body contains the same effective system prompt, tool definitions,
response schema, model and protocol settings as provider preparation. For Codex
it uses the final Codex transformation, not the intermediate Responses request:
for example, the public API's output-token field is not sent to Codex and does
not enter its profile. Some non-prefix settings in a wire body are conservatively
included rather than guessing which settings a provider incorporates into caching.

Prompt generation uses `openAISystemPromptFor`: invocation-local replacements
(including an explicitly empty replacement) win over default/project/append
settings. The caller must already have applied any runtime or per-agent override
to the selected model. Changes to settings hidden by a replacement do not change
the profile. Settings interpolated into the actual prompt, such as agent alias
and inline-result limit, do change it when effective.

Raw parameter JSON is not hashed. Its spelling/key order and ignored parameters
do not matter if the prepared request is unchanged. Binding revisions, provider
display labels, MCMTR compaction, turn limits and prelude configuration do not
independently change this static request/profile. A prelude present in historical
messages is transcript data, not part of this static configuration fingerprint.

Both hashes use structured JSON rather than ambiguous delimiter concatenation.
There is no growing transcript, event-ID list, history byte limit or rendered
history in the profile input. The no-conversation template construction must be
preserved; hashing an actual growing request would prevent cross-request reuse.

The transcript-format constant is declared beside the renderer without changing
its output. Bump compatibility versions when serialization/ordering or
TADW-to-provider message packaging changes incompatibly. The separate
[provider-input adapter](tadw-provider-input.md) sends individual event messages
and checks that its selected configuration matches the prepared scope. Earlier
packaging profiles become cold without a schema migration.

## Preparation and DBOS lifecycle

`prepareTADWHistory(ctx, store, anchor, selected, projectChat, limits)`:

1. Derives/validates the keys from the selected workspace agent. The anchor is
   still the triggering event, not the newly created agent request.
2. Calls the existing `checkpointTADWHistoryWithCacheHints` once at request entry.
   Only its fresh callback reads shared hints and selects/renders history.
3. Returns `tadwPreparedHistory{Agent, Scope, History}` in **ordinary worker
   memory**, reusing the fresh rendered buffer after the checkpoint succeeds.

No new durable step is added. The original named history step still persists only
its ordered event-ID list. Recovery derives the keys in memory but skips all hint
lookup and window reselection, rehydrating the recorded IDs even if hints change
or the table is absent. No provider body, prompt, credential fingerprint, keys,
rendered XML or repeated history-ID lists enter DBOS through this adapter.

The prepared object belongs to this preparation call. Request state consists of
the selected IDs. The [request workflow](tadw-request-context.md) consumes the
ID-only step result directly and renders on demand inside fresh provider steps.
Rendered history and provider bodies have attempt-local ownership.

Preparation **never publishes or touches a shared hint**. The normal provider
execution callback derives publication identity from the selected snapshot and
actual sent static wire fields after successful use. Best-effort publication
follows the application-decision commit; a crash in between may lose the hint.
It does not reread model configuration after HTTP. Replay of a completed attempt
does not publish or refresh recency. See [runtime publication](context-cache-hints.md#runtime-publication).

## Limits and recovery consistency

Keys describe effective local configuration, not a durable snapshot or provider
cache receipt. They are not persisted as a replay-time configuration guard. The
caller still owns configuration consistency across worker restarts. A changed
configuration does not authorize history reselection: recovery continues to use
the recorded IDs, subject to the existing renderability and hard-byte checks.

Matching base IDs/configuration hashes do not guarantee byte-identical history:
authoritative event/file metadata can change, and provider caches can expire.
These are alignment hints only. No metadata/versioned-visibility reconstruction,
provider breakpoint or cache control is provided by preparation.

## Validation

Tests cover deterministic keys for Chat Completions, Responses and Codex; no
mutation or raw-configuration leakage; each relevant configuration change with
unchanged IDs; credential/encryption-context changes; actual tool/response schema
changes; invocation-local/empty replacements; project prompts; ignored settings
and parameter spelling; protocol validation; structured encoding and failures.

Real subprocess crash/recovery cases exercise the selected-agent adapter with
changed hints or a removed hint table. They retain the existing fresh-buffer
metadata trigger, no-auto-publication checks, traversal-index negative controls,
one-ID-list checkpoint assertion and DBOS payload-absence scan. Full existing
renderer, MCMTR and provider contracts have separate regression coverage.
