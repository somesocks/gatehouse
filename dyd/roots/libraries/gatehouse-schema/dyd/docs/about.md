# Gatehouse Schemas

The `config` and `models` schema documents share this root and can reuse
definitions where their shapes agree. The supported output/renderer combinations
are `config/go` (JSON codec), `config/json-schema`, `models/go`, `models/ts`,
`api/ts`, and `api/json-schema`.
Database migrations and persistence code remain hand-written.

The API document describes the projected session-event transcript, not stored
event payloads. The transcript redacts input results and tool output, hydrates
message attachments into file summaries, and represents unrecognized event
kinds as `other` with an `original_kind` field. The unprojected event endpoint
continues to expose the original kind.
