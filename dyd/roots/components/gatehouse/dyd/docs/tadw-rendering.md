# TADW transcript rendering

This first TADW increment is a standalone, full-fidelity renderer in
`agent/transcript_tadw.go`. It is not wired into provider requests or selected by
compaction profiles. Existing MCMTR rendering and agent execution are unchanged.
The separate pure history-window budget/base-selection policy is described in
[tadw-history-window.md](tadw-history-window.md). Neither component is wired into
the runtime yet. History retrieval, DBOS checkpoints, cache-table access and
tooling compaction remain separate increments.

## Event fragments

The transcript is an ordered stream of independent XML fragments, **without an
outer envelope**. History fragments precede tooling fragments; the renderer does
not sort, deduplicate or reconstruct the supplied event lists.

| Stored event kind | Element | Type-specific attributes / body |
| --- | --- | --- |
| `message.text` | `message` | Recipient references, direct text, attachment references. |
| `agent.request` | `agent.request` | `agent` identifies the target; empty body. |
| `agent.success` | `agent.success` | Direct reply text and attachment references. |
| `agent.failure` | `agent.failure` | `code`; optional direct diagnostic text. |
| `tool.request` | `tool.request` | `name`; separate `code` and `reason` child elements. |
| `tool.success` | `tool.success` | Direct output text or an `output-file` reference. |
| `tool.failure` | `tool.failure` | `code`; optional diagnostic text, then output or an `output-file` reference. |

Every fragment has `id` and `author`, using stable IDs without display labels or
author-kind prefixes. A real parent produces a `parent` attribute even when that
parent is outside the rendered window. Only genuinely parentless events omit
it. Recipients are `<recipient id="..." />`; legacy agent-target lists are also
accepted. Recipient order is preserved, with duplicates removed by the existing
message-target parser.

Attributes and body text are XML-escaped. Attribute order and field order are
fixed. No `kind`, status, timestamp, provider call ID, metrics, or MCMTR annotation
is copied into the output. Tool code precedes reason. When a tool failure has
both a summary and inline output, a newline separates the two nonempty texts.

Empty replies remain real, self-closing `agent.success` fragments. An empty
inline tool result also remains a self-closing event. There is no implicit
truncation, output cap, omission marker or generated summary.

`approval.*`, `input.*`, `cancel.*`, current `thinking.*` bookkeeping and unknown
kinds are not rendered. Cancellation-only `agent.failure` events are also skipped.
Ordinary messages and completed results remain available even when a later agent
failure reports cancellation. A `tool.failure` reporting cancellation is still a
tool result and remains renderable. There is no generic payload-JSON fallback.

## File references

Attachments and file-backed tool results require the transient metadata produced
by the existing database hydration functions. Canonical stored file references
or bare attachment IDs are not enough; passing unhydrated references is an
explicit error, not a reason to silently drop them.

```xml
<message id="m1" author="user1">Report attached.<attachment id="f1" name="report.pdf" media-type="application/pdf" size-bytes="12345" /></message>
<agent.request id="r1" parent="m1" author="user1" agent="agent1" />
<tool.success id="t2" parent="t1" author="agent1"><output-file id="f2" name="result.txt" media-type="text/plain" size-bytes="6789" /></tool.success>
```

Each file reference includes ID, name, media type and size in bytes. Optional
unknown media types are emitted as `media-type=""`, without inventing a type.
Integer sizes retain their full int64 precision. File content is never opened or
loaded by this renderer. Internal fingerprints and other metadata are excluded.

## API and byte accounting

- `renderTADWEvent` returns a `tadwRenderedEvent`, inclusion flag and error.
- `renderTADWTranscript` returns separate ordered `History` and `Tooling` fragment
  lists; `String()` concatenates them without an envelope.
- A rendered event contains its ID and XML. `Bytes()` is the exact UTF-8 length of
  that XML, including markup, escaped values, metadata and its trailing newline.
- Every included event owns exactly one trailing newline. Skipped events add no
  separators or bytes. Appending events therefore leaves existing fragments
  byte-for-byte unchanged, with cache breakpoints between complete fragments.
- The transcript exposes `HistoryBytes()`, `ToolingBytes()` and total `Bytes()`.
  File sizes are metadata, not amounts charged to the transcript budget.
- Malformed renderable events return errors. A failed list render returns no
  partial transcript. Input events are never mutated.

These rendered values are ephemeral rendering results, not proposed DBOS state.
The later durable initial-window checkpoint will contain only ordered event IDs.

Tests in `agent/transcript_tadw_test.go` cover formats, exclusions, XML escaping,
Unicode byte sizes, file metadata, integer precision, input order, stable append
prefixes, malformed inputs and full-fidelity output above existing MCMTR caps.
