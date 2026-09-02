package migrations

func postgresMigrationV029CreateProjectSecrets() VersionedMigration {
	return VersionedMigration{
		Index:       29,
		Description: "create_project_secrets",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_project_secrets (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					id TEXT NOT NULL CHECK (id ~ '^psc_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
					author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
					description TEXT NOT NULL CHECK (length(trim(description)) > 0) CHECK (octet_length(description) <= 4096),
					ciphertext TEXT NOT NULL,
					enabled BOOLEAN NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					PRIMARY KEY (workspace, project, id),
					FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
				);

				CREATE INDEX gatehouse_project_secrets_by_project_created
				ON gatehouse_project_secrets (workspace, project, created_at DESC, id DESC);
			`),
	}
}
