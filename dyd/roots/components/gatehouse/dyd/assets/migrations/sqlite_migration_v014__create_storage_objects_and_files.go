package migrations

func sqliteMigrationV014CreateStorageObjectsAndFiles() VersionedMigration {
	return VersionedMigration{
		Index:       14,
		Description: "create_storage_objects_and_files",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_storage_objects (
				id TEXT PRIMARY KEY CHECK (length(id) = 30 AND substr(id, 1, 4) = 'obj_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				provider TEXT NOT NULL REFERENCES gatehouse_storage_providers (id),
				object TEXT NOT NULL CHECK (length(trim(object)) > 0),
				state TEXT NOT NULL CHECK (state IN ('pending', 'success', 'failure')),
				sha256 BLOB CHECK (sha256 IS NULL OR length(sha256) = 32),
				size INTEGER CHECK (size IS NULL OR size >= 0),
				created_at TEXT NOT NULL,
				CHECK ((sha256 IS NULL AND size IS NULL) OR (sha256 IS NOT NULL AND size IS NOT NULL))
			) STRICT;

			CREATE TABLE gatehouse_embedded_storage_objects (
				id TEXT PRIMARY KEY
			) STRICT;

			CREATE TABLE gatehouse_embedded_storage_object_chunks (
				embedded_storage_object TEXT NOT NULL REFERENCES gatehouse_embedded_storage_objects (id),
				ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
				sha256 BLOB NOT NULL CHECK (length(sha256) = 32),
				size INTEGER NOT NULL CHECK (size > 0 AND size <= 262144),
				bytes BLOB NOT NULL CHECK (length(bytes) = size),
				PRIMARY KEY (embedded_storage_object, ordinal)
			) STRICT;

			CREATE TABLE gatehouse_session_files (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sfi_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				storage_object TEXT NOT NULL REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, id),
				UNIQUE (storage_object),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id)
			) STRICT;

			CREATE TABLE gatehouse_project_files (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pfi_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				storage_object TEXT NOT NULL REFERENCES gatehouse_storage_objects (id),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				media_type TEXT CHECK (media_type IS NULL OR length(trim(media_type)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, id),
				UNIQUE (storage_object),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id)
			) STRICT;
		`),
	}
}
