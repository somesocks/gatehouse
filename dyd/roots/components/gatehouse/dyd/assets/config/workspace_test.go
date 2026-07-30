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
			{Key: "zebra", Name: "Zebra"},
			{Key: "alpha-2", Name: "Alpha", Enabled: &disabled},
		},
	}

	err, workspaces := ResolveWorkspaces(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []Workspace{
		{Key: "alpha-2", Name: "Alpha", Enabled: false},
		{Key: "zebra", Name: "Zebra", Enabled: true},
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
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Key: "Engineering", Name: "Engineering"}},
			contains:   "must match",
		},
		{
			name: "duplicate key",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{
				{Key: "engineering", Name: "Engineering"},
				{Key: "engineering", Name: "Engineering again"},
			},
			contains: "duplicated",
		},
		{
			name:       "blank name",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Key: "engineering", Name: " \t"}},
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
