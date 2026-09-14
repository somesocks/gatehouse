package migrations

func postgresMigrationV048AddProjectRecordFileReferences() VersionedMigration {
	return VersionedMigration{
		Index:       48,
		Description: "add_project_record_file_references",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_project_record_attributes
				DROP CONSTRAINT gatehouse_project_record_attributes_type_check,
				ADD CONSTRAINT gatehouse_project_record_attributes_type_check CHECK (type IN ('text', 'number', 'boolean', 'datetime', 'record', 'file')),
				DROP CONSTRAINT gatehouse_project_record_attributes_check,
				ADD CONSTRAINT gatehouse_project_record_attributes_check CHECK ((type = 'record' AND target_schema IS NOT NULL) OR (type <> 'record' AND target_schema IS NULL));
			ALTER TABLE gatehouse_project_record_values
				ADD COLUMN value_reference_file TEXT,
				DROP CONSTRAINT gatehouse_project_record_values_value_type_check,
				ADD CONSTRAINT gatehouse_project_record_values_value_type_check CHECK (value_type IN ('text', 'number', 'boolean', 'datetime', 'record', 'file')),
				DROP CONSTRAINT gatehouse_project_record_values_check,
				ADD CONSTRAINT gatehouse_project_record_values_check CHECK ((value_type = 'text' AND value_text IS NOT NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'number' AND value_text IS NULL AND value_number IS NOT NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'boolean' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NOT NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'datetime' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NOT NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NULL) OR (value_type = 'record' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NOT NULL AND value_reference IS NOT NULL AND value_reference_file IS NULL) OR (value_type = 'file' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference_schema IS NULL AND value_reference IS NULL AND value_reference_file IS NOT NULL)),
				ADD CONSTRAINT gatehouse_project_record_values_file_reference_foreign_key FOREIGN KEY (workspace, project, value_reference_file) REFERENCES gatehouse_project_files (workspace, project, id) ON DELETE RESTRICT;
			CREATE INDEX gatehouse_project_record_values_by_file_reference ON gatehouse_project_record_values (workspace, project, value_reference_file, id DESC) WHERE value_type = 'file';
			CREATE FUNCTION gatehouse_project_record_file_reference_validate() RETURNS TRIGGER AS $$
			BEGIN
				IF NEW.value_type = 'file' AND NOT EXISTS (SELECT 1 FROM gatehouse_project_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = NEW.workspace AND files.project = NEW.project AND files.id = NEW.value_reference_file AND files.enabled AND objects.state = 'success') THEN
					RAISE EXCEPTION 'project file reference is unavailable';
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;
			CREATE TRIGGER gatehouse_project_record_values_file_reference_validate BEFORE INSERT OR UPDATE ON gatehouse_project_record_values FOR EACH ROW EXECUTE FUNCTION gatehouse_project_record_file_reference_validate();
			CREATE FUNCTION gatehouse_project_file_remove_referenced_validate() RETURNS TRIGGER AS $$
			BEGIN
				IF OLD.enabled AND NOT NEW.enabled AND EXISTS (SELECT 1 FROM gatehouse_project_record_values WHERE workspace = OLD.workspace AND project = OLD.project AND value_type = 'file' AND value_reference_file = OLD.id) THEN
					RAISE EXCEPTION 'project file is referenced by a record value';
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;
			CREATE TRIGGER gatehouse_project_files_reject_remove_when_referenced BEFORE UPDATE OF enabled ON gatehouse_project_files FOR EACH ROW EXECUTE FUNCTION gatehouse_project_file_remove_referenced_validate();
		`),
	}
}
