package config

import (
	"fmt"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Tool struct {
	WorkspaceID string
	ID          string
	Ref         string
	Enabled     bool
}

type Resource struct {
	WorkspaceID string
	ID          string
	Source      string
	Secret      bool
	Enabled     bool
}

func ResolveTools(document configschema.GatehouseConfig) (error, []Tool) {
	if document.Workspaces == nil {
		return nil, nil
	}

	tools := make([]Tool, 0)
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Tools == nil {
			continue
		}
		if !workspaceID.MatchString(workspace.Id) || workspace.Id == gatehouseWorkspaceID {
			return fmt.Errorf("workspaces[%d].id is not a configurable workspace", workspaceIndex), nil
		}
		ids := make(map[string]struct{}, len(*workspace.Tools))
		for toolIndex, configured := range *workspace.Tools {
			if !workspaceID.MatchString(configured.Id) {
				return fmt.Errorf("workspaces[%d].tools[%d].id must match %q", workspaceIndex, toolIndex, workspaceID.String()), nil
			}
			if _, exists := ids[configured.Id]; exists {
				return fmt.Errorf("workspaces[%d].tools[%d].id %q is duplicated", workspaceIndex, toolIndex, configured.Id), nil
			}
			if strings.TrimSpace(configured.Ref) == "" {
				return fmt.Errorf("workspaces[%d].tools[%d].ref must not be blank", workspaceIndex, toolIndex), nil
			}

			enabled := true
			if configured.Enabled != nil {
				enabled = *configured.Enabled
			}
			ids[configured.Id] = struct{}{}
			tools = append(tools, Tool{WorkspaceID: workspace.Id, ID: configured.Id, Ref: configured.Ref, Enabled: enabled})
		}
	}
	sort.Slice(tools, func(left, right int) bool {
		if tools[left].WorkspaceID == tools[right].WorkspaceID {
			return tools[left].ID < tools[right].ID
		}
		return tools[left].WorkspaceID < tools[right].WorkspaceID
	})
	return nil, tools
}

func ResolveResources(document configschema.GatehouseConfig) (error, []Resource) {
	if document.Workspaces == nil {
		return nil, nil
	}

	resources := make([]Resource, 0)
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Resources == nil {
			continue
		}
		if !workspaceID.MatchString(workspace.Id) || workspace.Id == gatehouseWorkspaceID {
			return fmt.Errorf("workspaces[%d].id is not a configurable workspace", workspaceIndex), nil
		}
		ids := make(map[string]struct{}, len(*workspace.Resources))
		for resourceIndex, configured := range *workspace.Resources {
			if !workspaceID.MatchString(configured.Id) {
				return fmt.Errorf("workspaces[%d].resources[%d].id must match %q", workspaceIndex, resourceIndex, workspaceID.String()), nil
			}
			if _, exists := ids[configured.Id]; exists {
				return fmt.Errorf("workspaces[%d].resources[%d].id %q is duplicated", workspaceIndex, resourceIndex, configured.Id), nil
			}
			if strings.TrimSpace(configured.Source) == "" {
				return fmt.Errorf("workspaces[%d].resources[%d].source must not be blank", workspaceIndex, resourceIndex), nil
			}

			enabled := true
			if configured.Enabled != nil {
				enabled = *configured.Enabled
			}
			ids[configured.Id] = struct{}{}
			resources = append(resources, Resource{
				WorkspaceID: workspace.Id,
				ID:          configured.Id,
				Source:      configured.Source,
				Secret:      configured.Secret,
				Enabled:     enabled,
			})
		}
	}
	sort.Slice(resources, func(left, right int) bool {
		if resources[left].WorkspaceID == resources[right].WorkspaceID {
			return resources[left].ID < resources[right].ID
		}
		return resources[left].WorkspaceID < resources[right].WorkspaceID
	})
	return nil, resources
}
