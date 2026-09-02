package migrations

func postgresMigrationV014CreateStorageObjectsAndFiles() VersionedMigration {
	return VersionedMigration{
		Index:       14,
		Description: "create_storage_objects_and_files",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_storage_objects (
				id TEXT PRIMARY KEY CHECK (id ~ '^obj_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				object TEXT NOT NULL CHECK (length(trim(object)) > 0),
				state TEXT NOT NULL CHECK (state IN ('pending', 'success', 'failure')),
				sha256 BYTEA CHECK (sha256 IS NULL OR octet_length(sha256) = 32),
				size BIGINT CHECK (size IS NULL OR size >= 0),
				created_at TIMESTAMPTZ NOT NULL,
				CHECK ((sha256 IS NULL AND size IS NULL) OR (sha256 IS NOT NULL AND size IS NOT NULL))
			);

			CREATE TABLE gatehouse_embedded_storage_objects (
				id TEXT PRIMARY KEY
			);

			CREATE TABLE gatehouse_embedded_storage_object_chunks (
				embedded_storage_object TEXT NOT NULL REFERENCES gatehouse_embedded_storage_objects (id),
				ordinal BIGINT NOT NULL CHECK (ordinal >= 0),
				sha256 BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
				size BIGINT NOT NULL CHECK (size > 0 AND size <= 262144),
				bytes BYTEA NOT NULL CHECK (octet_length(bytes) = size),
				PRIMARY KEY (embedded_storage_object, ordinal)
			);

			CREATE TABLE gatehouse_session_files (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^sfi_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				storage_object TEXT NOT NULL UNIQUE REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, session, id),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			);

			CREATE TABLE gatehouse_project_files (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^pfi_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				storage_object TEXT NOT NULL UNIQUE REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				enabled BOOLEAN NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, project, id),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			);
		`),
	}
}
