package config

import (
	"fmt"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Group struct {
	WorkspaceID      string
	Alias            string
	Name             *string
	Enabled          bool
	Members          []GroupMember
	ToolGrants       []GroupToolGrant
	ResourceGrants   []GroupResourceGrant
}

type GroupMember struct {
	PrincipalID string
	Enabled     bool
}

type GroupToolGrant struct {
	ToolID  string
	Enabled bool
}

type GroupResourceGrant struct {
	ResourceID string
	Enabled    bool
}

func ResolveGroups(document configschema.GatehouseConfig) (error, []Group) {
	if document.Workspaces == nil {
		if document.Principals == nil {
			return nil, []Group{{
				WorkspaceID: defaultWorkspaceAlias,
				Alias:       "root",
				Enabled:     true,
				Members:     []GroupMember{{PrincipalID: defaultPrincipalAlias, Enabled: true}},
			}}
		}
		return nil, nil
	}

	groups := make([]Group, 0)
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Groups == nil {
			continue
		}
		groupAliases := make(map[string]struct{}, len(*workspace.Groups))
		for groupIndex, configured := range *workspace.Groups {
			if !workspaceAlias.MatchString(workspace.Alias) || workspace.Alias == gatehouseWorkspaceAlias {
				return fmt.Errorf("workspaces[%d].alias is not a configurable workspace", workspaceIndex), nil
			}
			if !workspaceID.MatchString(configured.Alias) {
				return fmt.Errorf("workspaces[%d].groups[%d].alias must match %q", workspaceIndex, groupIndex, workspaceID.String()), nil
			}
			if _, exists := groupAliases[configured.Alias]; exists {
				return fmt.Errorf("workspaces[%d].groups[%d].alias %q is duplicated", workspaceIndex, groupIndex, configured.Alias), nil
			}
			if configured.Name != nil && strings.TrimSpace(*configured.Name) == "" {
				return fmt.Errorf("workspaces[%d].groups[%d].name must not be blank", workspaceIndex, groupIndex), nil
			}

			enabled := true
			if configured.Enabled != nil {
				enabled = *configured.Enabled
			}
			members := make([]GroupMember, 0)
			if configured.Members != nil {
				members = make([]GroupMember, 0, len(*configured.Members))
				memberIDs := make(map[string]struct{}, len(*configured.Members))
				for memberIndex, configuredMember := range *configured.Members {
					if !principalAlias.MatchString(configuredMember.Principal) {
						return fmt.Errorf("workspaces[%d].groups[%d].members[%d].principal must match %q", workspaceIndex, groupIndex, memberIndex, principalAlias.String()), nil
					}
					if _, exists := memberIDs[configuredMember.Principal]; exists {
						return fmt.Errorf("workspaces[%d].groups[%d].members[%d].principal %q is duplicated", workspaceIndex, groupIndex, memberIndex, configuredMember.Principal), nil
					}
					memberEnabled := true
					if configuredMember.Enabled != nil {
						memberEnabled = *configuredMember.Enabled
					}
					memberIDs[configuredMember.Principal] = struct{}{}
					members = append(members, GroupMember{PrincipalID: configuredMember.Principal, Enabled: memberEnabled})
				}
			}
			sort.Slice(members, func(left, right int) bool {
				return members[left].PrincipalID < members[right].PrincipalID
			})
			toolGrants := make([]GroupToolGrant, 0)
			if configured.ToolGrants != nil {
				toolGrants = make([]GroupToolGrant, 0, len(*configured.ToolGrants))
				toolIDs := make(map[string]struct{}, len(*configured.ToolGrants))
				for grantIndex, configuredGrant := range *configured.ToolGrants {
					if !workspaceID.MatchString(configuredGrant.Tool) {
						return fmt.Errorf("workspaces[%d].groups[%d].tool_grants[%d].tool must match %q", workspaceIndex, groupIndex, grantIndex, workspaceID.String()), nil
					}
					if _, exists := toolIDs[configuredGrant.Tool]; exists {
						return fmt.Errorf("workspaces[%d].groups[%d].tool_grants[%d].tool %q is duplicated", workspaceIndex, groupIndex, grantIndex, configuredGrant.Tool), nil
					}
					enabled := true
					if configuredGrant.Enabled != nil {
						enabled = *configuredGrant.Enabled
					}
					toolIDs[configuredGrant.Tool] = struct{}{}
					toolGrants = append(toolGrants, GroupToolGrant{ToolID: configuredGrant.Tool, Enabled: enabled})
				}
			}
			sort.Slice(toolGrants, func(left, right int) bool {
				return toolGrants[left].ToolID < toolGrants[right].ToolID
			})

			resourceGrants := make([]GroupResourceGrant, 0)
			if configured.ResourceGrants != nil {
				resourceGrants = make([]GroupResourceGrant, 0, len(*configured.ResourceGrants))
				resourceIDs := make(map[string]struct{}, len(*configured.ResourceGrants))
				for grantIndex, configuredGrant := range *configured.ResourceGrants {
					if !workspaceID.MatchString(configuredGrant.Resource) {
						return fmt.Errorf("workspaces[%d].groups[%d].resource_grants[%d].resource must match %q", workspaceIndex, groupIndex, grantIndex, workspaceID.String()), nil
					}
					if _, exists := resourceIDs[configuredGrant.Resource]; exists {
						return fmt.Errorf("workspaces[%d].groups[%d].resource_grants[%d].resource %q is duplicated", workspaceIndex, groupIndex, grantIndex, configuredGrant.Resource), nil
					}
					enabled := true
					if configuredGrant.Enabled != nil {
						enabled = *configuredGrant.Enabled
					}
					resourceIDs[configuredGrant.Resource] = struct{}{}
					resourceGrants = append(resourceGrants, GroupResourceGrant{ResourceID: configuredGrant.Resource, Enabled: enabled})
				}
			}
			sort.Slice(resourceGrants, func(left, right int) bool {
				return resourceGrants[left].ResourceID < resourceGrants[right].ResourceID
			})

			groupAliases[configured.Alias] = struct{}{}
			groups = append(groups, Group{
				WorkspaceID:    workspace.Alias,
				Alias:          configured.Alias,
				Name:           configured.Name,
				Enabled:        enabled,
				Members:        members,
				ToolGrants:     toolGrants,
				ResourceGrants: resourceGrants,
			})
		}
	}
	sort.Slice(groups, func(left, right int) bool {
		if groups[left].WorkspaceID == groups[right].WorkspaceID {
			return groups[left].Alias < groups[right].Alias
		}
		return groups[left].WorkspaceID < groups[right].WorkspaceID
	})
	return nil, groups
}
