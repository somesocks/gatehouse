package migrations

import "gatehouse/config"

func sqliteMigrationR016ReconcileGroupMembers(groups []config.Group) RepeatableMigration {
	return RepeatableMigration{Index: 16, Description: "reconcile_group_members", Builder: templateMigrationBuilder(`
		CREATE TEMP TABLE gatehouse_migration_member_desired (
			workspace_alias TEXT NOT NULL, group_alias TEXT NOT NULL, principal_alias TEXT NOT NULL,
			enabled INTEGER NOT NULL, revision INTEGER NOT NULL, role TEXT NOT NULL
		) STRICT;
		{{ range . }}{{ $group := . }}{{ range .Members }}
		INSERT INTO gatehouse_migration_member_desired VALUES ({{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.Alias }}, {{ sqlLiteral .PrincipalID }}, {{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }}, {{ if .Role }}{{ sqlLiteral .Role }}{{ else }}'member'{{ end }});
		{{ end }}{{ end }}
		CREATE TEMP TABLE gatehouse_migration_member_state (
			workspace TEXT NOT NULL, "group" TEXT NOT NULL, principal TEXT NOT NULL,
			enabled INTEGER NOT NULL, revision INTEGER NOT NULL, role TEXT NOT NULL, event TEXT NOT NULL, activity TEXT NOT NULL,
			admitted INTEGER NOT NULL CONSTRAINT group_members_require_workspace_admission CHECK (admitted = TRUE)
		) STRICT;
		INSERT INTO gatehouse_migration_member_state
		SELECT workspace.id, groups.id, principal.id, desired.enabled, desired.revision, desired.role,
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
		CREATE TEMP TABLE gatehouse_migration_member_guard (
			has_manager INTEGER NOT NULL CONSTRAINT group_requires_enabled_manager CHECK (has_manager = TRUE)
		) STRICT;
		WITH admitted AS (
			SELECT DISTINCT grants.workspace, grants.principal FROM gatehouse_workspace_grants grants
			JOIN gatehouse_workspaces workspace ON workspace.id = grants.workspace AND workspace.enabled = TRUE
			JOIN gatehouse_principals principal ON principal.id = grants.principal AND principal.enabled = TRUE
			WHERE grants.enabled = TRUE
		)
		INSERT INTO gatehouse_migration_member_guard
		SELECT EXISTS (
			SELECT 1 FROM gatehouse_migration_member_state replacement
			JOIN admitted ON admitted.workspace = replacement.workspace AND admitted.principal = replacement.principal
			WHERE replacement.workspace = changes.workspace AND replacement."group" = changes."group"
			AND replacement.enabled = TRUE AND replacement.role = 'manager'
		) OR EXISTS (
			SELECT 1 FROM gatehouse_group_members remaining
			JOIN admitted ON admitted.workspace = remaining.workspace_id AND admitted.principal = remaining.principal_id
			WHERE remaining.workspace_id = changes.workspace AND remaining.group_id = changes."group"
			AND remaining.enabled = TRUE AND remaining.role = 'manager'
			AND NOT EXISTS (SELECT 1 FROM gatehouse_migration_member_state edited
				WHERE edited.workspace = remaining.workspace_id AND edited."group" = remaining.group_id AND edited.principal = remaining.principal_id)
		)
		FROM gatehouse_migration_member_state changes
		JOIN gatehouse_group_members current ON current.workspace_id = changes.workspace AND current.group_id = changes."group" AND current.principal_id = changes.principal
		JOIN gatehouse_groups groups ON groups.workspace_id = current.workspace_id AND groups.id = current.group_id AND groups.enabled = TRUE
		JOIN admitted ON admitted.workspace = current.workspace_id AND admitted.principal = current.principal_id
		WHERE current.enabled = TRUE AND current.role = 'manager' AND (changes.enabled = FALSE OR changes.role <> 'manager');
		DROP TABLE gatehouse_migration_member_guard;
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision, role)
		SELECT workspace, "group", principal, enabled, revision, role FROM gatehouse_migration_member_state WHERE TRUE
		ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision, role = excluded.role
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
