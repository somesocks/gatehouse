package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
)

func postgresMigrationR007ReconcileWorkspaceGrants(state config.State) RepeatableMigration {
	return RepeatableMigration{Index: 7, Description: "reconcile_workspace_grants", Builder: postgresMigrationR007ReconcileWorkspaceGrantsBuilder(state.WorkspaceGrants)}
}

func postgresMigrationR007ReconcileWorkspaceGrantsBuilder(grants []config.WorkspaceGrant) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		for _, grant := range grants {
			if grant.PrincipalID == nil {
				return fmt.Errorf("workspace grants require a principal"), ""
			}
		}
		// Only configuration contributes to the checksum. Identities, revisions,
		// and audit decisions are resolved after the execution transaction locks
		// the affected tables, never from a pre-lock builder snapshot.
		return session.RenderTemplate(`
		LOCK TABLE gatehouse_workspaces, gatehouse_principals, gatehouse_workspace_grants IN SHARE ROW EXCLUSIVE MODE;
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_desired (
			workspace_alias TEXT NOT NULL, role TEXT NOT NULL, principal_alias TEXT NOT NULL,
			revision BIGINT NOT NULL, enabled BOOLEAN NOT NULL,
			PRIMARY KEY (workspace_alias, role, principal_alias)
		) ON COMMIT DROP;
		{{ range . }}
		INSERT INTO gatehouse_migration_workspace_grant_desired VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Role }}, {{ sqlOptionalString .PrincipalID }}, {{ .Revision }}, {{ sqlBool .Enabled }});
		{{ end }}
		DO $$ DECLARE missing RECORD; BEGIN
			SELECT desired.workspace_alias, desired.principal_alias INTO missing
			FROM gatehouse_migration_workspace_grant_desired desired
			LEFT JOIN gatehouse_workspaces workspace ON workspace.alias = desired.workspace_alias
			LEFT JOIN gatehouse_principals principal ON principal.alias = desired.principal_alias
			WHERE workspace.id IS NULL OR principal.id IS NULL LIMIT 1;
			IF FOUND THEN RAISE EXCEPTION 'workspace % or principal % is unavailable', missing.workspace_alias, missing.principal_alias; END IF;
		END $$;
		CREATE FUNCTION pg_temp.gatehouse_workspace_grant_id(kind TEXT, at_time TIMESTAMPTZ) RETURNS TEXT AS $$
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
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_state ON COMMIT DROP AS
		SELECT workspace.id AS workspace, desired.role, principal.id AS principal,
			COALESCE(grants.id, pg_temp.gatehouse_workspace_grant_id('wgr', clock_timestamp())) AS id,
			desired.revision, desired.enabled,
			CASE WHEN grants.id IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END AS event,
			date_trunc('milliseconds', clock_timestamp()) AS created_at
		FROM gatehouse_migration_workspace_grant_desired desired
		JOIN gatehouse_workspaces workspace ON workspace.alias = desired.workspace_alias
		JOIN gatehouse_principals principal ON principal.alias = desired.principal_alias
		LEFT JOIN gatehouse_workspace_grants grants ON grants.workspace = workspace.id AND grants.role = desired.role AND grants.principal = principal.id
		WHERE grants.id IS NULL OR grants.revision < desired.revision;
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_applied (
			id TEXT NOT NULL, workspace TEXT NOT NULL, principal TEXT NOT NULL,
			event TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL, activity TEXT
		) ON COMMIT DROP;
		WITH changed AS (
			INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
			SELECT id, workspace, role, principal, NULL, enabled, revision FROM gatehouse_migration_workspace_grant_state
			ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_workspace_grants.revision < excluded.revision
			RETURNING id, workspace, principal, role
		)
		INSERT INTO gatehouse_migration_workspace_grant_applied (id, workspace, principal, event, created_at)
		SELECT changed.id, changed.workspace, changed.principal, state.event, state.created_at
		FROM changed JOIN gatehouse_migration_workspace_grant_state state
		ON state.workspace = changed.workspace AND state.principal = changed.principal AND state.role = changed.role;
		UPDATE gatehouse_migration_workspace_grant_applied SET activity = pg_temp.gatehouse_workspace_grant_id('act', created_at);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_workspace_grant, created_at)
		SELECT activity, event, 'workspace_grant', id, created_at FROM gatehouse_migration_workspace_grant_applied;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || id FROM gatehouse_migration_workspace_grant_applied
		UNION ALL SELECT activity, principal || '/' || id FROM gatehouse_migration_workspace_grant_applied;
		DROP FUNCTION pg_temp.gatehouse_workspace_grant_id(TEXT, TIMESTAMPTZ);
		`, grants)
	}
}
