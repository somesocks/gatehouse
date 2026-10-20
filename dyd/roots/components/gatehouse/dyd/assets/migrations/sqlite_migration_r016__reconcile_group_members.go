package migrations

import "gatehouse/config"

func sqliteMigrationR016ReconcileGroupMembers(groups []config.Group) RepeatableMigration {
	return RepeatableMigration{Index: 16, Description: "reconcile_group_members", Builder: templateMigrationBuilder(`
		CREATE TEMP TABLE gatehouse_migration_member_desired (
			workspace_alias TEXT NOT NULL, group_alias TEXT NOT NULL, principal_alias TEXT NOT NULL,
			enabled INTEGER NOT NULL, revision INTEGER NOT NULL
		) STRICT;
		{{ range . }}{{ $group := . }}{{ range .Members }}
		INSERT INTO gatehouse_migration_member_desired VALUES ({{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.Alias }}, {{ sqlLiteral .PrincipalID }}, {{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }});
		{{ end }}{{ end }}
		CREATE TEMP TABLE gatehouse_migration_member_state (
			workspace TEXT NOT NULL, "group" TEXT NOT NULL, principal TEXT NOT NULL,
			enabled INTEGER NOT NULL, revision INTEGER NOT NULL, event TEXT NOT NULL, activity TEXT NOT NULL,
			admitted INTEGER NOT NULL CONSTRAINT group_members_require_workspace_admission CHECK (admitted = TRUE)
		) STRICT;
		INSERT INTO gatehouse_migration_member_state
		SELECT workspace.id, groups.id, principal.id, desired.enabled, desired.revision,
			CASE WHEN members.principal_id IS NULL THEN 'group_member.create' ELSE 'group_member.update' END, gh_id_new('act'),
			CASE WHEN desired.enabled = FALSE OR members.enabled = TRUE THEN TRUE
			ELSE principal.enabled = TRUE AND workspace.enabled = TRUE AND EXISTS (
				SELECT 1 FROM gatehouse_workspace_grants grants WHERE grants.workspace = workspace.id AND grants.principal = principal.id AND grants.enabled = TRUE
			) END
		FROM gatehouse_migration_member_desired desired
		LEFT JOIN gatehouse_workspaces workspace ON workspace.alias = desired.workspace_alias
		LEFT JOIN gatehouse_groups groups ON groups.workspace_id = workspace.id AND groups.alias = desired.group_alias
		LEFT JOIN gatehouse_principals principal ON principal.alias = desired.principal_alias
		LEFT JOIN gatehouse_group_members members ON members.workspace_id = workspace.id AND members.group_id = groups.id AND members.principal_id = principal.id
		WHERE members.principal_id IS NULL OR members.revision < desired.revision;
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision)
		SELECT workspace, "group", principal, enabled, revision FROM gatehouse_migration_member_state WHERE TRUE
		ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_group_members.revision < excluded.revision;
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_member_group, resource_group_member_principal, created_at)
		SELECT activity, event, 'group_member', "group", principal, gh_id_timestamp(activity) FROM gatehouse_migration_member_state;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || "group" FROM gatehouse_migration_member_state
		UNION ALL SELECT activity, principal || '/' || "group" FROM gatehouse_migration_member_state;
		DROP TABLE gatehouse_migration_member_state;
		DROP TABLE gatehouse_migration_member_desired;
	`, groups)}
}
