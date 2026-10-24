package authz

import "testing"

func TestWorkspaceAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		roles   []Role
		action  WorkspaceAction
		allowed bool
	}{
		{name: "guest denied", roles: []Role{Guest}, action: WorkspaceProjectCreate},
		{name: "member creates project", roles: []Role{Member}, action: WorkspaceProjectCreate, allowed: true},
		{name: "member creates session", roles: []Role{Member}, action: WorkspaceSessionCreate, allowed: true},
		{name: "member cannot edit", roles: []Role{Member}, action: WorkspaceEdit},
		{name: "admin edits", roles: []Role{Admin}, action: WorkspaceEdit, allowed: true},
		{name: "admin creates group", roles: []Role{Admin}, action: WorkspaceGroupCreate, allowed: true},
		{name: "admin manages grants", roles: []Role{Admin}, action: WorkspaceGrantManage, allowed: true},
		{name: "admin rejects unrelated action", roles: []Role{Admin}, action: "workspace.project.session.create"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := WorkspaceAllows(test.roles, test.action); got != test.allowed {
				t.Fatalf("WorkspaceAllows(%v, %q) = %t, want %t", test.roles, test.action, got, test.allowed)
			}
		})
	}
}
