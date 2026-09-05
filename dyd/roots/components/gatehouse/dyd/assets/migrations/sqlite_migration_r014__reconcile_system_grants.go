package migrations

import "gatehouse/config"

func sqliteMigrationR014ReconcileSystemGrants(state config.State) RepeatableMigration {
	return RepeatableMigration{Index: 14, Description: "reconcile_system_grants", Builder: sqliteMigrationR014ReconcileSystemGrantsBuilder(state.SystemGrants)}
}

func sqliteMigrationR014ReconcileSystemGrantsBuilder(grants []config.SystemGrant) MigrationBuilder {
	return templateMigrationBuilder(`
		DROP TABLE IF EXISTS gatehouse_migration_system_grants_desired;
		DROP TABLE IF EXISTS gatehouse_migration_system_grants_state;
		DROP TABLE IF EXISTS gatehouse_migration_system_grant_activities;
		CREATE TEMP TABLE gatehouse_migration_system_grants_desired (
			principal_alias TEXT PRIMARY KEY NOT NULL,
			revision INTEGER NOT NULL,
			enabled INTEGER NOT NULL
		) STRICT;
		{{ range . }}
		INSERT INTO gatehouse_migration_system_grants_desired (principal_alias, revision, enabled)
		VALUES ({{ sqlLiteral .PrincipalID }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }});
		{{ end }}
		CREATE TEMP TABLE gatehouse_migration_system_grants_state AS
		SELECT desired.*, principals.id AS principal, COALESCE(grants.id, gh_id_new('syg')) AS grant_id, grants.revision AS existing_revision
		FROM gatehouse_migration_system_grants_desired AS desired
		JOIN gatehouse_principals AS principals ON principals.alias = desired.principal_alias
		LEFT JOIN gatehouse_system_grants AS grants ON grants.principal = principals.id
		WHERE grants.revision IS NULL OR grants.revision < desired.revision;
		CREATE TEMP TABLE gatehouse_migration_system_grant_activities (
			grant_id TEXT NOT NULL, principal TEXT NOT NULL, id TEXT NOT NULL, event TEXT NOT NULL
		) STRICT;
		INSERT INTO gatehouse_migration_system_grant_activities (grant_id, principal, id, event)
		SELECT grant_id, principal, gh_id_new('act'), CASE WHEN existing_revision IS NULL THEN 'system_grant.create' ELSE 'system_grant.update' END
		FROM gatehouse_migration_system_grants_state;
		INSERT INTO gatehouse_system_grants (id, principal, role, enabled, revision)
		SELECT grant_id, principal, 'manager', enabled, revision FROM gatehouse_migration_system_grants_state
		WHERE TRUE
		ON CONFLICT (principal) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_system_grants.revision < excluded.revision;
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_system_grant, created_at)
		SELECT id, event, 'system_grant', grant_id, gh_id_timestamp(id) FROM gatehouse_migration_system_grant_activities;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT id, 'sys/' || grant_id FROM gatehouse_migration_system_grant_activities
		UNION ALL
		SELECT id, principal || '/' || grant_id FROM gatehouse_migration_system_grant_activities;
		DROP TABLE gatehouse_migration_system_grant_activities;
		DROP TABLE gatehouse_migration_system_grants_state;
		DROP TABLE gatehouse_migration_system_grants_desired;
	`, grants)
}
