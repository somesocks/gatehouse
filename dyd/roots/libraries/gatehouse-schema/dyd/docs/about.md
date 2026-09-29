# Gatehouse Schemas

The `config` and `models` schema documents share this root and can reuse
definitions where their shapes agree. The supported output/renderer combinations
are `config/go` (JSON codec), `config/json-schema`, `models/go`, and `models/ts`.
Database migrations and persistence code remain hand-written.
