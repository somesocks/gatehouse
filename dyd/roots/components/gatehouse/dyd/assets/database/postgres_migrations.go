package database

var postgresMigrations = Registry{
	Init: InitMigration{SQL: `
		CREATE TABLE IF NOT EXISTS gatehouse_schema_migrations (
			installed_rank BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			migration_type TEXT NOT NULL
				CHECK (migration_type IN ('versioned', 'repeatable', 'baseline')),
			migration_index BIGINT NOT NULL
				CHECK (migration_index > 0),
			description TEXT NOT NULL
				CHECK (length(description) > 0),
			checksum BYTEA NOT NULL
				CHECK (octet_length(checksum) = 32),
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE UNIQUE INDEX IF NOT EXISTS gatehouse_schema_migrations_once
		ON gatehouse_schema_migrations (migration_type, migration_index)
		WHERE migration_type IN ('versioned', 'baseline');

		CREATE INDEX IF NOT EXISTS gatehouse_schema_migrations_repeatable_latest
		ON gatehouse_schema_migrations (migration_type, migration_index, installed_rank DESC)
		WHERE migration_type = 'repeatable';
	`},
	Versioned: []VersionedMigration{{
		Index:       1,
		Description: "create_workspaces",
		SQL: `
			CREATE TABLE gatehouse_workspaces (
				id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
				workspace_key TEXT NOT NULL UNIQUE
					CHECK (workspace_key ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);
		`,
	}},
	Repeatable: []RepeatableMigration{{
		Index:       1,
		Description: "reconcile_workspaces",
		Template: `
			SELECT 1;
			{{ range .Workspaces }}
			INSERT INTO gatehouse_workspaces (workspace_key, name, enabled)
			VALUES ({{ sqlLiteral .Key }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_key) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`,
	}},
}
