// Package authz defines static authorization policies.
package authz

type Role string

const (
	Member      Role = "member"
	Contributor Role = "contributor"
	Manager     Role = "manager"
)

type WorkspaceAction string

const (
	WorkspaceEdit              WorkspaceAction = "workspace.edit"
	WorkspaceProjectCreate     WorkspaceAction = "workspace.project.create"
	WorkspaceSessionCreate     WorkspaceAction = "workspace.session.create"
	WorkspaceGroupCreate       WorkspaceAction = "workspace.group.create"
	WorkspaceGrantManage       WorkspaceAction = "workspace.grant.manage"
)

func ValidRole(role string) bool {
	switch Role(role) {
	case Member, Contributor, Manager:
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
		case Manager:
			return true
		case Contributor:
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
