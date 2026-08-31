package config

import (
	"fmt"
	"sort"

	"gatehouse/authz"
	"gatehouse/configschema"
)

type WorkspaceGrant struct {
	WorkspaceID string
	Role        string
	PrincipalID *string
	GroupID     *string
	Revision    int
	Enabled     bool
}

func ResolveWorkspaceGrants(document configschema.GatehouseConfig, principals []Principal, groups []Group) (error, []WorkspaceGrant) {
	if document.Workspaces == nil {
		if document.Principals != nil {
			return nil, []WorkspaceGrant{}
		}
		root := "root"
		return nil, []WorkspaceGrant{{WorkspaceID: defaultWorkspaceAlias, Role: string(authz.Manager), GroupID: &root, Revision: 1, Enabled: true}}
	}

	groupAliases := make(map[workspaceGroupAliasKey]struct{}, len(groups))
	for _, group := range groups {
		groupAliases[workspaceGroupAliasKey{Workspace: group.WorkspaceID, Alias: group.Alias}] = struct{}{}
	}

	grants := make([]WorkspaceGrant, 0)
	seen := map[workspaceGrantKey]struct{}{}
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.Grants == nil {
			continue
		}
		for grantIndex, configured := range *workspace.Grants {
			path := fmt.Sprintf("workspaces[%d].grants[%d]", workspaceIndex, grantIndex)
			if !authz.ValidRole(configured.Role) {
				return fmt.Errorf("%s.role must be member, contributor, or manager", path), nil
			}
			if configured.Principal == nil && configured.Group == nil || configured.Principal != nil && configured.Group != nil {
				return fmt.Errorf("%s must specify exactly one of principal or group", path), nil
			}
			revision := 1
			if configured.Revision != nil {
				revision = *configured.Revision
			}
			if revision < 1 {
				return fmt.Errorf("%s.revision must be positive", path), nil
			}
			enabled := true
			if configured.Enabled != nil {
				enabled = *configured.Enabled
			}
			grant := WorkspaceGrant{WorkspaceID: workspace.Alias, Role: configured.Role, Revision: revision, Enabled: enabled}
			key := workspaceGrantKey{Workspace: workspace.Alias, Role: configured.Role}
			if configured.Principal != nil {
				if !principalAlias.MatchString(*configured.Principal) {
					return fmt.Errorf("%s.principal must match %q", path, principalAlias.String()), nil
				}
				grant.PrincipalID = configured.Principal
				key.Principal = *configured.Principal
			} else {
				if _, exists := groupAliases[workspaceGroupAliasKey{Workspace: workspace.Alias, Alias: *configured.Group}]; !exists {
					return fmt.Errorf("%s.group %q is not configured in workspace %q", path, *configured.Group, workspace.Alias), nil
				}
				grant.GroupID = configured.Group
				key.Group = *configured.Group
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%s duplicates an earlier grant", path), nil
			}
			seen[key] = struct{}{}
			grants = append(grants, grant)
		}
	}
	sort.Slice(grants, func(left, right int) bool {
		leftGrant, rightGrant := grants[left], grants[right]
		if leftGrant.WorkspaceID != rightGrant.WorkspaceID {
			return leftGrant.WorkspaceID < rightGrant.WorkspaceID
		}
		if leftGrant.Role != rightGrant.Role {
			return leftGrant.Role < rightGrant.Role
		}
		return workspaceGrantSubject(leftGrant) < workspaceGrantSubject(rightGrant)
	})
	return nil, grants
}

type workspaceGrantKey struct {
	Workspace, Role, Principal, Group string
}

type workspaceGroupAliasKey struct {
	Workspace, Alias string
}

func workspaceGrantSubject(grant WorkspaceGrant) string {
	if grant.PrincipalID != nil {
		return "principal:" + *grant.PrincipalID
	}
	return "group:" + *grant.GroupID
}
