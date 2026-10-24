package authz

type GroupAction string

const (
	GroupEdit             GroupAction = "group.edit"
	GroupMembershipManage GroupAction = "group.membership.manage"
	GroupSecretCreate     GroupAction = "group.secret.create"
	GroupSecretEdit       GroupAction = "group.secret.edit"
	GroupSecretRemove     GroupAction = "group.secret.remove"
)

func ValidGroupRole(role string) bool {
	return Role(role) == Member || Role(role) == Admin
}

func GroupAllows(roles []Role, action GroupAction) bool {
	if action != GroupEdit && action != GroupMembershipManage && action != GroupSecretCreate && action != GroupSecretEdit && action != GroupSecretRemove {
		return false
	}
	for _, role := range roles {
		if role == Admin {
			return true
		}
	}
	return false
}
