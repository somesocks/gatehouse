package authz

import "testing"

func TestProjectAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		roles   []Role
		action  ProjectAction
		allowed bool
	}{
		{name: "member denied", roles: []Role{Member}, action: ProjectNoteCreate},
		{name: "contributor edits note", roles: []Role{Contributor}, action: ProjectNoteEdit, allowed: true},
		{name: "contributor creates task", roles: []Role{Contributor}, action: ProjectTaskCreate, allowed: true},
		{name: "contributor removes task", roles: []Role{Contributor}, action: ProjectTaskRemove, allowed: true},
		{name: "contributor creates session", roles: []Role{Contributor}, action: ProjectSessionCreate, allowed: true},
		{name: "contributor cannot edit project", roles: []Role{Contributor}, action: ProjectEdit},
		{name: "manager edits project", roles: []Role{Manager}, action: ProjectEdit, allowed: true},
		{name: "manager manages grants", roles: []Role{Manager}, action: ProjectGrantManage, allowed: true},
		{name: "manager rejects session action", roles: []Role{Manager}, action: "session.edit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ProjectAllows(test.roles, test.action); got != test.allowed {
				t.Fatalf("ProjectAllows(%v, %q) = %t, want %t", test.roles, test.action, got, test.allowed)
			}
		})
	}
}

func TestSessionAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		roles   []Role
		action  SessionAction
		allowed bool
	}{
		{name: "member denied", roles: []Role{Member}, action: SessionMessageCreate},
		{name: "contributor creates message", roles: []Role{Contributor}, action: SessionMessageCreate, allowed: true},
		{name: "contributor edits task", roles: []Role{Contributor}, action: SessionTaskEdit, allowed: true},
		{name: "contributor removes task", roles: []Role{Contributor}, action: SessionTaskRemove, allowed: true},
		{name: "contributor responds to approval", roles: []Role{Contributor}, action: SessionApprovalRespond, allowed: true},
		{name: "contributor cannot set project", roles: []Role{Contributor}, action: SessionProjectSet},
		{name: "manager sets project", roles: []Role{Manager}, action: SessionProjectSet, allowed: true},
		{name: "manager manages grants", roles: []Role{Manager}, action: SessionGrantManage, allowed: true},
		{name: "manager rejects project action", roles: []Role{Manager}, action: "project.edit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SessionAllows(test.roles, test.action); got != test.allowed {
				t.Fatalf("SessionAllows(%v, %q) = %t, want %t", test.roles, test.action, got, test.allowed)
			}
		})
	}
}
