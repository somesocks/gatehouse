package migrations

func postgresMigrationV065DropProjectSecrets() VersionedMigration {
	return VersionedMigration{Index: 65, Description: "drop_project_secrets", Builder: staticMigrationBuilder(`
		DELETE FROM gatehouse_activity_event_topics WHERE activity IN (
			SELECT id FROM gatehouse_activity_events WHERE resource_kind = 'project_secret'
		);
		DELETE FROM gatehouse_activity_events WHERE resource_kind = 'project_secret';
		ALTER TABLE gatehouse_activity_events
			DROP CONSTRAINT gatehouse_activity_events_check,
			DROP COLUMN resource_project_secret;
		ALTER TABLE gatehouse_activity_events ADD CONSTRAINT gatehouse_activity_events_check CHECK (` + projectSecretsV065ActivityCheck(true) + `);
		DROP TABLE gatehouse_project_secrets;
	`)}
}
