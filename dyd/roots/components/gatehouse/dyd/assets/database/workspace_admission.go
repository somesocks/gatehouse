package database

import (
	"context"
	"gatehouse/model"
)

// Workspace admission is explicit and independent of group/resource grants.
// Arguments are trusted SQL expressions, never request text.
func workspacePrincipalAdmissionSQL(workspace, principal string) string {
	return `EXISTS (SELECT 1 FROM gatehouse_workspace_grants admission_grant
		JOIN gatehouse_principals admission_principal ON admission_principal.id = admission_grant.principal AND admission_principal.enabled = TRUE
		JOIN gatehouse_workspaces admission_workspace ON admission_workspace.id = admission_grant.workspace AND admission_workspace.enabled = TRUE
		WHERE admission_grant.workspace = ` + workspace + ` AND admission_grant.principal = ` + principal + ` AND admission_grant.enabled = TRUE)`
}

func (store *Store) WorkspaceAdmissionGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, bool) {
	p := store.sessionParticipantPlaceholder
	var admitted bool
	err := store.QueryRowContext(ctx, `SELECT `+workspacePrincipalAdmissionSQL(p(1), p(2)), workspace.Id, principal.Id).Scan(&admitted)
	return err, admitted
}
