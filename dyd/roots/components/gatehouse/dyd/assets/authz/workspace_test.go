package authz

import "testing"

func TestWorkspaceAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		roles   []Role
		action  WorkspaceAction
		allowed bool
	}{
		{name: "member denied", roles: []Role{Member}, action: WorkspaceProjectCreate},
		{name: "contributor creates project", roles: []Role{Contributor}, action: WorkspaceProjectCreate, allowed: true},
		{name: "contributor creates session", roles: []Role{Contributor}, action: WorkspaceSessionCreate, allowed: true},
		{name: "contributor cannot edit", roles: []Role{Contributor}, action: WorkspaceEdit},
		{name: "manager edits", roles: []Role{Manager}, action: WorkspaceEdit, allowed: true},
		{name: "manager creates group", roles: []Role{Manager}, action: WorkspaceGroupCreate, allowed: true},
		{name: "manager manages grants", roles: []Role{Manager}, action: WorkspaceGrantManage, allowed: true},
		{name: "manager rejects unrelated action", roles: []Role{Manager}, action: "workspace.project.session.create"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := WorkspaceAllows(test.roles, test.action); got != test.allowed {
				t.Fatalf("WorkspaceAllows(%v, %q) = %t, want %t", test.roles, test.action, got, test.allowed)
			}
		})
	}
}
