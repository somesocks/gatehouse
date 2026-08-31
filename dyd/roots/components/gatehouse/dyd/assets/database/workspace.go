package database

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/authz"
	"gatehouse/model"
)

func (store *Store) WorkspacesGet(ctx context.Context, principal model.PrincipalRef) (error, []model.Workspace) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH input AS (SELECT `+placeholder(1)+` AS principal)
		SELECT workspaces.id, workspaces.alias, workspaces.name, workspaces.enabled
		FROM gatehouse_workspaces AS workspaces
		CROSS JOIN input
		WHERE workspaces.enabled = TRUE
			AND (
				EXISTS (
					SELECT 1
					FROM gatehouse_workspace_grants AS grants
					JOIN gatehouse_principals AS principals ON principals.id = grants.principal
					WHERE grants.workspace = workspaces.id
						AND grants.principal = input.principal
						AND grants.enabled = TRUE
						AND principals.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1
					FROM gatehouse_workspace_grants AS grants
					JOIN gatehouse_groups AS groups
						ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members
						ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
					WHERE grants.workspace = workspaces.id
						AND members.principal_id = input.principal
						AND grants.enabled = TRUE
						AND groups.enabled = TRUE
						AND members.enabled = TRUE
						AND principals.enabled = TRUE
				)
			)
		ORDER BY workspaces.name IS NULL, workspaces.name, workspaces.id
	`, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspaces: %w", err), nil
	}
	defer rows.Close()

	workspaces := []model.Workspace{}
	for rows.Next() {
		var workspace model.Workspace
		var alias, name sql.NullString
		if err := rows.Scan(&workspace.Ref.Id, &alias, &name, &workspace.Enabled); err != nil {
			return fmt.Errorf("scan workspace: %w", err), nil
		}
		if name.Valid {
			workspace.Name = &name.String
		}
		if alias.Valid {
			workspace.Alias = &alias.String
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspaces: %w", err), nil
	}
	return nil, workspaces
}

func (store *Store) WorkspaceRolesGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []authz.Role) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH input AS (
			SELECT `+placeholder(1)+` AS workspace, `+placeholder(2)+` AS principal
		)
		SELECT role FROM (
			SELECT grants.role
			FROM gatehouse_workspace_grants AS grants
			CROSS JOIN input
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = grants.workspace
			JOIN gatehouse_principals AS principals ON principals.id = grants.principal
			WHERE grants.workspace = input.workspace
				AND grants.principal = input.principal
				AND workspaces.enabled = TRUE
				AND grants.enabled = TRUE
				AND principals.enabled = TRUE
			UNION
			SELECT grants.role
			FROM gatehouse_workspace_grants AS grants
			CROSS JOIN input
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = grants.workspace
			JOIN gatehouse_groups AS groups
				ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
			JOIN gatehouse_group_members AS members
				ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
			JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
			WHERE grants.workspace = input.workspace
				AND members.principal_id = input.principal
				AND workspaces.enabled = TRUE
				AND grants.enabled = TRUE
				AND groups.enabled = TRUE
				AND members.enabled = TRUE
				AND principals.enabled = TRUE
		)
		ORDER BY role
	`, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspace roles: %w", err), nil
	}
	defer rows.Close()

	roles := []authz.Role{}
	for rows.Next() {
		var role authz.Role
		if err := rows.Scan(&role); err != nil {
			return fmt.Errorf("scan workspace role: %w", err), nil
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace roles: %w", err), nil
	}
	return nil, roles
}

func (store *Store) WorkspaceGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, *model.Workspace) {
	err, workspaces := store.WorkspacesGet(ctx, principal)
	if err != nil {
		return err, nil
	}
	for _, candidate := range workspaces {
		if candidate.Ref == workspace {
			return nil, &candidate
		}
	}
	return nil, nil
}

func (store *Store) WorkspaceRefGetByAlias(ctx context.Context, alias string) (error, *model.WorkspaceRef) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT id FROM gatehouse_workspaces WHERE alias = `+placeholder(1), alias)
	var workspace model.WorkspaceRef
	if err := row.Scan(&workspace.Id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get workspace by alias: %w", err), nil
	}
	return nil, &workspace
}

func (store *Store) WorkspaceGroupsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Group) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT groups.workspace_id, groups.id, groups.alias, groups.name, groups.enabled
		FROM gatehouse_groups AS groups
		JOIN gatehouse_group_members AS members
			ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
		JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
		WHERE groups.workspace_id = `+placeholder(1)+`
			AND groups.enabled = TRUE
			AND principals.id = `+placeholder(2)+`
			AND principals.enabled = TRUE
			AND members.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = groups.workspace_id AND workspaces.enabled = TRUE
			)
		ORDER BY groups.id
	`, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspace groups: %w", err), nil
	}
	defer rows.Close()

	groups := []model.Group{}
	for rows.Next() {
		var group model.Group
		var alias, name sql.NullString
		if err := rows.Scan(&group.Ref.Workspace.Id, &group.Ref.Id, &alias, &name, &group.Enabled); err != nil {
			return fmt.Errorf("scan workspace group: %w", err), nil
		}
		if name.Valid {
			group.Name = &name.String
		}
		if alias.Valid {
			group.Alias = &alias.String
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace groups: %w", err), nil
	}
	return nil, groups
}
