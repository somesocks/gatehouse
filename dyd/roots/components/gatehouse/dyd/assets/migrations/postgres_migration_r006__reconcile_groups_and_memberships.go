package migrations

import "gatehouse/config"

func postgresMigrationR006ReconcileGroupsAndMemberships(state config.State) RepeatableMigration {
	return RepeatableMigration{Index: 6, Description: "reconcile_groups_and_memberships", Builder: postgresMigrationR006ReconcileGroupsAndMembershipsBuilder(state.Groups)}
}

func postgresMigrationR006ReconcileGroupsAndMembershipsBuilder(groups []config.Group) MigrationBuilder {
	return templateMigrationBuilder(`
		LOCK TABLE gatehouse_groups, gatehouse_group_members IN SHARE ROW EXCLUSIVE MODE;
		CREATE FUNCTION pg_temp.gatehouse_group_migration_id(kind TEXT, at_time TIMESTAMPTZ) RETURNS TEXT AS $$
		DECLARE
			alphabet CONSTANT TEXT := '0123456789abcdefghjkmnpqrstvwxyz';
			milliseconds BIGINT := floor(extract(epoch FROM at_time) * 1000)::BIGINT;
			encoded TEXT := ''; position INTEGER;
		BEGIN
			FOR position IN REVERSE 9..0 LOOP
				encoded := substr(alphabet, (milliseconds % 32)::INTEGER + 1, 1) || encoded;
				milliseconds := milliseconds / 32;
			END LOOP;
			FOR position IN 1..16 LOOP encoded := encoded || substr(alphabet, floor(random() * 32)::INTEGER + 1, 1); END LOOP;
			RETURN kind || '_' || encoded;
		END; $$ LANGUAGE plpgsql VOLATILE;
		CREATE TEMP TABLE gatehouse_migration_group_desired (
			workspace_alias TEXT NOT NULL, alias TEXT NOT NULL, name TEXT, enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL,
			PRIMARY KEY (workspace_alias, alias)
		) ON COMMIT DROP;
		{{ range . }}
		INSERT INTO gatehouse_migration_group_desired VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }});
		{{ end }}
		CREATE TEMP TABLE gatehouse_migration_group_state ON COMMIT DROP AS
		SELECT desired.*, workspace.id AS workspace, COALESCE(groups.id, pg_temp.gatehouse_group_migration_id('grp', clock_timestamp())) AS id,
			CASE WHEN groups.id IS NULL THEN 'group.create' ELSE 'group.update' END AS event, date_trunc('milliseconds', clock_timestamp()) AS created_at
		FROM gatehouse_migration_group_desired desired
		LEFT JOIN gatehouse_workspaces workspace ON workspace.alias = desired.workspace_alias
		LEFT JOIN gatehouse_groups groups ON groups.workspace_id = workspace.id AND groups.alias = desired.alias
		WHERE groups.id IS NULL OR groups.revision < desired.revision;
		ALTER TABLE gatehouse_migration_group_state ADD COLUMN activity TEXT;
		UPDATE gatehouse_migration_group_state SET activity = pg_temp.gatehouse_group_migration_id('act', created_at);
		INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled, revision)
		SELECT workspace, id, alias, name, enabled, revision FROM gatehouse_migration_group_state
		ON CONFLICT (workspace_id, alias) DO UPDATE SET name = excluded.name, enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_groups.revision < excluded.revision;
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group, created_at)
		SELECT activity, event, 'group', id, created_at FROM gatehouse_migration_group_state;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || id FROM gatehouse_migration_group_state
		UNION
		SELECT state.activity, members.principal_id || '/' || state.id FROM gatehouse_migration_group_state state
		JOIN gatehouse_group_members members ON members.workspace_id = state.workspace AND members.group_id = state.id AND members.enabled = TRUE
		JOIN gatehouse_principals principal ON principal.id = members.principal_id AND principal.enabled = TRUE
		JOIN gatehouse_workspaces admitted_workspace ON admitted_workspace.id = members.workspace_id AND admitted_workspace.enabled = TRUE
		JOIN gatehouse_workspace_grants grants ON grants.workspace = members.workspace_id AND grants.principal = members.principal_id AND grants.enabled = TRUE;
		DROP FUNCTION pg_temp.gatehouse_group_migration_id(TEXT, TIMESTAMPTZ);
	`, groups)
}
