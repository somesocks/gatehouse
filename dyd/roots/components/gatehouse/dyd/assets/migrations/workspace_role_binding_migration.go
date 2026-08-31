package migrations

import (
	"context"

	"gatehouse/config"
)

type workspaceRoleBindingMigrationValue struct {
	WorkspaceID string
	Role        string
	SubjectID   string
	Revision    int
	Enabled     bool
}

type workspaceRoleBindingMigrationValues struct {
	Principals []workspaceRoleBindingMigrationValue
	Groups     []workspaceRoleBindingMigrationValue
}

func workspaceRoleBindingMigrationBuilder(bindings []config.WorkspaceRoleBinding) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		values := workspaceRoleBindingMigrationValues{
			Principals: make([]workspaceRoleBindingMigrationValue, 0, len(bindings)),
			Groups:     make([]workspaceRoleBindingMigrationValue, 0, len(bindings)),
		}
		for _, binding := range bindings {
			value := workspaceRoleBindingMigrationValue{WorkspaceID: binding.WorkspaceID, Role: binding.Role, Revision: binding.Revision, Enabled: binding.Enabled}
			if binding.PrincipalID != nil {
				value.SubjectID = *binding.PrincipalID
				values.Principals = append(values.Principals, value)
			} else {
				value.SubjectID = *binding.GroupID
				values.Groups = append(values.Groups, value)
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Principals }}
			INSERT INTO gatehouse_workspace_role_bindings (workspace, role, principal, "group", enabled, revision)
			VALUES (
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}),
				{{ sqlLiteral .Role }},
				(SELECT id FROM gatehouse_principals WHERE alias = {{ sqlLiteral .SubjectID }}),
				NULL,
				{{ sqlBool .Enabled }},
				{{ sqlLiteral .Revision }}
			)
			ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_workspace_role_bindings.revision < excluded.revision;
			{{ end }}
			{{ range .Groups }}
			INSERT INTO gatehouse_workspace_role_bindings (workspace, role, principal, "group", enabled, revision)
			VALUES (
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}),
				{{ sqlLiteral .Role }},
				NULL,
				(SELECT id FROM gatehouse_groups WHERE workspace_id = (SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}) AND alias = {{ sqlLiteral .SubjectID }}),
				{{ sqlBool .Enabled }},
				{{ sqlLiteral .Revision }}
			)
			ON CONFLICT (workspace, role, "group") WHERE "group" IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_workspace_role_bindings.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}
