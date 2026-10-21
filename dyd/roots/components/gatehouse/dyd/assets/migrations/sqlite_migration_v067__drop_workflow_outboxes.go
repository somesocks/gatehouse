package migrations

func sqliteMigrationV067DropWorkflowOutboxes() VersionedMigration {
	return VersionedMigration{Index: 67, Description: "drop_workflow_outboxes", Builder: staticMigrationBuilder(dropWorkflowOutboxesSQL)}
}
