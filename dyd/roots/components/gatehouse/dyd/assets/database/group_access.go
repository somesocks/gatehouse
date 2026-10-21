package database

import (
	"context"
	"database/sql"
	"errors"

	"gatehouse/authz"
	"gatehouse/model"
)

var ErrGroupForbidden = errors.New("group management permission is required")

// GroupRoleGet returns the caller's current role in a visible workspace group.
func (store *Store) GroupRoleGet(ctx context.Context, workspace, group string, principal model.PrincipalRef) (error, authz.Role) {
	p := store.sessionParticipantPlaceholder
	var role authz.Role
	err := store.QueryRowContext(ctx, `SELECT members.role FROM gatehouse_group_members members
		JOIN gatehouse_groups groups ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
		WHERE members.workspace_id = `+p(1)+` AND members.group_id = `+p(2)+` AND members.principal_id = `+p(3)+`
		AND members.enabled = TRUE AND groups.enabled = TRUE
		AND `+workspacePrincipalAdmissionSQL("members.workspace_id", "members.principal_id"), workspace, group, principal.Id).Scan(&role)
	if err == sql.ErrNoRows {
		return nil, ""
	}
	return err, role
}

func (store *Store) groupMutationAllowed(ctx context.Context, tx *sql.Tx, workspace, group string, actor *model.PrincipalRef) error {
	if actor == nil {
		// System administration has already authorized the request.
		return nil
	}
	p := store.sessionParticipantPlaceholder
	var allowed bool
	query := workspacePrincipalAdmissionSQL(p(1), p(2))
	args := []any{workspace, actor.Id}
	if group == "" {
		query += ` AND EXISTS (SELECT 1 FROM gatehouse_workspace_grants WHERE workspace = ` + p(1) + ` AND principal = ` + p(2) + ` AND role = 'manager' AND enabled = TRUE)`
	} else {
		query += ` AND EXISTS (SELECT 1 FROM gatehouse_group_members members
			JOIN gatehouse_groups groups ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
			WHERE members.workspace_id = ` + p(1) + ` AND members.principal_id = ` + p(2) + ` AND members.group_id = ` + p(3) + ` AND members.role = 'manager' AND members.enabled = TRUE AND groups.enabled = TRUE)`
		args = append(args, group)
	}
	if err := tx.QueryRowContext(ctx, `SELECT `+query, args...).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrGroupForbidden
	}
	return nil
}

func (store *Store) WorkspaceGroupDirectoryGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, pages ...CollectionPage) (error, []SystemGroup) {
	p := keychainPlaceholder(store.kind)
	suffix, args, err := store.collectionPageSQL(pages, "groups.id", []string{"groups.name", "groups.alias"}, []any{workspace.Id, principal.Id}, "groups.id")
	if err != nil {
		return err, nil
	}
	rows, err := store.QueryContext(ctx, `SELECT groups.id, groups.workspace_id, groups.alias, groups.name, groups.enabled, groups.revision, members.role
		FROM gatehouse_groups groups JOIN gatehouse_group_members members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
		WHERE groups.workspace_id = `+p(1)+` AND members.principal_id = `+p(2)+` AND members.enabled = TRUE
		AND groups.enabled = TRUE
		AND `+workspacePrincipalAdmissionSQL("members.workspace_id", "members.principal_id")+suffix, args...)
	if err != nil {
		return err, nil
	}
	defer rows.Close()
	groups := []SystemGroup{}
	for rows.Next() {
		var group SystemGroup
		var alias, name sql.NullString
		if err := rows.Scan(&group.ID, &group.Workspace, &alias, &name, &group.Enabled, &group.Revision, &group.Role); err != nil {
			return err, nil
		}
		if alias.Valid {
			group.Alias = &alias.String
		}
		if name.Valid {
			group.Name = &name.String
		}
		groups = append(groups, group)
	}
	return rows.Err(), groups
}
