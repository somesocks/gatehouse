package migrations

func sqliteMigrationV016CreateSessionNotes() VersionedMigration {
	return VersionedMigration{
		Index:       16,
		Description: "create_session_notes",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_notes (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'snt_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) <= 1048576),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;

				CREATE INDEX gatehouse_session_notes_by_session_created
				ON gatehouse_session_notes (workspace, session, created_at DESC, id DESC);
			`),
	}
}
