package authz

import "testing"

func TestProjectAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		roles   []Role
		action  ProjectAction
		allowed bool
	}{
		{name: "guest denied", roles: []Role{Guest}, action: ProjectNoteCreate},
		{name: "member edits note", roles: []Role{Member}, action: ProjectNoteEdit, allowed: true},
		{name: "member creates task", roles: []Role{Member}, action: ProjectTaskCreate, allowed: true},
		{name: "member removes task", roles: []Role{Member}, action: ProjectTaskRemove, allowed: true},
		{name: "member updates file", roles: []Role{Member}, action: ProjectFileUpdate, allowed: true},
		{name: "guest cannot update file", roles: []Role{Guest}, action: ProjectFileUpdate},
		{name: "member creates session", roles: []Role{Member}, action: ProjectSessionCreate, allowed: true},
		{name: "member cannot edit project", roles: []Role{Member}, action: ProjectEdit},
		{name: "admin edits project", roles: []Role{Admin}, action: ProjectEdit, allowed: true},
		{name: "admin manages grants", roles: []Role{Admin}, action: ProjectGrantManage, allowed: true},
		{name: "admin rejects session action", roles: []Role{Admin}, action: "session.edit"},
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
		{name: "guest denied", roles: []Role{Guest}, action: SessionMessageCreate},
		{name: "member creates message", roles: []Role{Member}, action: SessionMessageCreate, allowed: true},
		{name: "member edits task", roles: []Role{Member}, action: SessionTaskEdit, allowed: true},
		{name: "member removes task", roles: []Role{Member}, action: SessionTaskRemove, allowed: true},
		{name: "member updates file", roles: []Role{Member}, action: SessionFileUpdate, allowed: true},
		{name: "guest cannot update file", roles: []Role{Guest}, action: SessionFileUpdate},
		{name: "member responds to approval", roles: []Role{Member}, action: SessionApprovalRespond, allowed: true},
		{name: "member responds to input", roles: []Role{Member}, action: SessionInputRespond, allowed: true},
		{name: "guest cannot respond to input", roles: []Role{Guest}, action: SessionInputRespond},
		{name: "admin responds to input", roles: []Role{Admin}, action: SessionInputRespond, allowed: true},
		{name: "member cannot set project", roles: []Role{Member}, action: SessionProjectSet},
		{name: "admin sets project", roles: []Role{Admin}, action: SessionProjectSet, allowed: true},
		{name: "admin manages grants", roles: []Role{Admin}, action: SessionGrantManage, allowed: true},
		{name: "admin rejects project action", roles: []Role{Admin}, action: "project.edit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SessionAllows(test.roles, test.action); got != test.allowed {
				t.Fatalf("SessionAllows(%v, %q) = %t, want %t", test.roles, test.action, got, test.allowed)
			}
		})
	}
}
