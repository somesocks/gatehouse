package migrations

import (
	"context"

	"gatehouse/config"
)

type workspaceGrantMigrationValue struct {
	WorkspaceID string
	Role        string
	SubjectID   string
	Revision    int
	Enabled     bool
}

type workspaceGrantMigrationValues struct {
	Principals []workspaceGrantMigrationValue
	Groups     []workspaceGrantMigrationValue
}

func workspaceGrantMigrationBuilder(grants []config.WorkspaceGrant) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		values := workspaceGrantMigrationValues{
			Principals: make([]workspaceGrantMigrationValue, 0, len(grants)),
			Groups:     make([]workspaceGrantMigrationValue, 0, len(grants)),
		}
		for _, grant := range grants {
			value := workspaceGrantMigrationValue{WorkspaceID: grant.WorkspaceID, Role: grant.Role, Revision: grant.Revision, Enabled: grant.Enabled}
			if grant.PrincipalID != nil {
				value.SubjectID = *grant.PrincipalID
				values.Principals = append(values.Principals, value)
			} else {
				value.SubjectID = *grant.GroupID
				values.Groups = append(values.Groups, value)
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Principals }}
			INSERT INTO gatehouse_workspace_grants (workspace, role, principal, "group", enabled, revision)
			VALUES (
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}),
				{{ sqlLiteral .Role }},
				(SELECT id FROM gatehouse_principals WHERE alias = {{ sqlLiteral .SubjectID }}),
				NULL,
				{{ sqlBool .Enabled }},
				{{ sqlLiteral .Revision }}
			)
			ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_workspace_grants.revision < excluded.revision;
			{{ end }}
			{{ range .Groups }}
			INSERT INTO gatehouse_workspace_grants (workspace, role, principal, "group", enabled, revision)
			VALUES (
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}),
				{{ sqlLiteral .Role }},
				NULL,
				(SELECT id FROM gatehouse_groups WHERE workspace_id = (SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}) AND alias = {{ sqlLiteral .SubjectID }}),
				{{ sqlBool .Enabled }},
				{{ sqlLiteral .Revision }}
			)
			ON CONFLICT (workspace, role, "group") WHERE "group" IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_workspace_grants.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}
