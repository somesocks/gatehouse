package migrations

import (
	"context"

	"gatehouse/config"
)

func sqliteMigrationR004ReconcilePrincipals(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       4,
		Description: "reconcile_principals",
		Builder:     sqliteMigrationR004ReconcilePrincipalsBuilder(state.Principals),
	}
}

func sqliteMigrationR004ReconcilePrincipalsBuilder(principals []config.Principal) MigrationBuilder {
	for index := range principals {
		if principals[index].Revision == 0 {
			principals[index].Revision = config.DefaultPrincipalRevision
		}
	}
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_principal_desired;
			DROP TABLE IF EXISTS gatehouse_migration_principal_state;
			DROP TABLE IF EXISTS gatehouse_migration_principal_activities;

			CREATE TEMP TABLE gatehouse_migration_principal_desired (
				alias TEXT NOT NULL PRIMARY KEY,
				name TEXT,
				revision INTEGER NOT NULL,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_principal_desired (alias, name, revision, enabled)
			VALUES ({{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_principal_state AS
			SELECT desired.*, COALESCE(principals.id, gh_id_new('prn')) AS id, principals.revision AS existing_revision
			FROM gatehouse_migration_principal_desired AS desired
			LEFT JOIN gatehouse_principals AS principals ON principals.alias = desired.alias
			WHERE principals.id IS NULL OR principals.revision < desired.revision;

			CREATE TEMP TABLE gatehouse_migration_principal_activities (
				principal TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_principal_activities (principal, id, event)
			SELECT id, gh_id_new('act'), CASE WHEN existing_revision IS NULL THEN 'principal.create' ELSE 'principal.update' END
			FROM gatehouse_migration_principal_state;

			INSERT INTO gatehouse_principals (id, alias, name, revision, enabled)
			SELECT id, alias, name, revision, enabled
			FROM gatehouse_migration_principal_state
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET name = excluded.name, revision = excluded.revision, enabled = excluded.enabled
			WHERE gatehouse_principals.revision < excluded.revision;

			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_principal, created_at
			)
			SELECT id, event, 'principal', principal, gh_id_timestamp(id)
			FROM gatehouse_migration_principal_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, principal FROM gatehouse_migration_principal_activities;

			DROP TABLE gatehouse_migration_principal_activities;
			DROP TABLE gatehouse_migration_principal_state;
			DROP TABLE gatehouse_migration_principal_desired;
		`, principals)
	}
}
