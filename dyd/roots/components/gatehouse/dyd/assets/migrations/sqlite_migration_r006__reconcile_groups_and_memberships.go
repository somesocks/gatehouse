package migrations

import (
	"context"
	"gatehouse/config"
)

func sqliteMigrationR006ReconcileGroupsAndMemberships(groups []config.Group) RepeatableMigration {
	return RepeatableMigration{
		Index:       6,
		Description: "reconcile_groups_and_memberships",
		Builder:     sqliteMigrationR006ReconcileGroupsAndMembershipsBuilder(groups),
	}
}

func sqliteMigrationR006ReconcileGroupsAndMembershipsBuilder(groups []config.Group) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_group_desired;
			DROP TABLE IF EXISTS gatehouse_migration_group_members_desired;
			DROP TABLE IF EXISTS gatehouse_migration_group_state;
			DROP TABLE IF EXISTS gatehouse_migration_group_member_state;
			DROP TABLE IF EXISTS gatehouse_migration_group_activities;

			CREATE TEMP TABLE gatehouse_migration_group_desired (
				workspace_alias TEXT NOT NULL,
				alias TEXT NOT NULL,
				name TEXT,
				enabled INTEGER NOT NULL,
				PRIMARY KEY (workspace_alias, alias)
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_group_desired (workspace_alias, alias, name, enabled)
			VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_group_members_desired (
				workspace_alias TEXT NOT NULL,
				group_alias TEXT NOT NULL,
				principal_alias TEXT NOT NULL,
				enabled INTEGER NOT NULL,
				PRIMARY KEY (workspace_alias, group_alias, principal_alias)
			) STRICT;
			{{ range . }}
			{{ $group := . }}
			{{ range .Members }}
			INSERT INTO gatehouse_migration_group_members_desired (workspace_alias, group_alias, principal_alias, enabled)
			VALUES ({{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.Alias }}, {{ sqlLiteral .PrincipalID }}, {{ sqlBool .Enabled }});
			{{ end }}
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_group_state AS
			SELECT
				desired.workspace_alias,
				desired.alias,
				workspaces.id AS workspace_id,
				COALESCE(groups.id, gh_id_new('grp')) AS group_id,
				groups.id AS existing_group_id,
				desired.name,
				groups.name AS existing_name,
				desired.enabled,
				groups.enabled AS existing_enabled
			FROM gatehouse_migration_group_desired AS desired
			LEFT JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
			LEFT JOIN gatehouse_groups AS groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.alias;

			CREATE TEMP TABLE gatehouse_migration_group_member_state AS
			SELECT
				desired.workspace_alias,
				desired.group_alias,
				desired.principal_alias,
				groups.workspace_id,
				groups.group_id,
				principals.id AS principal_id,
				members.principal_id AS existing_principal_id,
				desired.enabled,
				members.enabled AS existing_enabled
			FROM gatehouse_migration_group_members_desired AS desired
			JOIN gatehouse_migration_group_state AS groups ON groups.workspace_alias = desired.workspace_alias AND groups.alias = desired.group_alias
			LEFT JOIN gatehouse_principals AS principals ON principals.alias = desired.principal_alias
			LEFT JOIN gatehouse_group_members AS members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.group_id AND members.principal_id = principals.id;

			CREATE TEMP TABLE gatehouse_migration_group_activities (
				workspace_id TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL,
				resource_kind TEXT NOT NULL,
				group_id TEXT NOT NULL,
				principal_id TEXT
			) STRICT;
			INSERT INTO gatehouse_migration_group_activities (workspace_id, id, event, resource_kind, group_id, principal_id)
			SELECT
				workspace_id,
				gh_id_new('act'),
				CASE WHEN existing_group_id IS NULL THEN 'group.create' ELSE 'group.update' END,
				'group',
				group_id,
				NULL
			FROM gatehouse_migration_group_state
			WHERE existing_group_id IS NULL OR existing_name IS NOT name OR existing_enabled IS NOT enabled;
			INSERT INTO gatehouse_migration_group_activities (workspace_id, id, event, resource_kind, group_id, principal_id)
			SELECT
				workspace_id,
				gh_id_new('act'),
				CASE WHEN existing_principal_id IS NULL THEN 'group_member.create' ELSE 'group_member.update' END,
				'group_member',
				group_id,
				principal_id
			FROM gatehouse_migration_group_member_state
			WHERE existing_principal_id IS NULL OR existing_enabled IS NOT enabled;

			INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled)
			SELECT workspace_id, group_id, alias, name, enabled
			FROM gatehouse_migration_group_state
			WHERE TRUE
			ON CONFLICT (workspace_id, alias) DO UPDATE SET name = excluded.name, enabled = excluded.enabled;
			INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled)
			SELECT workspace_id, group_id, principal_id, enabled
			FROM gatehouse_migration_group_member_state
			WHERE TRUE
			ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET enabled = excluded.enabled;

			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_group, resource_group_member_group, resource_group_member_principal, created_at
			)
			SELECT id, event, resource_kind,
				CASE WHEN resource_kind = 'group' THEN group_id END,
				CASE WHEN resource_kind = 'group_member' THEN group_id END,
				CASE WHEN resource_kind = 'group_member' THEN principal_id END,
				gh_id_timestamp(id)
			FROM gatehouse_migration_group_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, workspace_id || '/' || group_id
			FROM gatehouse_migration_group_activities
			WHERE resource_kind = 'group'
			UNION ALL
			SELECT id, principal_id || '/' || group_id
			FROM gatehouse_migration_group_activities
			WHERE resource_kind = 'group_member'
			UNION ALL
			SELECT id, workspace_id || '/' || group_id
			FROM gatehouse_migration_group_activities
			WHERE resource_kind = 'group_member';

			DROP TABLE gatehouse_migration_group_activities;
			DROP TABLE gatehouse_migration_group_member_state;
			DROP TABLE gatehouse_migration_group_state;
			DROP TABLE gatehouse_migration_group_members_desired;
			DROP TABLE gatehouse_migration_group_desired;
		`, groups)
	}
}
