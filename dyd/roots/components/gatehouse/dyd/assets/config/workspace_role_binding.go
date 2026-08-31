package config

import (
	"fmt"
	"sort"

	"gatehouse/authorization"
	"gatehouse/configschema"
)

type WorkspaceRoleBinding struct {
	WorkspaceID string
	Role        string
	PrincipalID *string
	GroupID     *string
	Revision    int
	Enabled     bool
}

func ResolveWorkspaceRoleBindings(document configschema.GatehouseConfig, principals []Principal, groups []Group) (error, []WorkspaceRoleBinding) {
	if document.Workspaces == nil {
		if document.Principals != nil {
			return nil, []WorkspaceRoleBinding{}
		}
		root := "root"
		return nil, []WorkspaceRoleBinding{{WorkspaceID: defaultWorkspaceAlias, Role: string(authorization.Manager), GroupID: &root, Revision: 1, Enabled: true}}
	}

	groupAliases := make(map[workspaceGroupAliasKey]struct{}, len(groups))
	for _, group := range groups {
		groupAliases[workspaceGroupAliasKey{Workspace: group.WorkspaceID, Alias: group.Alias}] = struct{}{}
	}

	bindings := make([]WorkspaceRoleBinding, 0)
	seen := map[workspaceRoleBindingKey]struct{}{}
	for workspaceIndex, workspace := range *document.Workspaces {
		if workspace.RoleBindings == nil {
			continue
		}
		for bindingIndex, configured := range *workspace.RoleBindings {
			path := fmt.Sprintf("workspaces[%d].role_bindings[%d]", workspaceIndex, bindingIndex)
			if !authorization.ValidRole(configured.Role) {
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
			binding := WorkspaceRoleBinding{WorkspaceID: workspace.Alias, Role: configured.Role, Revision: revision, Enabled: enabled}
			key := workspaceRoleBindingKey{Workspace: workspace.Alias, Role: configured.Role}
			if configured.Principal != nil {
				if !principalAlias.MatchString(*configured.Principal) {
					return fmt.Errorf("%s.principal must match %q", path, principalAlias.String()), nil
				}
				binding.PrincipalID = configured.Principal
				key.Principal = *configured.Principal
			} else {
				if _, exists := groupAliases[workspaceGroupAliasKey{Workspace: workspace.Alias, Alias: *configured.Group}]; !exists {
					return fmt.Errorf("%s.group %q is not configured in workspace %q", path, *configured.Group, workspace.Alias), nil
				}
				binding.GroupID = configured.Group
				key.Group = *configured.Group
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%s duplicates an earlier role binding", path), nil
			}
			seen[key] = struct{}{}
			bindings = append(bindings, binding)
		}
	}
	sort.Slice(bindings, func(left, right int) bool {
		leftBinding, rightBinding := bindings[left], bindings[right]
		if leftBinding.WorkspaceID != rightBinding.WorkspaceID {
			return leftBinding.WorkspaceID < rightBinding.WorkspaceID
		}
		if leftBinding.Role != rightBinding.Role {
			return leftBinding.Role < rightBinding.Role
		}
		return workspaceRoleBindingSubject(leftBinding) < workspaceRoleBindingSubject(rightBinding)
	})
	return nil, bindings
}

type workspaceRoleBindingKey struct {
	Workspace, Role, Principal, Group string
}

type workspaceGroupAliasKey struct {
	Workspace, Alias string
}

func workspaceRoleBindingSubject(binding WorkspaceRoleBinding) string {
	if binding.PrincipalID != nil {
		return "principal:" + *binding.PrincipalID
	}
	return "group:" + *binding.GroupID
}
