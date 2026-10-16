package migrations

import (
	"context"

	"gatehouse/config"
)

func postgresMigrationR006ReconcileGroupsAndMemberships(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       6,
		Description: "reconcile_groups_and_memberships",
		Builder:     postgresMigrationR006ReconcileGroupsAndMembershipsBuilder(state.Groups),
	}
}

func postgresMigrationR006ReconcileGroupsAndMembershipsBuilder(groups []config.Group) MigrationBuilder {
	if len(groups) == 0 {
		return staticMigrationBuilder("SELECT 1;")
	}
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			CREATE TEMP TABLE gatehouse_migration_group_desired (
				workspace_alias TEXT NOT NULL, alias TEXT NOT NULL, name TEXT,
				enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL,
				PRIMARY KEY (workspace_alias, alias)
			) ON COMMIT DROP;
			{{ range . }}
			INSERT INTO gatehouse_migration_group_desired VALUES (
				{{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }},
				{{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }}
			);
			{{ end }}
			CREATE TEMP TABLE gatehouse_migration_group_members_desired (
				workspace_alias TEXT NOT NULL, group_alias TEXT NOT NULL, principal_alias TEXT NOT NULL,
				enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL,
				PRIMARY KEY (workspace_alias, group_alias, principal_alias)
			) ON COMMIT DROP;
			{{ range . }}{{ $group := . }}{{ range .Members }}
			INSERT INTO gatehouse_migration_group_members_desired VALUES (
				{{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.Alias }}, {{ sqlLiteral .PrincipalID }},
				{{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }}
			);
			{{ end }}{{ end }}

			-- Resolve aliases only after excluding concurrent group and membership
			-- writes. Ordinary reads remain available throughout reconciliation.
			LOCK TABLE gatehouse_groups, gatehouse_group_members IN SHARE ROW EXCLUSIVE MODE;

			-- IDs must be generated at execution time too. This session-local helper
			-- follows the typed ULID encoding used by prior PostgreSQL migrations;
			-- no function is installed in the application's persistent schema.
			CREATE FUNCTION pg_temp.gatehouse_group_migration_id(kind TEXT, at_time TIMESTAMPTZ DEFAULT clock_timestamp()) RETURNS TEXT AS $$
			DECLARE
				alphabet CONSTANT TEXT := '0123456789abcdefghjkmnpqrstvwxyz';
				milliseconds BIGINT := floor(extract(epoch FROM at_time) * 1000)::BIGINT;
				encoded TEXT := '';
				position INTEGER;
			BEGIN
				FOR position IN REVERSE 9..0 LOOP
					encoded := substr(alphabet, (milliseconds % 32)::INTEGER + 1, 1) || encoded;
					milliseconds := milliseconds / 32;
				END LOOP;
				FOR position IN 1..16 LOOP
					encoded := encoded || substr(alphabet, floor(random() * 32)::INTEGER + 1, 1);
				END LOOP;
				RETURN kind || '_' || encoded;
			END;
			$$ LANGUAGE plpgsql VOLATILE;

			CREATE TEMP TABLE gatehouse_migration_group_state ON COMMIT DROP AS
			SELECT desired.*, workspaces.id AS workspace_id,
				COALESCE(groups.id, pg_temp.gatehouse_group_migration_id('grp')) AS group_id,
				groups.revision AS existing_revision
			FROM gatehouse_migration_group_desired AS desired
			LEFT JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
			LEFT JOIN gatehouse_groups AS groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.alias;
			CREATE TEMP TABLE gatehouse_migration_group_member_state ON COMMIT DROP AS
			SELECT desired.*, groups.workspace_id, groups.group_id, principals.id AS principal_id,
				members.revision AS existing_revision
			FROM gatehouse_migration_group_members_desired AS desired
			JOIN gatehouse_migration_group_state AS groups ON groups.workspace_alias = desired.workspace_alias AND groups.alias = desired.group_alias
			LEFT JOIN gatehouse_principals AS principals ON principals.alias = desired.principal_alias
			LEFT JOIN gatehouse_group_members AS members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.group_id AND members.principal_id = principals.id;

			CREATE TEMP TABLE gatehouse_migration_group_activities (
				workspace_id TEXT NOT NULL, id TEXT NOT NULL, event TEXT NOT NULL,
				resource_kind TEXT NOT NULL, group_id TEXT NOT NULL, principal_id TEXT,
				event_time TIMESTAMPTZ NOT NULL
			) ON COMMIT DROP;
			WITH changes AS MATERIALIZED (
				SELECT *, date_trunc('milliseconds', clock_timestamp()) AS event_time
				FROM gatehouse_migration_group_state WHERE existing_revision IS NULL OR existing_revision < revision
			)
			INSERT INTO gatehouse_migration_group_activities
			SELECT workspace_id, pg_temp.gatehouse_group_migration_id('act', event_time),
				CASE WHEN existing_revision IS NULL THEN 'group.create' ELSE 'group.update' END,
				'group', group_id, NULL, event_time FROM changes;
			WITH changes AS MATERIALIZED (
				SELECT *, date_trunc('milliseconds', clock_timestamp()) AS event_time
				FROM gatehouse_migration_group_member_state WHERE existing_revision IS NULL OR existing_revision < revision
			)
			INSERT INTO gatehouse_migration_group_activities
			SELECT workspace_id, pg_temp.gatehouse_group_migration_id('act', event_time),
				CASE WHEN existing_revision IS NULL THEN 'group_member.create' ELSE 'group_member.update' END,
				'group_member', group_id, principal_id, event_time FROM changes;

			INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled, revision)
			SELECT workspace_id, group_id, alias, name, enabled, revision FROM gatehouse_migration_group_state
			ON CONFLICT (workspace_id, alias) DO UPDATE SET name = excluded.name, enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_groups.revision < excluded.revision;
			INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision)
			SELECT workspace_id, group_id, principal_id, enabled, revision FROM gatehouse_migration_group_member_state
			ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_group_members.revision < excluded.revision;

			INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group, resource_group_member_group, resource_group_member_principal, created_at)
			SELECT id, event, resource_kind,
				CASE WHEN resource_kind = 'group' THEN group_id END,
				CASE WHEN resource_kind = 'group_member' THEN group_id END,
				CASE WHEN resource_kind = 'group_member' THEN principal_id END,
				event_time
			FROM gatehouse_migration_group_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, workspace_id || '/' || group_id FROM gatehouse_migration_group_activities
			UNION ALL
			SELECT activities.id, members.principal_id || '/' || activities.group_id
			FROM gatehouse_migration_group_activities AS activities
			JOIN gatehouse_group_members AS members ON members.workspace_id = activities.workspace_id AND members.group_id = activities.group_id
			WHERE activities.resource_kind = 'group'
			UNION ALL
			SELECT id, principal_id || '/' || group_id FROM gatehouse_migration_group_activities WHERE resource_kind = 'group_member';
			DROP FUNCTION pg_temp.gatehouse_group_migration_id(TEXT, TIMESTAMPTZ);
		`, groups)
	}
}
