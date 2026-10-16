package database

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/authz"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type SystemWorkspaceGrant struct {
	ID        string     `json:"id"`
	Workspace string     `json:"workspace"`
	Principal *string    `json:"principal,omitempty"`
	Group     *string    `json:"group,omitempty"`
	Role      authz.Role `json:"role"`
	Enabled   bool       `json:"enabled"`
	Revision  int        `json:"revision"`
}

// SystemWorkspaceGrantRoleUpdate edits a single grant, preserving its identity,
// recipient and enabled state. Role collisions include disabled grants.
func (store *Store) SystemWorkspaceGrantRoleUpdate(ctx context.Context, workspaceID, id string, role authz.Role, expectedRevision int) (error, *SystemWorkspaceGrant) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.WorkspaceGrant, id) {
		return nil, nil
	}
	if !authz.ValidRole(string(role)) {
		return ErrWorkspaceGrantRoleInvalid, nil
	}
	if expectedRevision < 1 {
		return ErrRevisionConflict, nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	var grant SystemWorkspaceGrant
	var principal, group sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, workspace, principal, "group", role, enabled, revision
		FROM gatehouse_workspace_grants WHERE workspace = `+p(1)+` AND id = `+p(2), workspaceID, id).Scan(&grant.ID, &grant.Workspace, &principal, &group, &grant.Role, &grant.Enabled, &grant.Revision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get workspace grant for role update: %w", err), nil
	}
	if principal.Valid {
		grant.Principal = &principal.String
	}
	if group.Valid {
		grant.Group = &group.String
	}
	if grant.Revision != expectedRevision {
		return ErrRevisionConflict, nil
	}
	if grant.Role == role {
		if err := tx.Commit(); err != nil {
			return err, nil
		}
		return nil, &grant
	}
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_workspace_grants SET role = `+p(1)+`, revision = revision + 1
		WHERE workspace = `+p(2)+` AND id = `+p(3)+` AND revision = `+p(4), role, workspaceID, id, expectedRevision)
	if uniquePrincipalConstraint(err) {
		return ErrWorkspaceGrantAlreadyExists, nil
	}
	if err != nil {
		return fmt.Errorf("update workspace grant role: %w", err), nil
	}
	if count, err := result.RowsAffected(); err != nil {
		return err, nil
	} else if count != 1 {
		return ErrRevisionConflict, nil
	}
	workspace := model.WorkspaceRef{Id: workspaceID}
	topics := []string{ActivityTopicWorkspaceGrant(workspace, id)}
	if grant.Principal != nil {
		topics = append(topics, ActivityTopicPrincipalGrant(model.PrincipalRef{Id: *grant.Principal}, id))
	} else {
		topics = append(topics, ActivityTopicGroupGrant(workspace, model.GroupRef{Id: *grant.Group}, id))
	}
	if err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{
		Event: "workspace_grant.update", ResourceKind: ActivityResourceKindWorkspaceGrant, ResourceWorkspaceGrant: &id,
	}, topics); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	grant.Role = role
	grant.Revision++
	return nil, &grant
}
