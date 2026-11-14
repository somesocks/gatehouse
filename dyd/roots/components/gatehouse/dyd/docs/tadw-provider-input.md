# TADW provider-input assembly

`agent/provider_request_tadw.go` adds an **inactive, worker-local** adapter. It
builds provider bodies but does not send them. No live reply/round/provider path
calls it, and MCMTR is unchanged. No database migration, setting/default, durable
step, publication hook, provider cache control or activation is added.

## One user message, flat semantic XML

`agentProviderRequestPrepareTADW(selected, projectChat, prepared, transcript, limits)`
accepts:

- The resolved selected workspace agent, with invocation-local overrides applied.
- `tadwPreparedHistory` from the selected-agent history preparation adapter.
- The complete retained `tadwTranscript` from request-tooling assembly/checkpointing.
- The existing transcript byte limits.

The conversation contains exactly **one user message** whose content is:

```text
fixed history fragments + retained tooling fragments
```

The adapter concatenates their existing XML strings in order, including existing
trailing newlines. It adds no envelope, labels, separators, summaries or extra
newline. It does not parse XML, reconstruct trees, hydrate files, render events,
roll windows or synthesize assistant/tool messages. Actual parent references,
escaping, file metadata and empty self-closing results remain exactly as rendered.

Native Lisp tools and final-response schemas remain declared in the provider
request, using the existing preparer's definitions. Past tool calls/results are
semantic XML within user content, **not** native `tool_calls`, `tool_call_id` or
Responses `function_call_output` items. This component does not alter parsing or
execution of future provider decisions.

Protocol mapping reuses `agentProviderRequestPrepare`:

- **Chat Completions:** optional existing system message, then the one user message.
- **Responses:** existing instructions, then one role/content input item.
- **Codex:** the same intermediate Responses input, with the existing final Codex
  `.body()` transformation: stream/store/tool-choice settings are preserved and
  the unsupported public-API output-token field is absent from the wire body.

An explicitly empty system-prompt replacement still means no Chat system message
or empty Responses instructions, matching existing provider behavior. Builtin and
unsupported/incomplete provider configurations fail through preparation validation.

## Compatibility describes the actual template

The selected-agent preparation code now exposes a private shared template helper:
`tadwHistoryProviderTemplate` returns both compatibility keys and the **same fresh
static provider template** used to calculate them. The wire adapter fills that
returned template; it does not independently rebuild static prompt/tools/settings.
The original scope-only API delegates to this helper.

Before filling conversation input, the adapter requires:

- `prepared.Agent` equals the selected workspace-agent reference.
- The derived scope, including its session and profile, equals `prepared.Scope`.

Relevant model/provider/prompt/tool/credential changes therefore fail explicitly
rather than quietly using stale publication keys. There is no automatic re-keying,
history reselection or scope mutation. The caller must resolve configuration drift
at the request-lifecycle layer. Treat `selected` as immutable during assembly.

The profile now also includes `tadw-single-user-xml-v1`, the internal conversation-
packaging version. This changes profiles derived by the previous preparation-only
helper, making earlier hint namespaces cold; no table/data migration is required.
Neither growing XML nor event lists enter the fingerprint. Bump the packaging
salt if role, envelope or message-boundary semantics change incompatibly.

This check is not a transport/configuration lock: callers must still use the same
route/credentials/configuration when actually sending the built body. Nor is it a
durable replay-time configuration guard. Provider calls and their integration
with live request lifetime, cancellation and child workflows remain separate work.
The [inactive request handoff](tadw-request-handoff.md) adds a separate small
compatibility guard across child workflows; this pure adapter itself still has
no durable state.

## Frozen history and hard byte checks

The adapter validates frozen history IDs, fragments, base and accounting through
the existing `tadwFixedHistoryBudget`, without rerendering or reselection. The
transcript's history must equal the prepared fragments **including XML strings
and order**, not merely have matching IDs or byte counts.

It also requires nonempty tooling, nonempty/unique IDs and fragments, no overlap
with history, and valid UTF-8 XML strings. UTF-8 validation prevents JSON encoding
from silently replacing invalid bytes. Tooling must include the pinned owning
request according to the assembler contract. The adapter checks nonemptiness but
does not reparse XML to prove event kind, authorship, request ownership or the
thinking cutoff; those remain the database cursor/assembler's responsibility.

Each tooling fragment is charged against `context_max_bytes - fixed_history_bytes`
with subtract-before-add arithmetic. Exact fit is accepted, including safe
`INT64_MAX` configurations. Overflow returns the existing typed
`*tadwToolingBudgetError`; every failure returns an empty provider request, not a
successful partial body. This is strict assembly, not a hidden rollover policy.

Limits count exact rendered transcript UTF-8 bytes. They do **not** count system
prompts, schemas, transport JSON escaping or model tokens. JSON escaping is a
transport layer: decoding the content field yields the exact XML. The adapter
does not double-escape semantic XML.

## Growth, rollover and durability

With fixed history/configuration and an unchanged tooling boundary, new events
append to the same user content. Existing content is a byte prefix of the grown
content. Rollover drops the old tooling prefix but preserves history and the
owning request; the user role/message boundary stays unchanged.

This is an alignment property, not proof of provider KV reuse. Tokenization can
change around an append boundary, providers serialize messages/tools internally,
and caches can expire. Rebuilding an earlier attempt after later rollover must
use that earlier attempt's recorded boundary; given unchanged source metadata and
configuration it yields the same provider body, not the latest suffix.

Only ordinary worker memory holds the returned `agentProviderRequest` and its
content. Do not put it in DBOS workflow/step inputs or outputs. Existing durable
state remains the initial ordered ID list once/request and one small tooling
boundary per thinking attempt. The adapter performs no storage/DBOS operations,
never publishes/touches shared hints and does not read file content or credentials
from a provider/keychain. Credential compatibility hashing uses already-selected
encrypted configuration through the existing opaque fingerprint helper.

The output contains no hint metadata or credentials. Future publication must
follow actual provider use, not assembly, using the matching prepared scope/base.
No publication timing/failure policy or live provider hook is introduced here.

## Validation

Tests inspect marshalled Chat/Responses/Codex bodies, compare all static fields
against the existing preparer, and strip conversation back out to verify the
profile matches the actual static wire fields. They cover exact UTF-8/XML content,
empty output/history, replacements, ownership/no input mutation, configuration
drift, malformed/inconsistent buffers, duplicates/overlap, exact-fit/overflow and
`INT64_MAX` arithmetic.

SQLite integration tests grow tooling to an exact fit, then roll over, preserving
history and testing byte-prefix growth for all three protocols. Reconstruction of
an earlier thinking attempt after later rollover must yield an identical wire
body. A real DBOS workflow builds bodies across three tooling checkpoints but
persists only IDs, boundaries and boolean simulated thinking results. Durable
records are inspected, rendered-payload markers are rejected and no hint table is
created by assembly. Existing crash/recovery, provider, renderer and MCMTR tests
remain in the validation suite.
