package config

import (
	"fmt"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Tool struct {
	WorkspaceID string
	Alias       string
	Source      string
	Enabled     bool
}

type Resource struct {
	WorkspaceID string
	Alias       string
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
		if !workspaceAlias.MatchString(workspace.Alias) || workspace.Alias == gatehouseWorkspaceAlias {
			return fmt.Errorf("workspaces[%d].alias is not a configurable workspace", workspaceIndex), nil
		}
		aliases := make(map[string]struct{}, len(*workspace.Tools))
		for toolIndex, configured := range *workspace.Tools {
			if !workspaceID.MatchString(configured.Alias) {
				return fmt.Errorf("workspaces[%d].tools[%d].alias must match %q", workspaceIndex, toolIndex, workspaceID.String()), nil
			}
			if _, exists := aliases[configured.Alias]; exists {
				return fmt.Errorf("workspaces[%d].tools[%d].alias %q is duplicated", workspaceIndex, toolIndex, configured.Alias), nil
			}
			if strings.TrimSpace(configured.Source) == "" {
				return fmt.Errorf("workspaces[%d].tools[%d].source must not be blank", workspaceIndex, toolIndex), nil
			}

			enabled := true
			if configured.Enabled != nil {
				enabled = *configured.Enabled
			}
			aliases[configured.Alias] = struct{}{}
			tools = append(tools, Tool{WorkspaceID: workspace.Alias, Alias: configured.Alias, Source: configured.Source, Enabled: enabled})
		}
	}
	sort.Slice(tools, func(left, right int) bool {
		if tools[left].WorkspaceID == tools[right].WorkspaceID {
			return tools[left].Alias < tools[right].Alias
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
		if !workspaceAlias.MatchString(workspace.Alias) || workspace.Alias == gatehouseWorkspaceAlias {
			return fmt.Errorf("workspaces[%d].alias is not a configurable workspace", workspaceIndex), nil
		}
		aliases := make(map[string]struct{}, len(*workspace.Resources))
		for resourceIndex, configured := range *workspace.Resources {
			if !workspaceID.MatchString(configured.Alias) {
				return fmt.Errorf("workspaces[%d].resources[%d].alias must match %q", workspaceIndex, resourceIndex, workspaceID.String()), nil
			}
			if _, exists := aliases[configured.Alias]; exists {
				return fmt.Errorf("workspaces[%d].resources[%d].alias %q is duplicated", workspaceIndex, resourceIndex, configured.Alias), nil
			}
			if strings.TrimSpace(configured.Source) == "" {
				return fmt.Errorf("workspaces[%d].resources[%d].source must not be blank", workspaceIndex, resourceIndex), nil
			}

			enabled := true
			if configured.Enabled != nil {
				enabled = *configured.Enabled
			}
			aliases[configured.Alias] = struct{}{}
			resources = append(resources, Resource{
				WorkspaceID: workspace.Alias,
				Alias:       configured.Alias,
				Source:      configured.Source,
				Secret:      configured.Secret,
				Enabled:     enabled,
			})
		}
	}
	sort.Slice(resources, func(left, right int) bool {
		if resources[left].WorkspaceID == resources[right].WorkspaceID {
			return resources[left].Alias < resources[right].Alias
		}
		return resources[left].WorkspaceID < resources[right].WorkspaceID
	})
	return nil, resources
}
