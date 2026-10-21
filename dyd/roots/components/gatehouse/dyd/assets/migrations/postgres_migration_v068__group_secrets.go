package migrations

func postgresMigrationV068GroupSecrets() VersionedMigration {
	return VersionedMigration{Index: 68, Description: "group_secrets", Builder: staticMigrationBuilder(`
		CREATE TABLE gatehouse_group_secrets (
			workspace TEXT NOT NULL,
			"group" TEXT NOT NULL,
			id TEXT NOT NULL UNIQUE CHECK (id ~ '^gsc_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
			author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
			description TEXT NOT NULL CHECK (length(trim(description)) > 0 AND octet_length(description) <= 4096),
			ciphertext TEXT NOT NULL,
			enabled BOOLEAN NOT NULL,
			revision BIGINT NOT NULL CHECK (revision > 0),
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			PRIMARY KEY (workspace, "group", id),
			FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id),
			CHECK (enabled = FALSE OR length(ciphertext) > 0)
		);
		ALTER TABLE gatehouse_activity_events
			ADD COLUMN resource_group_secret TEXT REFERENCES gatehouse_group_secrets (id),
			DROP CONSTRAINT gatehouse_activity_events_check;
		ALTER TABLE gatehouse_activity_events ADD CONSTRAINT gatehouse_activity_events_check CHECK (` + groupSecretsV068ActivityCheck(true) + `);
	`)}
}
