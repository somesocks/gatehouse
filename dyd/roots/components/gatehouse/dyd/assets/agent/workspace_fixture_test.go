package agent_test

import "gatehouse/config"

// Agent execution fixtures explicitly admit their requesting humans. Tests of
// revocation remove these grants after arranging the queued request.
func admittedAgentTestState(state config.State) config.State {
	for _, workspace := range state.Workspaces {
		for _, principal := range state.Principals {
			declared := false
			for _, grant := range state.WorkspaceGrants {
				if grant.WorkspaceID == workspace.Alias && grant.PrincipalID != nil && *grant.PrincipalID == principal.Alias {
					declared = true
				}
			}
			if !declared && principal.Enabled && workspace.Enabled {
				alias := principal.Alias
				state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{WorkspaceID: workspace.Alias, PrincipalID: &alias, Role: "guest", Revision: 1, Enabled: true})
			}
		}
	}
	return state
}
