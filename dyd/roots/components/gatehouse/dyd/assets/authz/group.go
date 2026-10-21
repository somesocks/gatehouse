package authz

type GroupAction string

const (
	GroupEdit             GroupAction = "group.edit"
	GroupMembershipManage GroupAction = "group.membership.manage"
)

func ValidGroupRole(role string) bool {
	return Role(role) == Member || Role(role) == Manager
}

func GroupAllows(roles []Role, action GroupAction) bool {
	if action != GroupEdit && action != GroupMembershipManage {
		return false
	}
	for _, role := range roles {
		if role == Manager {
			return true
		}
	}
	return false
}
