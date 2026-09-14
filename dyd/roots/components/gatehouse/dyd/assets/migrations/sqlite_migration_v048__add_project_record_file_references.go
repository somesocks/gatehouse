package migrations

func sqliteMigrationV048AddProjectRecordFileReferences() VersionedMigration {
	return VersionedMigration{
		Index:       48,
		Description: "add_project_record_file_references",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_project_record_attributes_migration_48 (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pra_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT NOT NULL CHECK (name GLOB '[a-z]*') CHECK (name NOT GLOB '*[^a-z0-9_]*'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (length(CAST(label AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				type TEXT NOT NULL CHECK (type IN ('text', 'number', 'boolean', 'datetime', 'record', 'file')),
				target_schema TEXT,
				cardinality TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
				uniqueness TEXT NOT NULL CHECK (uniqueness IN ('none', 'record', 'global')),
				display TEXT NOT NULL CHECK (display IN ('none', 'primary', 'secondary')),
				display_order INTEGER NOT NULL DEFAULT 0 CHECK (display_order >= 0),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, id), UNIQUE (id), UNIQUE (workspace, project, schema, name),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, project, target_schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((type = 'record' AND target_schema IS NOT NULL) OR (type <> 'record' AND target_schema IS NULL)),
				CHECK (NOT (cardinality = 'one' AND uniqueness = 'record')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			INSERT INTO gatehouse_project_record_attributes_migration_48 (workspace, project, schema, id, name, label, description, type, target_schema, cardinality, uniqueness, display, display_order, author_principal, author_agent, created_at)
			SELECT workspace, project, schema, id, name, label, description, type, target_schema, cardinality, uniqueness, display, display_order, author_principal, author_agent, created_at
			FROM gatehouse_project_record_attributes;
			CREATE UNIQUE INDEX gatehouse_project_record_attributes_value_compatibility_migration_48 ON gatehouse_project_record_attributes_migration_48 (workspace, project, schema, id, type, cardinality, uniqueness);
			CREATE UNIQUE INDEX gatehouse_project_record_attributes_reference_target_migration_48 ON gatehouse_project_record_attributes_migration_48 (workspace, project, schema, id, target_schema);
			CREATE INDEX gatehouse_project_record_attributes_by_schema_name_migration_48 ON gatehouse_project_record_attributes_migration_48 (workspace, project, schema, name);
			CREATE INDEX gatehouse_project_record_attributes_by_schema_display_order_name_migration_48 ON gatehouse_project_record_attributes_migration_48 (workspace, project, schema, display_order, name, id);
			CREATE TABLE gatehouse_project_record_values_migration_48 (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL, record TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prv_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				attribute TEXT NOT NULL,
				value_type TEXT NOT NULL CHECK (value_type IN ('text', 'number', 'boolean', 'datetime', 'record', 'file')),
				value_text TEXT, value_number REAL, value_boolean INTEGER CHECK (value_boolean IN (0, 1)), value_datetime TEXT, value_reference_schema TEXT, value_reference TEXT, value_reference_file TEXT,
				value_key TEXT NOT NULL, attribute_cardinality TEXT NOT NULL CHECK (attribute_cardinality IN ('one', 'many')), attribute_uniqueness TEXT NOT NULL CHECK (attribute_uniqueness IN ('none', 'record', 'global')),
				sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, record, id), UNIQUE (id),
				FOREIGN KEY (workspace, project, schema, record) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute, value_type, attribute_cardinality, attribute_uniqueness) REFERENCES gatehouse_project_record_attributes_migration_48 (workspace, project, schema, id, type, cardinality, uniqueness) ON UPDATE CASCADE ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute, value_reference_schema) REFERENCES gatehouse_project_record_attributes_migration_48 (workspace, project, schema, id, target_schema) ON UPDATE CASCADE ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, value_reference_schema, value_reference) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE RESTRICT,
				FOREIGN KEY (workspace, project, value_reference_file) REFERENCES gatehouse_project_files (workspace, project, id) ON DELETE RESTRICT,
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((value_type = 'text' AND value_text IS NOT NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'number' AND value_text IS NULL AND value_number IS NOT NULL AND value_number = value_number AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'boolean' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NOT NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'datetime' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NOT NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'record' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NOT NULL AND value_reference IS NOT NULL AND value_reference_file IS NULL) OR (value_type = 'file' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NOT NULL)),
				CHECK (NOT (sensitive = 1 AND attribute_uniqueness = 'global')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			INSERT INTO gatehouse_project_record_values_migration_48 (workspace, project, schema, record, id, attribute, value_type, value_text, value_number, value_boolean, value_datetime, value_reference_schema, value_reference, value_key, attribute_cardinality, attribute_uniqueness, sensitive, author_principal, author_agent, created_at)
			SELECT workspace, project, schema, record, id, attribute, value_type, value_text, value_number, value_boolean, value_datetime, value_reference_schema, value_reference, value_key, attribute_cardinality, attribute_uniqueness, sensitive, author_principal, author_agent, created_at
			FROM gatehouse_project_record_values;
			DROP TABLE gatehouse_project_record_values;
			DROP TABLE gatehouse_project_record_attributes;
			ALTER TABLE gatehouse_project_record_attributes_migration_48 RENAME TO gatehouse_project_record_attributes;
			ALTER TABLE gatehouse_project_record_values_migration_48 RENAME TO gatehouse_project_record_values;
			DROP INDEX gatehouse_project_record_attributes_value_compatibility_migration_48;
			DROP INDEX gatehouse_project_record_attributes_reference_target_migration_48;
			DROP INDEX gatehouse_project_record_attributes_by_schema_name_migration_48;
			DROP INDEX gatehouse_project_record_attributes_by_schema_display_order_name_migration_48;
			CREATE UNIQUE INDEX gatehouse_project_record_attributes_value_compatibility ON gatehouse_project_record_attributes (workspace, project, schema, id, type, cardinality, uniqueness);
			CREATE UNIQUE INDEX gatehouse_project_record_attributes_reference_target ON gatehouse_project_record_attributes (workspace, project, schema, id, target_schema);
			CREATE INDEX gatehouse_project_record_attributes_by_schema_name ON gatehouse_project_record_attributes (workspace, project, schema, name);
			CREATE INDEX gatehouse_project_record_attributes_by_schema_display_order_name ON gatehouse_project_record_attributes (workspace, project, schema, display_order, name, id);
			CREATE UNIQUE INDEX gatehouse_project_record_values_one ON gatehouse_project_record_values (workspace, project, schema, record, attribute) WHERE attribute_cardinality = 'one';
			CREATE UNIQUE INDEX gatehouse_project_record_values_record_unique ON gatehouse_project_record_values (workspace, project, schema, record, attribute, value_key) WHERE attribute_uniqueness = 'record';
			CREATE UNIQUE INDEX gatehouse_project_record_values_global_unique ON gatehouse_project_record_values (workspace, project, schema, attribute, value_key) WHERE attribute_uniqueness = 'global';
			CREATE INDEX gatehouse_project_record_values_by_record_created ON gatehouse_project_record_values (workspace, project, schema, record, created_at DESC, id DESC);
			CREATE INDEX gatehouse_project_record_values_by_reference ON gatehouse_project_record_values (workspace, project, value_reference_schema, value_reference, id DESC) WHERE value_type = 'record';
			CREATE INDEX gatehouse_project_record_values_by_file_reference ON gatehouse_project_record_values (workspace, project, value_reference_file, id DESC) WHERE value_type = 'file';
			CREATE TRIGGER gatehouse_project_record_values_file_reference_validate_insert
			BEFORE INSERT ON gatehouse_project_record_values
			WHEN NEW.value_type = 'file'
			BEGIN SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM gatehouse_project_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = NEW.workspace AND files.project = NEW.project AND files.id = NEW.value_reference_file AND files.enabled = 1 AND objects.state = 'success') THEN RAISE(ABORT, 'project file reference is unavailable') END; END;
			CREATE TRIGGER gatehouse_project_record_values_file_reference_validate_update
			BEFORE UPDATE ON gatehouse_project_record_values
			WHEN NEW.value_type = 'file'
			BEGIN SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM gatehouse_project_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = NEW.workspace AND files.project = NEW.project AND files.id = NEW.value_reference_file AND files.enabled = 1 AND objects.state = 'success') THEN RAISE(ABORT, 'project file reference is unavailable') END; END;
			CREATE TRIGGER gatehouse_project_files_reject_remove_when_referenced
			BEFORE UPDATE OF enabled ON gatehouse_project_files
			WHEN OLD.enabled = 1 AND NEW.enabled = 0 AND EXISTS (SELECT 1 FROM gatehouse_project_record_values WHERE workspace = OLD.workspace AND project = OLD.project AND value_type = 'file' AND value_reference_file = OLD.id)
			BEGIN SELECT RAISE(ABORT, 'project file is referenced by a record value'); END;
		`),
	}
}
