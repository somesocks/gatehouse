package migrations

func sqliteMigrationV028CreateSessionSecrets() VersionedMigration {
	return VersionedMigration{
		Index:       28,
		Description: "create_session_secrets",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_session_secrets (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ssc_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
					description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (length(CAST(description AS BLOB)) <= 4096),
					ciphertext TEXT NOT NULL,
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					PRIMARY KEY (workspace, session, id),
					FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
				) STRICT;

				CREATE INDEX gatehouse_session_secrets_by_session_created
				ON gatehouse_session_secrets (workspace, session, created_at DESC, id DESC);
			`),
	}
}
