package database_test

import "gatehouse/config"

// Resource-authorization fixtures explicitly enroll their enabled principals
// before assigning resource grants. Admission-specific tests do not use this.
func withTestWorkspaceMembers(state config.State) config.State {
	for _, workspace := range state.Workspaces {
		if !workspace.Enabled {
			continue
		}
		for _, principal := range state.Principals {
			if !principal.Enabled {
				continue
			}
			declared := false
			for _, grant := range state.WorkspaceGrants {
				if grant.WorkspaceID == workspace.Alias && grant.PrincipalID != nil && *grant.PrincipalID == principal.Alias {
					declared = true
					break
				}
			}
			if !declared {
				state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{WorkspaceID: workspace.Alias, PrincipalID: stringPointer(principal.Alias), Role: "guest", Enabled: true, Revision: 1})
			}
		}
	}
	return state
}
