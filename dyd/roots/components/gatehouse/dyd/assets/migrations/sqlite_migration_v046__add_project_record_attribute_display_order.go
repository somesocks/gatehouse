package migrations

func sqliteMigrationV046AddProjectRecordAttributeDisplayOrder() VersionedMigration {
	return VersionedMigration{
		Index:       46,
		Description: "add_project_record_attribute_display_order",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_project_record_attributes
				ADD COLUMN display_order INTEGER NOT NULL DEFAULT 0 CHECK (display_order >= 0);
			CREATE INDEX gatehouse_project_record_attributes_by_schema_display_order_name
			ON gatehouse_project_record_attributes (workspace, project, schema, display_order, name, id);
		`),
	}
}
