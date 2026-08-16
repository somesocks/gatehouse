package database

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/model"
)

type WorkspaceResourceSummary struct {
	ID     string
	Secret bool
}

func (store *Store) WorkspacesGet(ctx context.Context, principal model.PrincipalRef) (error, []model.Workspace) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT workspaces.id, workspaces.key, workspaces.name, workspaces.enabled
		FROM gatehouse_workspaces AS workspaces
		WHERE workspaces.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_group_members AS members
				JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
				JOIN gatehouse_groups AS groups
					ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
				WHERE members.workspace_id = workspaces.id
					AND principals.id = `+placeholder(1)+`
					AND principals.enabled = TRUE
					AND members.enabled = TRUE
					AND groups.enabled = TRUE
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
		var key, name sql.NullString
		if err := rows.Scan(&workspace.Ref.Id, &key, &name, &workspace.Enabled); err != nil {
			return fmt.Errorf("scan workspace: %w", err), nil
		}
		if name.Valid {
			workspace.Name = &name.String
		}
		if key.Valid {
			workspace.Key = &key.String
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspaces: %w", err), nil
	}
	return nil, workspaces
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

func (store *Store) WorkspaceRefGetByKey(ctx context.Context, key string) (error, *model.WorkspaceRef) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT id FROM gatehouse_workspaces WHERE key = `+placeholder(1), key)
	var workspace model.WorkspaceRef
	if err := row.Scan(&workspace.Id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get workspace by key: %w", err), nil
	}
	return nil, &workspace
}

func (store *Store) WorkspaceGroupsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Group) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT groups.workspace_id, groups.id, groups.name, groups.enabled
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
		var name sql.NullString
		if err := rows.Scan(&group.Ref.Workspace.Id, &group.Ref.Id, &name, &group.Enabled); err != nil {
			return fmt.Errorf("scan workspace group: %w", err), nil
		}
		if name.Valid {
			group.Name = &name.String
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace groups: %w", err), nil
	}
	return nil, groups
}

func (store *Store) WorkspaceToolIDsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []string) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT tools.id
		FROM gatehouse_tools AS tools
		WHERE tools.workspace_id = `+placeholder(1)+`
			AND tools.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_group_members AS members
				JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
				JOIN gatehouse_groups AS groups
					ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
				JOIN gatehouse_group_tool_grants AS grants
					ON grants.workspace_id = groups.workspace_id AND grants.group_id = groups.id
				WHERE members.workspace_id = tools.workspace_id
					AND principals.id = `+placeholder(2)+`
					AND principals.enabled = TRUE
					AND members.enabled = TRUE
					AND groups.enabled = TRUE
					AND grants.tool_id = tools.id
					AND grants.enabled = TRUE
			)
		ORDER BY tools.id
	`, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspace tool IDs: %w", err), nil
	}
	defer rows.Close()

	tools := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan workspace tool ID: %w", err), nil
		}
		tools = append(tools, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace tool IDs: %w", err), nil
	}
	return nil, tools
}

func (store *Store) WorkspaceResourceSummariesGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []WorkspaceResourceSummary) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT resources.id, resources.secret
		FROM gatehouse_resources AS resources
		WHERE resources.workspace_id = `+placeholder(1)+`
			AND resources.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_group_members AS members
				JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
				JOIN gatehouse_groups AS groups
					ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
				JOIN gatehouse_group_resource_grants AS grants
					ON grants.workspace_id = groups.workspace_id AND grants.group_id = groups.id
				WHERE members.workspace_id = resources.workspace_id
					AND principals.id = `+placeholder(2)+`
					AND principals.enabled = TRUE
					AND members.enabled = TRUE
					AND groups.enabled = TRUE
					AND grants.resource_id = resources.id
					AND grants.enabled = TRUE
			)
		ORDER BY resources.id
	`, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspace resource summaries: %w", err), nil
	}
	defer rows.Close()

	resources := []WorkspaceResourceSummary{}
	for rows.Next() {
		var resource WorkspaceResourceSummary
		if err := rows.Scan(&resource.ID, &resource.Secret); err != nil {
			return fmt.Errorf("scan workspace resource summary: %w", err), nil
		}
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace resource summaries: %w", err), nil
	}
	return nil, resources
}

func (store *Store) WorkspaceToolsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Tool) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT tools.workspace_id, tools.id, tools.source, tools.enabled
		FROM gatehouse_tools AS tools
		WHERE tools.workspace_id = `+placeholder(1)+`
			AND tools.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = tools.workspace_id AND workspaces.enabled = TRUE
			)
			AND EXISTS (
				SELECT 1
				FROM gatehouse_group_members AS members
				JOIN gatehouse_principals AS principals
					ON principals.id = members.principal_id
				JOIN gatehouse_groups AS groups
					ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
				JOIN gatehouse_group_tool_grants AS grants
					ON grants.workspace_id = groups.workspace_id AND grants.group_id = groups.id
				WHERE members.workspace_id = tools.workspace_id
					AND principals.id = `+placeholder(2)+`
					AND principals.enabled = TRUE
					AND members.enabled = TRUE
					AND groups.enabled = TRUE
					AND grants.tool_id = tools.id
					AND grants.enabled = TRUE
			)
		ORDER BY tools.id
	`, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspace tools: %w", err), nil
	}
	defer rows.Close()

	tools := []model.Tool{}
	for rows.Next() {
		var tool model.Tool
		if err := rows.Scan(&tool.Ref.Workspace.Id, &tool.Ref.Id, &tool.Source, &tool.Enabled); err != nil {
			return fmt.Errorf("scan workspace tool: %w", err), nil
		}
		tools = append(tools, tool)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace tools: %w", err), nil
	}
	return nil, tools
}

func (store *Store) WorkspaceResourcesGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Resource) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT resources.workspace_id, resources.id, resources.source, resources.secret, resources.enabled
		FROM gatehouse_resources AS resources
		WHERE resources.workspace_id = `+placeholder(1)+`
			AND resources.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = resources.workspace_id AND workspaces.enabled = TRUE
			)
			AND EXISTS (
				SELECT 1
				FROM gatehouse_group_members AS members
				JOIN gatehouse_principals AS principals
					ON principals.id = members.principal_id
				JOIN gatehouse_groups AS groups
					ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
				JOIN gatehouse_group_resource_grants AS grants
					ON grants.workspace_id = groups.workspace_id AND grants.group_id = groups.id
				WHERE members.workspace_id = resources.workspace_id
					AND principals.id = `+placeholder(2)+`
					AND principals.enabled = TRUE
					AND members.enabled = TRUE
					AND groups.enabled = TRUE
					AND grants.resource_id = resources.id
					AND grants.enabled = TRUE
			)
		ORDER BY resources.id
	`, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get workspace resources: %w", err), nil
	}
	defer rows.Close()

	resources := []model.Resource{}
	for rows.Next() {
		var resource model.Resource
		if err := rows.Scan(&resource.Ref.Workspace.Id, &resource.Ref.Id, &resource.Source, &resource.Secret, &resource.Enabled); err != nil {
			return fmt.Errorf("scan workspace resource: %w", err), nil
		}
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace resources: %w", err), nil
	}
	return nil, resources
}
