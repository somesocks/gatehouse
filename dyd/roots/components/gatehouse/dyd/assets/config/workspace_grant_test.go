package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveWorkspaceGrantsDefaultsAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
principals:
  - alias: alice
workspaces:
  - alias: zebra
    groups:
      - alias: operators
    grants:
      - role: admin
        principal: alice
      - role: guest
        principal: bob
        revision: 2
        enabled: false
      - role: member
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
	want := []WorkspaceGrant{
		{WorkspaceID: "zebra", Role: "admin", PrincipalID: stringPointer("alice"), Revision: 1, Enabled: true},
		{WorkspaceID: "zebra", Role: "guest", PrincipalID: stringPointer("bob"), Revision: 2, Enabled: disabled},
		{WorkspaceID: "zebra", Role: "member", PrincipalID: stringPointer("alice"), Revision: 1, Enabled: true},
	}
	if !reflect.DeepEqual(state.WorkspaceGrants, want) {
		t.Fatalf("WorkspaceGrants = %#v, want %#v", state.WorkspaceGrants, want)
	}
}

func TestResolveWorkspaceGrantsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		bindings string
		contains string
	}{
		{
			name: "unknown role",
			bindings: `
      - role: owner
        principal: alice`,
			contains: ".role must be guest, member, or admin",
		}, {
			name: "missing subject",
			bindings: `
      - role: member`,
			contains: "principal",
		}, {
			name: "multiple subjects",
			bindings: `
      - role: member
        principal: alice
        group: members`,
			contains: "group",
		}, {
			name: "invalid principal alias",
			bindings: `
      - role: member
        principal: Bob`,
			contains: "principal",
		}, {
			name: "unknown group",
			bindings: `
      - role: member
        group: operators`,
			contains: "group",
		}, {
			name: "invalid revision",
			bindings: `
      - role: member
        principal: alice
        revision: 0`,
			contains: ".revision must be positive",
		}, {
			name: "duplicate grant",
			bindings: `
      - role: member
        principal: alice
      - role: member
        principal: alice`,
			contains: "duplicates an earlier grant",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gatehouse.yaml")
			contents := "api_version: v1\nprincipals:\n  - alias: alice\nworkspaces:\n  - alias: engineering\n    groups:\n      - alias: members\n    grants:\n" + test.bindings + "\n"
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

func TestResolveWorkspaceGrantsRejectsRoleBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
workspaces:
  - alias: engineering
    role_bindings: []
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err, _ := ValidateFile(path); err == nil {
		t.Fatal("ValidateFile() accepted obsolete role_bindings")
	}
}
