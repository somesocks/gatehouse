package migrations

func sqliteMigrationV002CreatePrincipalsAndIdentities() VersionedMigration {
	return VersionedMigration{
		Index:       2,
		Description: "create_principals_and_identities",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_principals (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prn_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;

			CREATE TABLE gatehouse_identities (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'idt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT UNIQUE
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				key TEXT NOT NULL UNIQUE CHECK (instr(key, ':') > 1),
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				verifiers TEXT NOT NULL
					CHECK (json_valid(verifiers))
					CHECK (json_type(verifiers) = 'array')
					CHECK (json_array_length(verifiers) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1))
			) STRICT;

			CREATE INDEX gatehouse_identities_by_principal
			ON gatehouse_identities (principal_id);
		`),
	}
}
