# TADW provider-input assembly

`agent/provider_request_tadw.go` assembles provider bodies for TADW requests in the
normal reply runtime. Transport is shared with MCMTR, which remains the default.
See [context algorithms](context-algorithms.md) for model configuration and replay.
This adapter builds bodies; it does not send them or publish shared hints.

## One user message per event

`agentProviderRequestPrepareTADW(selected, projectChat, prepared, transcript, limits, hints, cacheCandidates...)`
accepts:

- The resolved selected workspace agent, with invocation-local overrides applied.
- `tadwPreparedHistory` from the selected-agent history preparation adapter.
- The complete retained `tadwTranscript` from request-tooling assembly/checkpointing.
- The existing transcript byte limits.
- Request/message references reconstructed from the actual owning request.
- Optional candidate event IDs for provider cache breakpoints.

The adapter sends each fixed-history event and each retained tooling event as its
own **user message**, in that order. Message content is the event's existing XML
fragment, including its trailing newline. A tool request and its result are
separate messages; neither history nor tooling is combined into a block.

The final user message contains only the orientation block:

```xml
<hints>
<agent alias="luna-high" id="wag_..." />
<request id="sev_AGENT_REQUEST" message="sev_USER_MESSAGE" />
<tooling-start id="sev_AGENT_REQUEST" />
Answer the identified message as this agent. Other conversation events are context, not new requests. Events from tooling-start onward record this request's retained execution; use its tool results as work already performed.
</hints>
```

`request.id` is the internal `agent.request`; `request.message` is its actual
parent message, the task to answer. `tooling-start.id` points to the pinned owning
request, not the first retained tool event or a timestamp. It stays valid across
rollover. Agent identity comes from the selected binding, and attribute values
are XML-escaped. The hints are framing, not a synthetic application event.

Event fragments and their actual relationships are unchanged. The adapter does
not parse XML, reconstruct trees, hydrate files, render events, roll windows or
synthesize assistant/tool messages. It rejects a tooling prefix that does not
begin with the identified owning request.

Native Lisp tools and final-response schemas remain declared in the provider
request, using the existing preparer's definitions. Past tool calls/results are
semantic XML within user content, **not** native `tool_calls`, `tool_call_id` or
Responses `function_call_output` items. This component does not alter parsing or
execution of future provider decisions.

Protocol mapping reuses `agentProviderRequestPrepare`:

- **Chat Completions:** optional existing system message, then event messages and
  the final orientation message.
- **Responses:** existing instructions, then one role/content input item per
  event and one final orientation item.
- **Codex:** the same intermediate Responses input, with the existing final Codex
  `.body()` transformation: stream/store/tool-choice settings are preserved and
  the unsupported public-API output-token field is absent from the wire body.

An explicitly empty system-prompt replacement still means no Chat system message
or empty Responses instructions, matching existing provider behavior. Builtin and
unsupported/incomplete provider configurations fail through preparation validation.

## OpenAI cache breakpoints

TADW uses [explicit breakpoints](https://developers.openai.com/api/docs/guides/prompt-caching#choose-a-caching-mode)
for GPT-5.6 and later on public OpenAI Responses and Chat Completions. The request
sets `prompt_cache_options: {"mode":"explicit"}`. Each user item has one text block
(`input_text` for Responses, `text` for Chat Completions). Marked blocks contain
`prompt_cache_breakpoint: {"mode":"explicit"}`; the final orientation item is
unmarked. Older models, compatible gateways, Codex, and MCMTR keep their existing
formats. No new configuration is required.

The adapter marks up to four distinct event IDs: the current last tooling event
(or owning request before tools), the end of fixed history if present, then
retained candidates in priority order. Runtime candidates prefer the last
endpoint published for the same base and profile, then the triggering message's
actual parent for sibling reuse, then earlier published endpoints. Dropped events
aren't replaced with markers at different positions. The provider verifies the
full prefix at each mark; sharing a branch ID by itself does not establish a hit.
An unmarked footer remains outside the cache writes. No cache TTL or routing key
is overridden.

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

The profile includes `tadw-event-user-xml-v3`, the internal conversation-packaging
version for separate event and orientation messages. Earlier alignment-hint
namespaces become cold; no table/data migration is required. The packaging version
also enters each request's recorded compatibility guard. Finish active TADW
requests before upgrading from the single-message format: an unfinished request
that needs fresh preparation fails the compatibility check rather than silently
changing its context format. Already-committed provider decisions still recover
without preparation or another send.
The static explicit-cache option separates its profile from implicit caching.
Growing XML, event lists and individual marker positions do not enter the
fingerprint.

This check is not a transport/configuration lock: callers must still use the same
route/credentials/configuration when actually sending the built body. Nor is it a
durable replay-time configuration guard. The normal runtime owns provider calls,
request lifetime, cancellation and child workflows.
The [request workflow](tadw-request-context.md) records a small compatibility
guard with its initial algorithm selection; this pure adapter itself has no
durable state.

## Frozen history and hard byte checks

The adapter validates frozen history IDs, fragments, base and accounting through
the existing `tadwFixedHistoryBudget`, without rerendering or reselection. The
transcript's history must equal the prepared fragments **including XML strings
and order**, not merely have matching IDs or byte counts.

It also requires nonempty tooling, nonempty/unique IDs and fragments, no overlap
with history, and valid UTF-8 XML strings. UTF-8 validation prevents JSON encoding
from silently replacing invalid bytes. Tooling must include the pinned owning
request according to the assembler contract. The adapter verifies that the first
tooling fragment ID matches the hints' owning request and that hint references
share the prepared session. It does not reparse XML to prove event kind,
authorship, parent relationships or the thinking cutoff; those remain the
database reader/assembler's responsibility.

Each tooling fragment is charged against
`context_max_bytes - fixed_history_bytes - hints_bytes`
with subtract-before-add arithmetic. Exact fit is accepted, including safe
`INT64_MAX` configurations. Overflow returns the existing typed
`*tadwToolingBudgetError`; every failure returns an empty provider request, not a
successful partial body. This is strict assembly, not a hidden rollover policy.

`context_max_bytes` counts exact rendered event XML plus the trailing hints'
UTF-8 bytes. The runtime reserves the exact hints size before both tooling-window
selection and strict reconstruction; history accounting remains unchanged. An
oversized hints block or owning request fails rather than silently omitting it.
Limits do **not** count system prompts, schemas, transport JSON escaping or model tokens. JSON escaping is a
transport layer: decoding the content field yields the exact XML. The adapter
does not double-escape semantic XML.

## Growth, rollover and durability

With fixed history/configuration and an unchanged tooling boundary, new events
append after the unchanged sequence of **event messages** and before the separate
hints message. No previous event message is rewritten or extended. The earlier
complete input, including its hints message at the old position, is not a prefix
of the new input. Rollover drops the old tooling prefix but preserves history and
the owning request; the hints still identify that request and its parent.

This preserves message boundaries eligible for provider KV reuse. On supported
OpenAI requests the explicit markers select a write boundary before the
hints and make earlier marked boundaries eligible for lookup; a hit still
requires a cached, matching prefix. Rebuilding an earlier attempt after later
rollover uses that attempt's recorded tooling boundary, not the latest suffix.
Shared cache candidates may change the optional markers, not its event content.

The returned `agentProviderRequest` and its content belong to one provider
attempt. Each retry prepares its own request. Durable workflow/step inputs and
outputs carry event references and checkpoint decisions; the provider body is
attempt-local working data. Existing durable
state remains the initial ordered ID list once/request and one small tooling
boundary per thinking attempt. The adapter performs no storage/DBOS operations,
never publishes/touches shared hints and does not read file content or credentials
from a provider/keychain. Credential compatibility hashing uses already-selected
encrypted configuration through the existing opaque fingerprint helper.

Orientation hints are derived anew per preparation and are not checkpointed.
They are separate from database-backed cache-alignment hints; the output contains
no alignment-hint metadata or credentials. The normal execution
callback derives publication identity locally from the selected snapshot and sent
static wire fields, publishing only after successful provider use and the
application-decision commit. That hook is outside this pure adapter. See [runtime publication](context-cache-hints.md#runtime-publication).

## Validation

Tests inspect marshalled Chat/Responses/Codex bodies, compare all static fields
against the existing preparer, and strip conversation back out to verify the
profile matches the actual static wire fields. They cover exact UTF-8/XML content,
empty output/history, replacements, ownership/no input mutation, configuration
drift, malformed/inconsistent buffers, duplicates/overlap, exact-fit/overflow and
`INT64_MAX` arithmetic.

SQLite integration tests grow tooling to an exact fit, then roll over, preserving
history and testing the unchanged event-message prefix before the separate hints
message for all three protocols. A reply workflow test also checks that a tool
request and result append as separate items and that a follow-up reply preserves
the earlier event items. Reconstruction of an earlier thinking attempt after
later rollover must yield an identical wire
body. A real DBOS workflow builds bodies across three tooling checkpoints but
persists only IDs, boundaries and boolean simulated thinking results. Durable
records are inspected, rendered-payload markers are rejected and no hint table is
created by assembly. Existing crash/recovery, provider, renderer and MCMTR tests
remain in the validation suite.
