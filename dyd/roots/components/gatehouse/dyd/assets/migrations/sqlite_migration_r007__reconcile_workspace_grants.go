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
		SELECT desired.*, workspaces.id AS workspace, principals.id AS principal, grants.revision AS existing_revision
		FROM gatehouse_migration_workspace_grant_principals_desired AS desired
		JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
		JOIN gatehouse_principals AS principals ON principals.alias = desired.principal_alias
		LEFT JOIN gatehouse_workspace_grants AS grants
			ON grants.workspace = workspaces.id AND grants.role = desired.role AND grants.principal = principals.id
		WHERE grants.revision IS NULL OR grants.revision < desired.revision;
		CREATE TEMP TABLE gatehouse_migration_workspace_grant_groups_state AS
		SELECT desired.*, workspaces.id AS workspace, groups.id AS "group", grants.revision AS existing_revision
		FROM gatehouse_migration_workspace_grant_groups_desired AS desired
		JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.workspace_alias
		JOIN gatehouse_groups AS groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.group_alias
		LEFT JOIN gatehouse_workspace_grants AS grants
			ON grants.workspace = workspaces.id AND grants.role = desired.role AND grants."group" = groups.id
		WHERE grants.revision IS NULL OR grants.revision < desired.revision;

		CREATE TEMP TABLE gatehouse_migration_workspace_grant_activities (
			workspace TEXT NOT NULL,
			id TEXT NOT NULL,
			event TEXT NOT NULL,
			topic TEXT NOT NULL
		) STRICT;
		INSERT INTO gatehouse_migration_workspace_grant_activities (workspace, id, event, topic)
		SELECT workspace, gh_id_new('act'), CASE WHEN existing_revision IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END, 'workspace_grant/' || role || '-' || principal
		FROM gatehouse_migration_workspace_grant_principals_state;
		INSERT INTO gatehouse_migration_workspace_grant_activities (workspace, id, event, topic)
		SELECT workspace, gh_id_new('act'), CASE WHEN existing_revision IS NULL THEN 'workspace_grant.create' ELSE 'workspace_grant.update' END, 'workspace_grant/' || role || '-' || "group"
		FROM gatehouse_migration_workspace_grant_groups_state;

		INSERT INTO gatehouse_workspace_grants (workspace, role, principal, "group", enabled, revision)
		SELECT workspace, role, principal, NULL, enabled, revision
		FROM gatehouse_migration_workspace_grant_principals_state
		WHERE TRUE
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;
		INSERT INTO gatehouse_workspace_grants (workspace, role, principal, "group", enabled, revision)
		SELECT workspace, role, NULL, "group", enabled, revision
		FROM gatehouse_migration_workspace_grant_groups_state
		WHERE TRUE
		ON CONFLICT (workspace, role, "group") WHERE "group" IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;

		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
		)
		SELECT workspace, id, event, 'workspace_grant', NULL, NULL, NULL, NULL, NULL, NULL, gh_id_timestamp(id)
		FROM gatehouse_migration_workspace_grant_activities;
		INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
		SELECT workspace, id, topic, gh_id_timestamp(id)
		FROM gatehouse_migration_workspace_grant_activities;

		DROP TABLE gatehouse_migration_workspace_grant_activities;
		DROP TABLE gatehouse_migration_workspace_grant_groups_state;
		DROP TABLE gatehouse_migration_workspace_grant_principals_state;
		DROP TABLE gatehouse_migration_workspace_grant_groups_desired;
		DROP TABLE gatehouse_migration_workspace_grant_principals_desired;
	`, values)
}
