package migrations

import "gatehouse/config"

// R006 retains its historical identity; human memberships now follow admission
// in R016. Group metadata must exist before workspace bots are reconciled.
func sqliteMigrationR006ReconcileGroupsAndMemberships(groups []config.Group) RepeatableMigration {
	return RepeatableMigration{Index: 6, Description: "reconcile_groups_and_memberships", Builder: sqliteMigrationR006ReconcileGroupsAndMembershipsBuilder(groups)}
}

func sqliteMigrationR006ReconcileGroupsAndMembershipsBuilder(groups []config.Group) MigrationBuilder {
	return templateMigrationBuilder(`
		CREATE TEMP TABLE gatehouse_migration_group_desired (
			workspace_alias TEXT NOT NULL, alias TEXT NOT NULL, name TEXT, enabled INTEGER NOT NULL, revision INTEGER NOT NULL,
			PRIMARY KEY (workspace_alias, alias)
		) STRICT;
		{{ range . }}
		INSERT INTO gatehouse_migration_group_desired VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }});
		{{ end }}
		CREATE TEMP TABLE gatehouse_migration_group_state AS
		SELECT desired.*, workspace.id AS workspace, COALESCE(groups.id, gh_id_new('grp')) AS id,
			CASE WHEN groups.id IS NULL THEN 'group.create' ELSE 'group.update' END AS event, gh_id_new('act') AS activity
		FROM gatehouse_migration_group_desired desired
		LEFT JOIN gatehouse_workspaces workspace ON workspace.alias = desired.workspace_alias
		LEFT JOIN gatehouse_groups groups ON groups.workspace_id = workspace.id AND groups.alias = desired.alias
		WHERE groups.id IS NULL OR groups.revision < desired.revision;
		INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled, revision)
		SELECT workspace, id, alias, name, enabled, revision FROM gatehouse_migration_group_state WHERE TRUE
		ON CONFLICT (workspace_id, alias) DO UPDATE SET name = excluded.name, enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_groups.revision < excluded.revision;
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group, created_at)
		SELECT activity, event, 'group', id, gh_id_timestamp(activity) FROM gatehouse_migration_group_state;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || id FROM gatehouse_migration_group_state
		UNION
		SELECT state.activity, members.principal_id || '/' || state.id FROM gatehouse_migration_group_state state
		JOIN gatehouse_group_members members ON members.workspace_id = state.workspace AND members.group_id = state.id AND members.enabled = TRUE
		JOIN gatehouse_principals principal ON principal.id = members.principal_id AND principal.enabled = TRUE
		JOIN gatehouse_workspaces admitted_workspace ON admitted_workspace.id = members.workspace_id AND admitted_workspace.enabled = TRUE
		JOIN gatehouse_workspace_grants grants ON grants.workspace = members.workspace_id AND grants.principal = members.principal_id AND grants.enabled = TRUE;
		DROP TABLE gatehouse_migration_group_state;
		DROP TABLE gatehouse_migration_group_desired;
	`, groups)
}
