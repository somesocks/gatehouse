package migrations

func postgresMigrationV028CreateSessionSecrets() VersionedMigration {
	return VersionedMigration{
		Index:       28,
		Description: "create_session_secrets",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_session_secrets (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					id TEXT NOT NULL CHECK (id ~ '^ssc_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
					author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
					description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (octet_length(description) <= 4096),
					ciphertext TEXT NOT NULL,
					enabled BOOLEAN NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					PRIMARY KEY (workspace, session, id),
					FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
				);

				CREATE INDEX gatehouse_session_secrets_by_session_created
				ON gatehouse_session_secrets (workspace, session, created_at DESC, id DESC);
			`),
	}
}
