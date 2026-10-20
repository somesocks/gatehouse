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
	Role      authz.Role `json:"role"`
	Enabled   bool       `json:"enabled"`
	Revision  int        `json:"revision"`
}

// SystemWorkspaceGrantRoleUpdate edits a single grant, preserving its identity,
// recipient and enabled state. Role collisions include disabled grants.
func (store *Store) SystemWorkspaceGrantRoleUpdate(ctx context.Context, workspaceID, id string, role authz.Role, expectedRevision int) (error, *SystemWorkspaceGrant) {
	return store.SystemWorkspaceGrantUpdate(ctx, workspaceID, id, &role, nil, expectedRevision)
}

func (store *Store) SystemWorkspaceGrantUpdate(ctx context.Context, workspaceID, id string, role *authz.Role, enabled *bool, expectedRevision int) (error, *SystemWorkspaceGrant) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.WorkspaceGrant, id) {
		return nil, nil
	}
	if role != nil && !authz.ValidRole(string(*role)) {
		return ErrWorkspaceGrantRoleInvalid, nil
	}
	if expectedRevision < 1 {
		return ErrRevisionConflict, nil
	}
	if role == nil && enabled == nil {
		return fmt.Errorf("%w: an update is required", ErrGroupInput), nil
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
	if !principal.Valid || group.Valid {
		return nil, nil
	}
	grant.Principal = &principal.String
	if grant.Revision != expectedRevision {
		return ErrRevisionConflict, nil
	}
	nextRole, nextEnabled := grant.Role, grant.Enabled
	if role != nil {
		nextRole = *role
	}
	if enabled != nil {
		nextEnabled = *enabled
	}
	if grant.Role == nextRole && grant.Enabled == nextEnabled {
		if err := tx.Commit(); err != nil {
			return err, nil
		}
		return nil, &grant
	}
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_workspace_grants SET role = `+p(1)+`, enabled = `+p(2)+`, revision = revision + 1
		WHERE workspace = `+p(3)+` AND id = `+p(4)+` AND revision = `+p(5), nextRole, nextEnabled, workspaceID, id, expectedRevision)
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
	if err := store.workspaceGrantActivity(ctx, tx, grant, "workspace_grant.update"); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	grant.Role = nextRole
	grant.Enabled = nextEnabled
	grant.Revision++
	return nil, &grant
}

func (store *Store) workspaceGrantActivity(ctx context.Context, tx *sql.Tx, grant SystemWorkspaceGrant, event string) error {
	workspace := model.WorkspaceRef{Id: grant.Workspace}
	topics := []string{ActivityTopicWorkspaceGrant(workspace, grant.ID)}
	if grant.Principal != nil {
		topics = append(topics, ActivityTopicPrincipalGrant(model.PrincipalRef{Id: *grant.Principal}, grant.ID))
	}
	err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindWorkspaceGrant, ResourceWorkspaceGrant: &grant.ID}, topics)
	return err
}
