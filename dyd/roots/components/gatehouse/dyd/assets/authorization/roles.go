// Package authorization defines the static authorization vocabulary.
package authorization

type Role string

const (
	Member      Role = "member"
	Contributor Role = "contributor"
	Manager     Role = "manager"
)

func ValidRole(role string) bool {
	switch Role(role) {
	case Member, Contributor, Manager:
		return true
	default:
		return false
	}
}
