package authz

import "testing"

func TestSystemAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		roles   []Role
		action  SystemAction
		allowed bool
	}{
		{name: "member denied", roles: []Role{Member}, action: SystemManage},
		{name: "manager allowed", roles: []Role{Manager}, action: SystemManage, allowed: true},
		{name: "unknown action denied", roles: []Role{Manager}, action: "system.unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SystemAllows(test.roles, test.action); got != test.allowed {
				t.Fatalf("SystemAllows(%v, %q) = %t, want %t", test.roles, test.action, got, test.allowed)
			}
		})
	}
}
