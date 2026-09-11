package migrations

func postgresMigrationV043IndexProjectRecordReferences() VersionedMigration {
	return VersionedMigration{
		Index:       43,
		Description: "index_project_record_references",
		Builder: staticMigrationBuilder(`
			CREATE INDEX gatehouse_project_record_values_by_reference
			ON gatehouse_project_record_values (workspace, project, value_reference, id DESC)
			WHERE value_type = 'record';
		`),
	}
}
