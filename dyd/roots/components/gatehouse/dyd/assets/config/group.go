package config

import (
	"fmt"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Group struct {
	WorkspaceID string
	Alias       string
	Name        *string
	Enabled     bool
	Revision    int
	Members     []GroupMember
	Agents      []GroupAgent
}

type GroupMember struct {
	PrincipalID string
	Enabled     bool
	Revision    int
}

type GroupAgent struct {
	AgentID  string
	Enabled  bool
	Revision int
}

func ResolveGroups(document configschema.GatehouseConfig) (error, []Group) {
	if document.Workspaces == nil {
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
			revision := 1
			if configured.Revision != nil {
				revision = *configured.Revision
			}
			if revision < 1 {
				return fmt.Errorf("workspaces[%d].groups[%d].revision must be positive", workspaceIndex, groupIndex), nil
			}
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
					memberRevision := 1
					if configuredMember.Revision != nil {
						memberRevision = *configuredMember.Revision
					}
					if memberRevision < 1 {
						return fmt.Errorf("workspaces[%d].groups[%d].members[%d].revision must be positive", workspaceIndex, groupIndex, memberIndex), nil
					}
					if configuredMember.Enabled != nil {
						memberEnabled = *configuredMember.Enabled
					}
					memberIDs[configuredMember.Principal] = struct{}{}
					members = append(members, GroupMember{PrincipalID: configuredMember.Principal, Enabled: memberEnabled, Revision: memberRevision})
				}
			}
			sort.Slice(members, func(left, right int) bool {
				return members[left].PrincipalID < members[right].PrincipalID
			})
			var agents []GroupAgent
			if configured.Agents != nil {
				agentAliases := make(map[string]bool, len(*configured.Agents))
				for agentIndex, configuredAgent := range *configured.Agents {
					if !workspaceAgentAlias.MatchString(configuredAgent.Agent) {
						return fmt.Errorf("workspaces[%d].groups[%d].agents[%d].agent is invalid", workspaceIndex, groupIndex, agentIndex), nil
					}
					if agentAliases[configuredAgent.Agent] {
						return fmt.Errorf("workspaces[%d].groups[%d].agents[%d].agent %q is duplicated", workspaceIndex, groupIndex, agentIndex, configuredAgent.Agent), nil
					}
					agentRevision := 1
					if configuredAgent.Revision != nil {
						agentRevision = *configuredAgent.Revision
					}
					if agentRevision < 1 {
						return fmt.Errorf("workspaces[%d].groups[%d].agents[%d].revision must be positive", workspaceIndex, groupIndex, agentIndex), nil
					}
					agentAliases[configuredAgent.Agent] = true
					agents = append(agents, GroupAgent{AgentID: configuredAgent.Agent, Enabled: configuredAgent.Enabled == nil || *configuredAgent.Enabled, Revision: agentRevision})
				}
			}
			sort.Slice(agents, func(left, right int) bool { return agents[left].AgentID < agents[right].AgentID })
			groupAliases[configured.Alias] = struct{}{}
			groups = append(groups, Group{
				WorkspaceID: workspace.Alias,
				Alias:       configured.Alias,
				Name:        configured.Name,
				Enabled:     enabled,
				Revision:    revision,
				Members:     members,
				Agents:      agents,
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
