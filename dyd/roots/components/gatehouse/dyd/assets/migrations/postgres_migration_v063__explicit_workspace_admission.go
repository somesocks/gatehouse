package migrations

func postgresMigrationV063ExplicitWorkspaceAdmission() VersionedMigration {
	return VersionedMigration{Index: 63, Description: "explicit_workspace_admission", Builder: staticMigrationBuilder(`
		LOCK TABLE gatehouse_workspace_grants, gatehouse_workspaces, gatehouse_groups, gatehouse_group_members, gatehouse_principals IN SHARE ROW EXCLUSIVE MODE;
		CREATE FUNCTION pg_temp.gatehouse_admission_id(kind TEXT, at_time TIMESTAMPTZ DEFAULT clock_timestamp()) RETURNS TEXT AS $$
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

		CREATE TEMP TABLE gatehouse_migration_admission_direct ON COMMIT DROP AS
		SELECT effective.workspace, effective.role, effective.principal,
			COALESCE(direct.id, pg_temp.gatehouse_admission_id('wgr')) AS id,
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

		-- Preserve revocation of the historical admin/root group bootstrap. Its
		-- new default direct grant must not recreate admission at revision 1.
		INSERT INTO gatehouse_migration_admission_direct
		SELECT workspace.id, 'manager', principal.id, pg_temp.gatehouse_admission_id('wgr'), 1, 'workspace_grant.create', FALSE
		FROM gatehouse_workspaces workspace JOIN gatehouse_principals principal ON principal.alias = 'root'
		WHERE workspace.alias = 'admin'
		AND NOT EXISTS (SELECT 1 FROM gatehouse_workspace_grants grants WHERE grants.workspace = workspace.id AND grants.principal = principal.id AND grants.role = 'manager')
		AND NOT EXISTS (SELECT 1 FROM gatehouse_migration_admission_direct grants WHERE grants.workspace = workspace.id AND grants.principal = principal.id AND grants.role = 'manager');
		CREATE TEMP TABLE gatehouse_migration_admission_events ON COMMIT DROP AS
		SELECT id, workspace, principal, NULL::TEXT AS "group", event, date_trunc('milliseconds', clock_timestamp()) AS created_at
		FROM gatehouse_migration_admission_direct
		UNION ALL
		SELECT id, workspace, NULL, "group", 'workspace_grant.update', date_trunc('milliseconds', clock_timestamp())
		FROM gatehouse_workspace_grants WHERE "group" IS NOT NULL AND enabled = TRUE;
		ALTER TABLE gatehouse_migration_admission_events ADD COLUMN activity TEXT;
		UPDATE gatehouse_migration_admission_events SET activity = pg_temp.gatehouse_admission_id('act', created_at);
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		SELECT id, workspace, role, principal, NULL, enabled, revision FROM gatehouse_migration_admission_direct
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision;
		UPDATE gatehouse_workspace_grants SET enabled = FALSE, revision = revision + 1 WHERE "group" IS NOT NULL AND enabled = TRUE;
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_workspace_grant, created_at)
		SELECT activity, event, 'workspace_grant', id, created_at FROM gatehouse_migration_admission_events;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || id FROM gatehouse_migration_admission_events
		UNION
		SELECT activity, principal || '/' || id FROM gatehouse_migration_admission_events WHERE principal IS NOT NULL
		UNION
		SELECT events.activity, members.principal_id || '/' || events.id
		FROM gatehouse_migration_admission_events events JOIN gatehouse_group_members members ON members.workspace_id = events.workspace AND members.group_id = events."group";
		DROP FUNCTION pg_temp.gatehouse_admission_id(TEXT, TIMESTAMPTZ);

		CREATE FUNCTION gatehouse_workspace_grants_principal_only() RETURNS trigger AS $$
		BEGIN
			IF NEW.principal IS NULL OR NEW."group" IS NOT NULL THEN RAISE EXCEPTION 'workspace grants require a principal'; END IF;
			IF TG_OP = 'UPDATE' AND OLD."group" IS NOT NULL THEN RAISE EXCEPTION 'group workspace grants are retired'; END IF;
			RETURN NEW;
		END; $$ LANGUAGE plpgsql;
		CREATE TRIGGER gatehouse_workspace_grants_principal_only BEFORE INSERT OR UPDATE ON gatehouse_workspace_grants
		FOR EACH ROW EXECUTE FUNCTION gatehouse_workspace_grants_principal_only();
	`)}
}
