package migrations

func postgresMigrationV057PrincipalIdentityActivity() VersionedMigration {
	return VersionedMigration{
		Index: 57, Description: "principal_identity_activity",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_activity_events DROP CONSTRAINT gatehouse_activity_events_check;
			ALTER TABLE gatehouse_activity_events DROP CONSTRAINT gatehouse_activity_events_resource_identity_fkey;
			ALTER TABLE gatehouse_activity_events RENAME COLUMN resource_identity TO detail_identity;
			UPDATE gatehouse_activity_events AS events
			SET resource_kind = 'principal', resource_principal = COALESCE(
				(SELECT substr(topic, 1, 30) FROM gatehouse_activity_event_topics
				 WHERE activity = events.id AND substr(topic, 31, 1) = '/' AND substr(topic, 32) = events.detail_identity ORDER BY topic LIMIT 1),
				identities.principal_id)
			FROM gatehouse_identities AS identities
			WHERE events.resource_kind = 'identity' AND events.detail_identity = identities.id;
			ALTER TABLE gatehouse_activity_events ADD CONSTRAINT gatehouse_activity_events_check CHECK (` + identityActivityV057Check(true) + `);
		`),
	}
}
