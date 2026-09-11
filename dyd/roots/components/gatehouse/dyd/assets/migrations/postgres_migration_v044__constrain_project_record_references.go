package migrations

func postgresMigrationV044ConstrainProjectRecordReferences() VersionedMigration {
	return VersionedMigration{
		Index:       44,
		Description: "constrain_project_record_references",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_project_record_values
				ADD COLUMN value_reference_schema TEXT;
			UPDATE gatehouse_project_record_values AS values_table
			SET value_reference_schema = attributes.target_schema
			FROM gatehouse_project_record_attributes AS attributes
			WHERE values_table.value_type = 'record'
				AND attributes.workspace = values_table.workspace
				AND attributes.project = values_table.project
				AND attributes.schema = values_table.schema
				AND attributes.id = values_table.attribute;
			DROP TRIGGER gatehouse_project_record_values_validate ON gatehouse_project_record_values;
			DROP FUNCTION gatehouse_project_record_value_validate();
			DROP TRIGGER gatehouse_project_record_attributes_validate_change ON gatehouse_project_record_attributes;
			DROP FUNCTION gatehouse_project_record_attribute_validate_change();
			DO $$
			DECLARE foreign_key TEXT;
			BEGIN
				SELECT constraints.conname INTO foreign_key
				FROM pg_constraint AS constraints
				WHERE constraints.conrelid = 'gatehouse_project_record_values'::regclass
					AND constraints.contype = 'f'
					AND constraints.confrelid = 'gatehouse_project_record_attributes'::regclass
					AND pg_get_constraintdef(constraints.oid) LIKE 'FOREIGN KEY (workspace, project, schema, attribute) REFERENCES gatehouse_project_record_attributes%';
				IF foreign_key IS NULL THEN RAISE EXCEPTION 'project record values attribute foreign key is unavailable'; END IF;
				EXECUTE format('ALTER TABLE gatehouse_project_record_values DROP CONSTRAINT %I', foreign_key);
			END;
			$$;
			ALTER TABLE gatehouse_project_record_attributes
				ADD CONSTRAINT gatehouse_project_record_attributes_value_compatibility_unique
					UNIQUE (workspace, project, schema, id, type, cardinality, uniqueness),
				ADD CONSTRAINT gatehouse_project_record_attributes_reference_target_unique
					UNIQUE (workspace, project, schema, id, target_schema);
			ALTER TABLE gatehouse_project_record_values
				ADD CONSTRAINT gatehouse_project_record_values_reference_schema_check
					CHECK ((value_type = 'record' AND value_reference_schema IS NOT NULL) OR (value_type <> 'record' AND value_reference_schema IS NULL)),
				ADD CONSTRAINT gatehouse_project_record_values_sensitive_global_check
					CHECK (NOT (sensitive AND attribute_uniqueness = 'global')),
				ADD CONSTRAINT gatehouse_project_record_values_attribute_compatibility_foreign_key
					FOREIGN KEY (workspace, project, schema, attribute, value_type, attribute_cardinality, attribute_uniqueness)
					REFERENCES gatehouse_project_record_attributes (workspace, project, schema, id, type, cardinality, uniqueness)
					ON UPDATE CASCADE ON DELETE CASCADE,
				ADD CONSTRAINT gatehouse_project_record_values_attribute_reference_foreign_key
					FOREIGN KEY (workspace, project, schema, attribute, value_reference_schema)
					REFERENCES gatehouse_project_record_attributes (workspace, project, schema, id, target_schema)
					ON UPDATE CASCADE ON DELETE CASCADE,
				ADD CONSTRAINT gatehouse_project_record_values_reference_foreign_key
					FOREIGN KEY (workspace, project, value_reference_schema, value_reference)
					REFERENCES gatehouse_project_records (workspace, project, schema, id)
					ON DELETE RESTRICT;
			DROP INDEX gatehouse_project_record_values_by_reference;
			CREATE INDEX gatehouse_project_record_values_by_reference
			ON gatehouse_project_record_values (workspace, project, value_reference_schema, value_reference, id DESC)
			WHERE value_type = 'record';
		`),
	}
}
