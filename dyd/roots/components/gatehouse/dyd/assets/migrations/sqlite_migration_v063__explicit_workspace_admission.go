package migrations

func sqliteMigrationV063ExplicitWorkspaceAdmission() VersionedMigration {
	return VersionedMigration{Index: 63, Description: "explicit_workspace_admission", Builder: staticMigrationBuilder(`
		CREATE TEMP TABLE gatehouse_migration_admission_direct AS
		SELECT effective.workspace, effective.role, effective.principal,
			COALESCE(direct.id, gh_id_new('wgr')) AS id,
			COALESCE(direct.revision + 1, 1) AS revision,
			CASE WHEN direct.id IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END AS event, TRUE AS enabled
		FROM (
			SELECT DISTINCT grants.workspace, grants.role, members.principal_id AS principal
			FROM gatehouse_workspace_grants grants
			JOIN gatehouse_workspaces workspace ON workspace.id = grants.workspace AND workspace.enabled = TRUE
			JOIN gatehouse_groups groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group" AND groups.enabled = TRUE
			JOIN gatehouse_group_members members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id AND members.enabled = TRUE
			JOIN gatehouse_principals principal ON principal.id = members.principal_id AND principal.enabled = TRUE
			WHERE grants.enabled = TRUE
		) effective
		LEFT JOIN gatehouse_workspace_grants direct ON direct.workspace = effective.workspace AND direct.role = effective.role AND direct.principal = effective.principal
		WHERE direct.id IS NULL OR direct.enabled = FALSE;

		-- The historical empty-config bootstrap used admin/root through the root
		-- group. Preserve a revoked bootstrap as a disabled direct revision marker,
		-- so the new default principal grant (revision 1) cannot resurrect it.
		INSERT INTO gatehouse_migration_admission_direct
		SELECT workspace.id, 'manager', principal.id, gh_id_new('wgr'), 1, 'workspace_grant.create', FALSE
		FROM gatehouse_workspaces workspace JOIN gatehouse_principals principal ON principal.alias = 'root'
		WHERE workspace.alias = 'admin'
		AND NOT EXISTS (SELECT 1 FROM gatehouse_workspace_grants grants WHERE grants.workspace = workspace.id AND grants.principal = principal.id AND grants.role = 'manager')
		AND NOT EXISTS (SELECT 1 FROM gatehouse_migration_admission_direct grants WHERE grants.workspace = workspace.id AND grants.principal = principal.id AND grants.role = 'manager');

		CREATE TEMP TABLE gatehouse_migration_admission_events AS
		SELECT gh_id_new('act') AS activity, id, workspace, principal, NULL AS "group", event FROM gatehouse_migration_admission_direct
		UNION ALL
		SELECT gh_id_new('act'), id, workspace, NULL, "group", 'workspace_grant.update'
		FROM gatehouse_workspace_grants WHERE "group" IS NOT NULL AND enabled = TRUE;

		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		SELECT id, workspace, role, principal, NULL, enabled, revision FROM gatehouse_migration_admission_direct WHERE TRUE
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL
		DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision;
		UPDATE gatehouse_workspace_grants SET enabled = FALSE, revision = revision + 1 WHERE "group" IS NOT NULL AND enabled = TRUE;

		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_workspace_grant, created_at)
		SELECT activity, event, 'workspace_grant', id, gh_id_timestamp(activity) FROM gatehouse_migration_admission_events;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || id FROM gatehouse_migration_admission_events
		UNION
		SELECT activity, principal || '/' || id FROM gatehouse_migration_admission_events WHERE principal IS NOT NULL
		UNION
		SELECT events.activity, members.principal_id || '/' || events.id
		FROM gatehouse_migration_admission_events events JOIN gatehouse_group_members members ON members.workspace_id = events.workspace AND members.group_id = events."group";
		DROP TABLE gatehouse_migration_admission_events;
		DROP TABLE gatehouse_migration_admission_direct;

		-- Retired group grants remain immutable because activity history references
		-- their identities. Neither new group grants nor restoration are permitted.
		CREATE TRIGGER gatehouse_workspace_grants_principal_insert BEFORE INSERT ON gatehouse_workspace_grants
		WHEN NEW.principal IS NULL OR NEW."group" IS NOT NULL BEGIN SELECT RAISE(ABORT, 'workspace grants require a principal'); END;
		CREATE TRIGGER gatehouse_workspace_grants_principal_update BEFORE UPDATE ON gatehouse_workspace_grants
		WHEN NEW.principal IS NULL OR NEW."group" IS NOT NULL OR OLD."group" IS NOT NULL BEGIN SELECT RAISE(ABORT, 'group workspace grants are retired'); END;
	`)}
}
