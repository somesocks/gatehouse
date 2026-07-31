package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveWorkspacesDefaultsEnabledAndSortsByID(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{
			{Id: "zebra"},
			{Id: "alpha-2", Name: stringPointer("Alpha"), Enabled: &disabled},
		},
	}

	err, workspaces := ResolveWorkspaces(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []Workspace{
		{ID: "alpha-2", Name: stringPointer("Alpha"), Enabled: false},
		{ID: "zebra", Name: nil, Enabled: true},
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
			name:       "invalid ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Id: "Engineering", Name: stringPointer("Engineering")}},
			contains:   "must match",
		},
		{
			name:       "reserved ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Id: "gatehouse", Name: stringPointer("Gatehouse")}},
			contains:   "is reserved",
		},
		{
			name: "duplicate ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{
				{Id: "engineering", Name: stringPointer("Engineering")},
				{Id: "engineering", Name: stringPointer("Engineering again")},
			},
			contains: "duplicated",
		},
		{
			name:       "blank name",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Id: "engineering", Name: stringPointer(" \t")}},
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
