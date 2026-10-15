# File-backed agent results

Agent models accept `result_inline_bytes`, a positive byte count with a default
of **4096**. It controls newly produced tool success/failure output and provider
failure diagnostics. Exactly-at-limit results stay inline; larger results are
stored as UTF-8 text session files. The bytes are the rendered Lisp result or
public diagnostic, including existing sensitive-value redaction.

Tool result payloads contain exactly one of `output` and `output_file`. The latter
is a canonical `SessionFileRef`; it does not copy file metadata into event history.
File-backed failures keep their code and a bounded summary. Provider failures use
`thinking.failure.output_file` for full diagnostics. Final agent replies keep their
existing message/attachment contract.

Result files use the highest-priority enabled workspace storage binding. If
there is no enabled binding/provider pair, Gatehouse selects the ordinary enabled
`embedded` storage provider. A write
failure is a failure: it does not select another provider or put the original large
body back into the event. Embedded objects retain their existing 64 MiB size limit.

Publication creates a fresh file/object pair for each uncommitted attempt.
File storage completes before an event can reference it. If the process stops
before the event transaction commits, the uploaded file may remain unreferenced;
a subsequent attempt publishes its own matching content. There is no separate
placement checkpoint. The terminal event and DBOS transaction record commit
together, and DBOS retains only the event reference. Publication errors are
checked inside the transaction callback: fresh execution propagates them, while
replay returns the already-committed event even if a new upload failed. Native
tool side effects retain their existing checkpoints.

Session files record their author. Uploads are principal-authored; agent-created
files, including spilled results, are agent-authored. File browsers and pickers
filter out agent-authored files by default; the file browser offers **Include
agent-authored files**. The files endpoint accepts `author=principal`, `author=agent`,
or `author=all`. Files predating author metadata remain visible by default with
unknown authorship. Explicit file references remain available, including when an
agent-authored file is selected as a message attachment.

Context renders an `<output-file>` notice with its ID, media type, and size. It
does not load full file content into the transcript. Rendering reads retained file
metadata even after a file is removed; explicit content retrieval then reports it
as unavailable. The agent can use
`session/files/info`, bounded `session/files/read`, and paginated
`session/files/search-file`; `(session/files/list)` includes files by all authors.
Event search and event read use the same representation: inline output text for
inline results, or the event payload containing the file reference for spilled
results. Search offsets address that representation. Full result bodies use the
file read/search operations. Returning a large file-read result may itself produce
another result file, so read bounded excerpts or filter/search before returning.

Migration 55 independently upgrades SQLite and PostgreSQL model/file metadata.
Normal storage-provider reconciliation creates the ordinary `embedded` provider
if absent, using the existing revision rules. Existing provider settings and
event bodies are preserved.
Separating result bytes from events permits future collection without rewriting
event history; this change introduces no retention or garbage-collection policy.
