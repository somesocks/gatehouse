package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveWorkspacesDefaultsEnabledAndSortsByKey(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{
			{Key: "zebra"},
			{Key: "alpha-2", Name: stringPointer("Alpha"), Enabled: &disabled},
		},
	}

	err, workspaces := ResolveWorkspaces(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []Workspace{
		{Key: "alpha-2", Name: stringPointer("Alpha"), Enabled: false},
		{Key: "zebra", Name: nil, Enabled: true},
	}
	if !reflect.DeepEqual(workspaces, want) {
		t.Fatalf("ResolveWorkspaces() = %#v, want %#v", workspaces, want)
	}
}

func TestResolveWorkspacesRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name       string
		workspaces []configschema.GatehouseConfigWorkspacesValues
		contains   string
	}{
		{
			name:       "invalid key",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Key: "Engineering", Name: stringPointer("Engineering")}},
			contains:   ".key must match",
		},
		{
			name:       "reserved key",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Key: "gatehouse", Name: stringPointer("Gatehouse")}},
			contains:   ".key \"gatehouse\" is reserved",
		},
		{
			name: "duplicate key",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{
				{Key: "engineering", Name: stringPointer("Engineering")},
				{Key: "engineering", Name: stringPointer("Engineering again")},
			},
			contains: ".key \"engineering\" is duplicated",
		},
		{
			name:       "blank name",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Key: "engineering", Name: stringPointer(" \t")}},
			contains:   "must not be blank",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := configschema.GatehouseConfig{ApiVersion: "v1", Workspaces: &test.workspaces}
			err, _ := ResolveWorkspaces(document)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ResolveWorkspaces() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
