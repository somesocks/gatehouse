package authz

type SystemAction string

const SystemManage SystemAction = "system.manage"

func SystemAllows(roles []Role, action SystemAction) bool {
	if action != SystemManage {
		return false
	}
	for _, role := range roles {
		if role == Manager {
			return true
		}
	}
	return false
}
