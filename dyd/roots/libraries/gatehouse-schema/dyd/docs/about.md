# Gatehouse Schemas

The `config` and `models` schema documents share this root and can reuse
definitions where their shapes agree. The supported output/renderer combinations
are `config/go-json`, `config/json-schema`, `models/go`, `models/ts`,
`api/ts`, `api/ts-json`, `api/go-json`, and `api/json-schema`. The `go-json` and
`ts-json` renderers produce types and JSON codecs; `go` and `ts` produce types
only.
The `config/json-schema` output includes the canonical `config/document.dhall`
alongside its generated JSON Schema so downstream Dhall-codegen documents can
compose Gatehouse config without duplicating its definitions.
Database migrations and persistence code remain hand-written.

The API document describes the projected session-event transcript, not stored
event payloads. The transcript redacts input results and tool output, hydrates
message attachments into file summaries, and represents unrecognized event
kinds as `other` with an `original_kind` field. The unprojected event endpoint
continues to expose the original kind.
