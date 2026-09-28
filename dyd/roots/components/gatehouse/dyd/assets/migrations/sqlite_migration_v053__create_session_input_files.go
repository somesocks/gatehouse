package migrations

func sqliteMigrationV053CreateSessionInputFiles() VersionedMigration {
	return VersionedMigration{
		Index: 53, Description: "create_session_input_files",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_input_files (
				workspace TEXT NOT NULL, session TEXT NOT NULL, input TEXT NOT NULL,
				field_path TEXT NOT NULL CHECK (json_valid(field_path)), file TEXT NOT NULL,
				PRIMARY KEY (workspace, session, file),
				FOREIGN KEY (workspace, session, input) REFERENCES gatehouse_session_events (workspace, session, id),
				FOREIGN KEY (workspace, session, file) REFERENCES gatehouse_session_files (workspace, session, id)
			) STRICT;
		`),
	}
}
