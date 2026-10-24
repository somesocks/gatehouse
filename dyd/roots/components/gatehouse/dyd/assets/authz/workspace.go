// Package authz defines static authorization policies.
package authz

type Role string

const (
	Guest  Role = "guest"
	Member Role = "member"
	Admin  Role = "admin"
)

type WorkspaceAction string

const (
	WorkspaceEdit          WorkspaceAction = "workspace.edit"
	WorkspaceProjectCreate WorkspaceAction = "workspace.project.create"
	WorkspaceSessionCreate WorkspaceAction = "workspace.session.create"
	WorkspaceGroupCreate   WorkspaceAction = "workspace.group.create"
	WorkspaceGrantManage   WorkspaceAction = "workspace.grant.manage"
)

func ValidRole(role string) bool {
	switch Role(role) {
	case Guest, Member, Admin:
		return true
	default:
		return false
	}
}

func WorkspaceAllows(roles []Role, action WorkspaceAction) bool {
	if !validWorkspaceAction(action) {
		return false
	}
	for _, role := range roles {
		switch role {
		case Admin:
			return true
		case Member:
			switch action {
			case WorkspaceProjectCreate, WorkspaceSessionCreate:
				return true
			}
		}
	}
	return false
}

func validWorkspaceAction(action WorkspaceAction) bool {
	switch action {
	case WorkspaceEdit, WorkspaceProjectCreate, WorkspaceSessionCreate, WorkspaceGroupCreate, WorkspaceGrantManage:
		return true
	default:
		return false
	}
}
