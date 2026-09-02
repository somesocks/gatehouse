package migrations

func postgresMigrationV002CreatePrincipalsAndIdentities() VersionedMigration {
	return VersionedMigration{
		Index:       2,
		Description: "create_principals_and_identities",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY CHECK (id ~ '^prn_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL
			);

			CREATE TABLE gatehouse_identities (
				id TEXT PRIMARY KEY CHECK (id ~ '^idt_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				alias TEXT UNIQUE CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				key TEXT NOT NULL UNIQUE CHECK (position(':' IN key) > 1),
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				verifiers JSONB NOT NULL
					CHECK (jsonb_typeof(verifiers) = 'array')
					CHECK (jsonb_array_length(verifiers) > 0),
				enabled BOOLEAN NOT NULL
			);

			CREATE INDEX gatehouse_identities_by_principal
			ON gatehouse_identities (principal_id);
		`),
	}
}
