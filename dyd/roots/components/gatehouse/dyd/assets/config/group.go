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
}

type GroupMember struct {
	PrincipalID string
	Enabled     bool
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
			groupAliases[configured.Alias] = struct{}{}
			groups = append(groups, Group{
				WorkspaceID:    workspace.Alias,
				Alias:          configured.Alias,
				Name:           configured.Name,
				Enabled:        enabled,
				Members:     members,
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
