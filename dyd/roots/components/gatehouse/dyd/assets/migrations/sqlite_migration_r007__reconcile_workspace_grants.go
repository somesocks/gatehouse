package migrations

import (
	"context"
	"fmt"
	"gatehouse/config"
)

func sqliteMigrationR007ReconcileWorkspaceGrants(state config.State) RepeatableMigration {
	return RepeatableMigration{Index: 7, Description: "reconcile_workspace_grants", Builder: sqliteMigrationR007ReconcileWorkspaceGrantsBuilder(state.WorkspaceGrants)}
}

func sqliteMigrationR007ReconcileWorkspaceGrantsBuilder(grants []config.WorkspaceGrant) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		for _, grant := range grants {
			if grant.PrincipalID == nil {
				return fmt.Errorf("workspace grants require a principal"), ""
			}
		}
		return session.RenderTemplate(`
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_desired (
			workspace_alias TEXT NOT NULL, role TEXT NOT NULL, principal_alias TEXT NOT NULL,
			revision INTEGER NOT NULL, enabled INTEGER NOT NULL,
			PRIMARY KEY (workspace_alias, role, principal_alias)
		) STRICT;
		{{ range . }}
		INSERT INTO gatehouse_migration_workspace_grant_desired VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Role }}, {{ sqlOptionalString .PrincipalID }}, {{ .Revision }}, {{ sqlBool .Enabled }});
		{{ end }}
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_state (
			workspace TEXT NOT NULL, role TEXT NOT NULL, principal TEXT NOT NULL, id TEXT NOT NULL,
			revision INTEGER NOT NULL, enabled INTEGER NOT NULL, event TEXT NOT NULL, activity TEXT NOT NULL
		) STRICT;
		INSERT INTO gatehouse_migration_workspace_grant_state
		SELECT workspace.id, desired.role, principal.id, COALESCE(grants.id, gh_id_new('wgr')), desired.revision, desired.enabled,
			CASE WHEN grants.id IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END, gh_id_new('act')
		FROM gatehouse_migration_workspace_grant_desired desired
		LEFT JOIN gatehouse_workspaces workspace ON workspace.alias = desired.workspace_alias
		LEFT JOIN gatehouse_principals principal ON principal.alias = desired.principal_alias
		LEFT JOIN gatehouse_workspace_grants grants ON grants.workspace = workspace.id AND grants.role = desired.role AND grants.principal = principal.id
		WHERE grants.id IS NULL OR grants.revision < desired.revision;
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		SELECT id, workspace, role, principal, NULL, enabled, revision FROM gatehouse_migration_workspace_grant_state WHERE TRUE
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_workspace_grant, created_at)
		SELECT activity, event, 'workspace_grant', id, gh_id_timestamp(activity) FROM gatehouse_migration_workspace_grant_state;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT activity, workspace || '/' || id FROM gatehouse_migration_workspace_grant_state
		UNION ALL SELECT activity, principal || '/' || id FROM gatehouse_migration_workspace_grant_state;
		DROP TABLE gatehouse_migration_workspace_grant_state;
		DROP TABLE gatehouse_migration_workspace_grant_desired;
		`, grants)
	}
}
