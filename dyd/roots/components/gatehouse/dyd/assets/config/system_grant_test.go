package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveSystemGrantsDefaultsAndPatches(t *testing.T) {
	tests := []struct {
		name, contents string
		want           []SystemGrant
	}{
		{name: "default root", contents: "api_version: v1\n", want: []SystemGrant{{PrincipalID: "root", Revision: 1, Enabled: true}}},
		{name: "explicit empty", contents: "api_version: v1\nsystem_grants: []\n", want: []SystemGrant{}},
		{name: "configured", contents: "api_version: v1\nprincipals:\n  - alias: bob\n  - alias: alice\nsystem_grants:\n  - principal: bob\n    revision: 2\n    enabled: false\n  - principal: alice\n", want: []SystemGrant{{PrincipalID: "alice", Revision: 1, Enabled: true}, {PrincipalID: "bob", Revision: 2, Enabled: false}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gatehouse.yaml")
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil { t.Fatal(err) }
			err, document := ValidateFile(path)
			if err != nil { t.Fatal(err) }
			err, state := ResolveState(document)
			if err != nil { t.Fatal(err) }
			if !reflect.DeepEqual(state.SystemGrants, test.want) { t.Fatalf("SystemGrants = %#v, want %#v", state.SystemGrants, test.want) }
		})
	}
}

func TestResolveSystemGrantsRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct{ contents, contains string }{
		{"system_grants:\n  - principal: missing\n", "is not configured"},
		{"system_grants:\n  - principal: root\n    revision: 0\n", "revision must be positive"},
		{"system_grants:\n  - principal: root\n  - principal: root\n", "is duplicated"},
	} {
		path := filepath.Join(t.TempDir(), "gatehouse.yaml")
		if err := os.WriteFile(path, []byte("api_version: v1\n"+test.contents), 0o600); err != nil { t.Fatal(err) }
		if err, _ := ValidateFile(path); err == nil || !strings.Contains(err.Error(), test.contains) { t.Fatalf("ValidateFile() error = %v, want %q", err, test.contains) }
	}
}
