package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveGroupsDefaultsEnabledAndSorts(t *testing.T) {
	disabled := false
	groupRevision, memberRevision := 7, 4
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{
			{
				Alias: "zebra",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{
					{Alias: "guests"},
					{
						Alias:    "admins",
						Name:     stringPointer("Administrators"),
						Enabled:  &disabled,
						Revision: &groupRevision,
						Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{
							{Principal: "zebra"},
							{Principal: "alpha", Enabled: &disabled, Revision: &memberRevision},
						},
					},
				},
			},
		},
	}

	err, groups := ResolveGroups(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{
		{
			WorkspaceID: "zebra",
			Alias:       "admins",
			Name:        stringPointer("Administrators"),
			Enabled:     false,
			Revision:    7,
			Members: []GroupMember{
				{PrincipalID: "alpha", Enabled: false, Revision: 4},
				{PrincipalID: "zebra", Enabled: true, Revision: 1},
			},
		},
		{
			WorkspaceID: "zebra",
			Alias:       "guests",
			Enabled:     true,
			Revision:    1,
			Members:     []GroupMember{},
		},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("ResolveGroups() = %#v, want %#v", groups, want)
	}
}

func TestResolveGroupsRejectsInvalidValues(t *testing.T) {
	zero, negative := 0, -1
	tests := []struct {
		name       string
		workspaces []configschema.GatehouseConfigWorkspacesValues
		contains   string
	}{
		{
			name:       "nonpositive group revision",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Alias: "engineering", Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{Alias: "admins", Revision: &zero}}}},
			contains:   "revision must be positive",
		},
		{
			name:       "nonpositive membership revision",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{Alias: "engineering", Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{Alias: "admins", Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{{Principal: "alice", Revision: &negative}}}}}},
			contains:   "revision must be positive",
		},
		{
			name: "invalid group alias",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Alias: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Alias: "Admins",
				}},
			}},
			contains: "must match",
		},
		{
			name: "duplicate group alias",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Alias: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{
					{Alias: "admins"},
					{Alias: "admins"},
				},
			}},
			contains: "duplicated",
		},
		{
			name: "blank group name",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Alias: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Alias: "admins",
					Name:  stringPointer(" "),
				}},
			}},
			contains: "must not be blank",
		},
		{
			name: "invalid member principal alias",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Alias: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Alias: "admins",
					Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{{
						Principal: "Alice",
					}},
				}},
			}},
			contains: "must match",
		},
		{
			name: "duplicate member principal alias",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Alias: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Alias: "admins",
					Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{
						{Principal: "alice"},
						{Principal: "alice"},
					},
				}},
			}},
			contains: "duplicated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := configschema.GatehouseConfig{ApiVersion: "v1", Workspaces: &test.workspaces}
			err, _ := ResolveGroups(document)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ResolveGroups() error = %v, want %q", err, test.contains)
			}
		})
	}
}
