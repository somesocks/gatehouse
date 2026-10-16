# Codex subscription inference

A connected `openai-codex` provider can back an agent model and workspace agent
using the ordinary agent → model → provider relationship. Set the model name to
a model available to the connected ChatGPT account, then use that workspace
agent in a conversation. Login and [hourly maintenance](provider-maintenance.md)
manage the encrypted OAuth credential independently of inference.

## Transport and agent integration

Gatehouse posts Responses-style requests to the fixed endpoint
`https://chatgpt.com/backend-api/codex/responses`. Authorization uses the bundle's
access token and `ChatGPT-Account-ID`; the refresh token is never sent to this
endpoint. Redirects are rejected. There is one HTTP attempt per inference call,
with a five-minute timeout and cancellation of an active reply's stream when
the reply is cancelled.

Requests use `stream: true`, `store: false`, the configured reasoning effort,
Gatehouse's system instructions, canonical bounded conversation history, the
Lisp function tool, and the existing structured final-reply schema. Session
naming uses the same transport with a plain-text title request and no tools.
The stream is consumed server-side; existing completed-turn events publish the
result. No browser streaming protocol is added.

Only completed output items are considered, and `response.completed` must
arrive before any tool requests or final reply can be committed. Failed,
incomplete, malformed, oversized, and truncated streams fail the turn. Deltas
are not executed. Commentary and reasoning items are not mistaken for final
answers. Output items may arrive through `response.output_item.done` even when
the terminal event omits the output array. Tool call IDs survive subsequent
turns, and tool execution and final decisions use the existing durable event
machinery. Request bodies, OAuth credentials, and raw upstream errors are not
stored in DBOS checkpoints or diagnostics.

The enabled model/provider binding and its encrypted credentials are reread for
each attempt, including after a rate-limit wait. An hourly refresh or reconnect
can therefore supply the next call's access token without changing routing.
An already-started call retains its original credential snapshot.

## Authorization and failure handling

Inference never refreshes tokens, changes provider credentials, or automatically
retries authorization failures. HTTP 401 is surfaced as an authorization error;
the currently stored connection remains intact. Scheduled maintenance handles
refresh separately, and the provider settings offer reconnect/disconnect.

HTTP 429 with `rate_limit_exceeded` and a valid retry header uses the existing
durable rate-limit wait/retry flow. Other HTTP errors and failed streams surface
sanitized errors without copying upstream bodies or transport error strings.

### Failure diagnostics

Each failed transport call emits a structured `Codex inference failed` server
log with provider/model/agent identifiers, duration, failure stage, HTTP status,
received content type, detected body format, and upstream request ID when
present. Recognized upstream error codes and parameter names are included;
unrecognized values are labelled `unrecognized`. Header values are bounded and
checked for reflected credentials. Raw response bodies, free-form upstream error
messages, and authorization headers are never logged.

The session failure includes the stage, HTTP status, content type, body format,
and recognized error details. For example, a successful HTTP response carrying
a JSON model rejection is reported as `stage=response_format`, `HTTP 200`,
`body=json`, `code=model_not_found`, `param=model`, rather than an unexplained
content-type error. The upstream request ID remains in the server log for
correlation.

The body is inspected through a bounded prefix. Actual SSE is parsed even when
the content-type header is missing or unexpected; a valid terminal completion
is still required. JSON/HTML responses are identified separately, and recognized
error details are extracted from bounded JSON bodies and SSE error events.

## Output limits and accounting

The Codex request does not send the public Responses API's `max_output_tokens`.
The configured limit remains the output reservation and a **post-completion
acceptance limit**: if reported output tokens exceed it, Gatehouse fails the turn
without executing tools or publishing the reply. This does not limit generation
or upstream usage. If upstream omits usage, a token-count acceptance limit cannot
be checked; stream/event byte limits and the timeout still apply.

Reported usage, including cached input and reasoning tokens, is attached to the
thinking outcome. Quota settlement debits actual usage even when it exceeds the
reservation or the response is rejected for excess output. Subsequent claims
observe that debit; missing usage retains the reservation rather than inventing
usage or refunding it.

Wire references: the upstream Codex [HTTP Responses transport][transport],
[request types][types], and [SSE handling][sse].

[transport]: https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/endpoint/responses.rs
[types]: https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/common.rs
[sse]: https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/sse/responses.rs
