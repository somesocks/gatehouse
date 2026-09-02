package migrations

func postgresMigrationV016CreateSessionNotes() VersionedMigration {
	return VersionedMigration{
		Index:       16,
		Description: "create_session_notes",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_notes (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^snt_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (octet_length(title) <= 256),
				description TEXT NOT NULL CHECK (octet_length(description) <= 4096),
				body TEXT NOT NULL CHECK (octet_length(body) <= 1048576),
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			);

				CREATE INDEX gatehouse_session_notes_by_session_created
				ON gatehouse_session_notes (workspace, session, created_at DESC, id DESC);
			`),
	}
}
