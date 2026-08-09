package database

import (
	"context"
	"fmt"

	"gatehouse/model"
)

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
