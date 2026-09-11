package migrations

func sqliteMigrationV044ConstrainProjectRecordReferences() VersionedMigration {
	return VersionedMigration{
		Index:       44,
		Description: "constrain_project_record_references",
		Builder: staticMigrationBuilder(`
			DROP TRIGGER gatehouse_project_record_attributes_validate_change;
			CREATE UNIQUE INDEX gatehouse_project_record_attributes_value_compatibility
			ON gatehouse_project_record_attributes (workspace, project, schema, id, type, cardinality, uniqueness);
			CREATE UNIQUE INDEX gatehouse_project_record_attributes_reference_target
			ON gatehouse_project_record_attributes (workspace, project, schema, id, target_schema);
			CREATE TABLE gatehouse_project_record_values_migration_44 (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL, record TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prv_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				attribute TEXT NOT NULL,
				value_type TEXT NOT NULL CHECK (value_type IN ('text', 'number', 'boolean', 'datetime', 'record')),
				value_text TEXT, value_number REAL, value_boolean INTEGER CHECK (value_boolean IN (0, 1)), value_datetime TEXT, value_reference_schema TEXT, value_reference TEXT,
				value_key TEXT NOT NULL, attribute_cardinality TEXT NOT NULL CHECK (attribute_cardinality IN ('one', 'many')), attribute_uniqueness TEXT NOT NULL CHECK (attribute_uniqueness IN ('none', 'record', 'global')),
				sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, record, id), UNIQUE (id),
				FOREIGN KEY (workspace, project, schema, record) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute, value_type, attribute_cardinality, attribute_uniqueness) REFERENCES gatehouse_project_record_attributes (workspace, project, schema, id, type, cardinality, uniqueness) ON UPDATE CASCADE ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute, value_reference_schema) REFERENCES gatehouse_project_record_attributes (workspace, project, schema, id, target_schema) ON UPDATE CASCADE ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, value_reference_schema, value_reference) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE RESTRICT,
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((value_type = 'text' AND value_text IS NOT NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL) OR (value_type = 'number' AND value_text IS NULL AND value_number IS NOT NULL AND value_number = value_number AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL) OR (value_type = 'boolean' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NOT NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL) OR (value_type = 'datetime' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NOT NULL AND value_reference_schema IS NULL AND value_reference IS NULL) OR (value_type = 'record' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NOT NULL AND value_reference IS NOT NULL)),
				CHECK (NOT (sensitive = 1 AND attribute_uniqueness = 'global')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			INSERT INTO gatehouse_project_record_values_migration_44 (workspace, project, schema, record, id, attribute, value_type, value_text, value_number, value_boolean, value_datetime, value_reference_schema, value_reference, value_key, attribute_cardinality, attribute_uniqueness, sensitive, author_principal, author_agent, created_at)
			SELECT values_table.workspace, values_table.project, values_table.schema, values_table.record, values_table.id, values_table.attribute, values_table.value_type, values_table.value_text, values_table.value_number, values_table.value_boolean, values_table.value_datetime, attributes.target_schema, values_table.value_reference, values_table.value_key, values_table.attribute_cardinality, values_table.attribute_uniqueness, values_table.sensitive, values_table.author_principal, values_table.author_agent, values_table.created_at
			FROM gatehouse_project_record_values AS values_table
			JOIN gatehouse_project_record_attributes AS attributes ON attributes.workspace = values_table.workspace AND attributes.project = values_table.project AND attributes.schema = values_table.schema AND attributes.id = values_table.attribute;
			DROP TABLE gatehouse_project_record_values;
			ALTER TABLE gatehouse_project_record_values_migration_44 RENAME TO gatehouse_project_record_values;
			CREATE UNIQUE INDEX gatehouse_project_record_values_one ON gatehouse_project_record_values (workspace, project, schema, record, attribute) WHERE attribute_cardinality = 'one';
			CREATE UNIQUE INDEX gatehouse_project_record_values_record_unique ON gatehouse_project_record_values (workspace, project, schema, record, attribute, value_key) WHERE attribute_uniqueness = 'record';
			CREATE UNIQUE INDEX gatehouse_project_record_values_global_unique ON gatehouse_project_record_values (workspace, project, schema, attribute, value_key) WHERE attribute_uniqueness = 'global';
			CREATE INDEX gatehouse_project_record_values_by_record_created ON gatehouse_project_record_values (workspace, project, schema, record, created_at DESC, id DESC);
			CREATE INDEX gatehouse_project_record_values_by_reference ON gatehouse_project_record_values (workspace, project, value_reference_schema, value_reference, id DESC) WHERE value_type = 'record';
		`),
	}
}
