package migrations

func sqliteMigrationV029CreateProjectSecrets() VersionedMigration {
	return VersionedMigration{
		Index:       29,
		Description: "create_project_secrets",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_project_secrets (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'psc_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
					description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (length(CAST(description AS BLOB)) <= 4096),
					ciphertext TEXT NOT NULL,
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					PRIMARY KEY (workspace, project, id),
					FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
				) STRICT;

				CREATE INDEX gatehouse_project_secrets_by_project_created
				ON gatehouse_project_secrets (workspace, project, created_at DESC, id DESC);
			`),
	}
}
