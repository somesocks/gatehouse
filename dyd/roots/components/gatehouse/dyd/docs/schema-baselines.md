# Schema baselines

SQLite and PostgreSQL each register B049 and B068 schema snapshots. A database
without migration history starts from the latest baseline. An existing database
keeps its recorded baseline and applies later versioned migrations normally.
Baseline SQL is checksummed like other migrations: retain previously published
snapshots unchanged when adding a newer one.

V069 upgrades either baseline to the Guest / Member / Admin role vocabulary;
fresh databases currently apply B068 followed by V069. See
[the role-name upgrade notes](role-names-upgrade.md).

## B068 sources

- SQLite: schema definitions from a temporary database migrated through v068
  using the normal SQLite driver. Export `sqlite_schema.sql` in creation order
  for named Gatehouse tables, indexes and triggers. Exclude objects belonging to
  `gatehouse_schema_migrations` and entries whose SQL is null (implicit indexes).
- PostgreSQL: a schema-only dump of the local-dev PostgreSQL 16.15 database,
  verified to have applied all versioned migrations through v068. Include only
  `public`, excluding `gatehouse_schema_migrations`. Retain Gatehouse functions,
  constraints, indexes and triggers; exclude the independent `dbos` schema.

PostgreSQL dump options:

```sh
pg_dump --schema-only --schema=public \
  --exclude-table=public.gatehouse_schema_migrations \
  --no-owner --no-privileges --no-comments --no-security-labels --no-tablespaces
```

Strip pg_dump's top-level session-setting statements, `set_config` preamble,
psql `\restrict`/`\unrestrict` commands and `CREATE SCHEMA public` statement.
The resulting file is SQL executed by the migration runner inside its transaction.
Migration-history objects are created separately by the initialization migration.
Neither baseline contains application rows or configuration reconciliation.

## Verification and historical fixtures

`TestSQLiteBaselineMatchesVersionedSchema` checks both registered baselines
against the full versioned schema, including a follow-up migration and foreign
key checks. `TestSQLiteNewBaselinePreservesExistingHistory` checks upgrading an
existing B049 database and fresh-database selection of B068.

`TestPostgresBaselinesMatchVersionedSchema` checks columns and their order,
types, defaults, constraints, indexes, triggers and functions against the full
versioned schema. It also checks follow-up migrations, existing B049 history and
preservation of existing rows. Set `GATEHOUSE_TEST_BASELINE_POSTGRES_URL` to a
server connection with CREATE DATABASE permission; the test creates and drops
its own uniquely named databases.

Historical upgrade fixtures use `testSchemaAtVersion` so both versioned
migrations and baselines are bounded by the schema version under test. This
prevents a newly registered baseline from silently bypassing an upgrade scenario.

`BenchmarkSQLiteFreshSchema` compares schema-only initialization via B049 plus
upgrades and B068, without repeatable configuration reconciliation.
