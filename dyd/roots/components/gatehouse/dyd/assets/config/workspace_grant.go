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
	Revision    int
	Enabled     bool
}

func ResolveWorkspaceGrants(document configschema.GatehouseConfig) (error, []WorkspaceGrant) {
	if document.Workspaces == nil {
		if document.Principals != nil {
			return nil, []WorkspaceGrant{}
		}
		root := defaultPrincipalAlias
		return nil, []WorkspaceGrant{{WorkspaceID: defaultWorkspaceAlias, Role: string(authz.Admin), PrincipalID: &root, Revision: 1, Enabled: true}}
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
				return fmt.Errorf("%s.role must be guest, member, or admin", path), nil
			}
			if !principalAlias.MatchString(configured.Principal) {
				return fmt.Errorf("%s.principal must match %q", path, principalAlias.String()), nil
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
			grant.PrincipalID = &configured.Principal
			key.Principal = configured.Principal
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
		return *leftGrant.PrincipalID < *rightGrant.PrincipalID
	})
	return nil, grants
}

type workspaceGrantKey struct {
	Workspace, Role, Principal string
}
