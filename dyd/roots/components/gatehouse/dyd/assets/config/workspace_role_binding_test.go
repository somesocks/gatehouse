package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveWorkspaceRoleBindingsDefaultsAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
principals:
  - alias: alice
workspaces:
  - alias: zebra
    groups:
      - alias: operators
    role_bindings:
      - role: manager
        group: operators
      - role: member
        principal: bob
        revision: 2
        enabled: false
      - role: contributor
        principal: alice
`), 0o600); err != nil {
		t.Fatal(err)
	}
	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, state := ResolveState(document)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	want := []WorkspaceRoleBinding{
		{WorkspaceID: "zebra", Role: "contributor", PrincipalID: stringPointer("alice"), Revision: 1, Enabled: true},
		{WorkspaceID: "zebra", Role: "manager", GroupID: stringPointer("operators"), Revision: 1, Enabled: true},
		{WorkspaceID: "zebra", Role: "member", PrincipalID: stringPointer("bob"), Revision: 2, Enabled: disabled},
	}
	if !reflect.DeepEqual(state.WorkspaceRoleBindings, want) {
		t.Fatalf("WorkspaceRoleBindings = %#v, want %#v", state.WorkspaceRoleBindings, want)
	}
}

func TestResolveWorkspaceRoleBindingsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		bindings string
		contains string
	}{
		{
			name: "unknown role",
			bindings: `
      - role: owner
        group: members`,
			contains: ".role must be member, contributor, or manager",
		}, {
			name: "missing subject",
			bindings: `
      - role: member`,
			contains: "must specify exactly one of principal or group",
		}, {
			name: "multiple subjects",
			bindings: `
      - role: member
        principal: alice
        group: members`,
			contains: "must specify exactly one of principal or group",
		}, {
			name: "invalid principal alias",
			bindings: `
      - role: member
        principal: Bob`,
			contains: ".principal must match",
		}, {
			name: "unknown group",
			bindings: `
      - role: member
        group: operators`,
			contains: ".group \"operators\" is not configured",
		}, {
			name: "invalid revision",
			bindings: `
      - role: member
        group: members
        revision: 0`,
			contains: ".revision must be positive",
		}, {
			name: "duplicate binding",
			bindings: `
      - role: member
        group: members
      - role: member
        group: members`,
			contains: "duplicates an earlier role binding",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gatehouse.yaml")
			contents := "api_version: v1\nprincipals:\n  - alias: alice\nworkspaces:\n  - alias: engineering\n    groups:\n      - alias: members\n    role_bindings:\n" + test.bindings + "\n"
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			err, _ := ValidateFile(path)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ValidateFile() error = %v, want %q", err, test.contains)
			}
		})
	}
}
