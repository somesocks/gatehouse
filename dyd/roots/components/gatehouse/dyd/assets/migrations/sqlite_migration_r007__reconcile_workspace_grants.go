package migrations

import (
	"gatehouse/config"
)

func sqliteMigrationR007ReconcileWorkspaceGrants(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       7,
		Description: "reconcile_workspace_grants",
		Builder:     sqliteMigrationR007ReconcileWorkspaceGrantsBuilder(state.WorkspaceGrants),
	}
}

type sqliteMigrationR007WorkspaceGrantValue struct {
	WorkspaceID string
	Role        string
	SubjectID   string
	Revision    int
	Enabled     bool
}

type sqliteMigrationR007WorkspaceGrantValues struct {
	Principals []sqliteMigrationR007WorkspaceGrantValue
	Groups     []sqliteMigrationR007WorkspaceGrantValue
}

func sqliteMigrationR007ReconcileWorkspaceGrantsBuilder(grants []config.WorkspaceGrant) MigrationBuilder {
	values := sqliteMigrationR007WorkspaceGrantValues{
		Principals: make([]sqliteMigrationR007WorkspaceGrantValue, 0, len(grants)),
		Groups:     make([]sqliteMigrationR007WorkspaceGrantValue, 0, len(grants)),
	}
	for _, grant := range grants {
		value := sqliteMigrationR007WorkspaceGrantValue{WorkspaceID: grant.WorkspaceID, Role: grant.Role, Revision: grant.Revision, Enabled: grant.Enabled}
		if grant.PrincipalID != nil {
			value.SubjectID = *grant.PrincipalID
			values.Principals = append(values.Principals, value)
		} else {
			value.SubjectID = *grant.GroupID
			values.Groups = append(values.Groups, value)
		}
	}
	return templateMigrationBuilder(`
		DROP TABLE IF EXISTS gatehouse_migration_workspace_grant_principals_desired;
		DROP TABLE IF EXISTS gatehouse_migration_workspace_grant_groups_desired;
		DROP TABLE IF EXISTS gatehouse_migration_workspace_grant_principals_state;
		DROP TABLE IF EXISTS gatehouse_migration_workspace_grant_groups_state;
		DROP TABLE IF EXISTS gatehouse_migration_workspace_grant_activities;

		CREATE TEMP TABLE gatehouse_migration_workspace_grant_principals_desired (
			workspace_alias TEXT NOT NULL,
			role TEXT NOT NULL,
			principal_alias TEXT NOT NULL,
			revision INTEGER NOT NULL,
			enabled INTEGER NOT NULL,
			PRIMARY KEY (workspace_alias, role, principal_alias)
		) STRICT;
		{{ range .Principals }}
		INSERT INTO gatehouse_migration_workspace_grant_principals_desired (workspace_alias, role, principal_alias, revision, enabled)
		VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Role }}, {{ sqlLiteral .SubjectID }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }});
		{{ end }}

		CREATE TEMP TABLE gatehouse_migration_workspace_grant_groups_desired (
			workspace_alias TEXT NOT NULL,
			role TEXT NOT NULL,
			group_alias TEXT NOT NULL,
			revision INTEGER NOT NULL,
			enabled INTEGER NOT NULL,
			PRIMARY KEY (workspace_alias, role, group_alias)
		) STRICT;
		{{ range .Groups }}
		INSERT INTO gatehouse_migration_workspace_grant_groups_desired (workspace_alias, role, group_alias, revision, enabled)
		VALUES ({{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .Role }}, {{ sqlLiteral .SubjectID }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }});
		{{ end }}

		CREATE TEMP TABLE gatehouse_migration_workspace_grant_principals_state AS
		SELECT desired.*, workspaces.id AS workspace, principals.id AS principal,
			COALESCE(grants.id, gh_id_new('wgr')) AS grant_id, grants.revision AS existing_revision
		FROM gatehouse_migration_workspace_grant_principals_desired AS desired
		JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
		JOIN gatehouse_principals AS principals ON principals.alias = desired.principal_alias
		LEFT JOIN gatehouse_workspace_grants AS grants
			ON grants.workspace = workspaces.id AND grants.role = desired.role AND grants.principal = principals.id
		WHERE grants.revision IS NULL OR grants.revision < desired.revision;
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_groups_state AS
		SELECT desired.*, workspaces.id AS workspace, groups.id AS "group",
			COALESCE(grants.id, gh_id_new('wgr')) AS grant_id, grants.revision AS existing_revision
		FROM gatehouse_migration_workspace_grant_groups_desired AS desired
		JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
		JOIN gatehouse_groups AS groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.group_alias
		LEFT JOIN gatehouse_workspace_grants AS grants
			ON grants.workspace = workspaces.id AND grants.role = desired.role AND grants."group" = groups.id
		WHERE grants.revision IS NULL OR grants.revision < desired.revision;

		CREATE TEMP TABLE gatehouse_migration_workspace_grant_activities (
			workspace TEXT NOT NULL,
			grant_id TEXT NOT NULL,
			principal TEXT,
			"group" TEXT,
			id TEXT NOT NULL,
			event TEXT NOT NULL
		) STRICT;
		INSERT INTO gatehouse_migration_workspace_grant_activities (workspace, grant_id, principal, id, event)
		SELECT workspace, grant_id, principal, gh_id_new('act'), CASE WHEN existing_revision IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END
		FROM gatehouse_migration_workspace_grant_principals_state;
		INSERT INTO gatehouse_migration_workspace_grant_activities (workspace, grant_id, "group", id, event)
		SELECT workspace, grant_id, "group", gh_id_new('act'), CASE WHEN existing_revision IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END
		FROM gatehouse_migration_workspace_grant_groups_state;

		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		SELECT grant_id, workspace, role, principal, NULL, enabled, revision
		FROM gatehouse_migration_workspace_grant_principals_state
		WHERE TRUE
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		SELECT grant_id, workspace, role, NULL, "group", enabled, revision
		FROM gatehouse_migration_workspace_grant_groups_state
		WHERE TRUE
		ON CONFLICT (workspace, role, "group") WHERE "group" IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;

		INSERT INTO gatehouse_activity_events (
			id, event, resource_kind, resource_workspace_grant, created_at
		)
		SELECT id, event, 'workspace_grant', grant_id, gh_id_timestamp(id)
		FROM gatehouse_migration_workspace_grant_activities;
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		SELECT id, workspace || '/' || grant_id
		FROM gatehouse_migration_workspace_grant_activities
		WHERE principal IS NOT NULL
		UNION ALL
		SELECT id, principal || '/' || grant_id
		FROM gatehouse_migration_workspace_grant_activities
		WHERE principal IS NOT NULL
		UNION ALL
		SELECT id, workspace || '/' || "group" || '/' || grant_id
		FROM gatehouse_migration_workspace_grant_activities
		WHERE "group" IS NOT NULL;

		DROP TABLE gatehouse_migration_workspace_grant_activities;
		DROP TABLE gatehouse_migration_workspace_grant_groups_state;
		DROP TABLE gatehouse_migration_workspace_grant_principals_state;
		DROP TABLE gatehouse_migration_workspace_grant_groups_desired;
		DROP TABLE gatehouse_migration_workspace_grant_principals_desired;
	`, values)
}
