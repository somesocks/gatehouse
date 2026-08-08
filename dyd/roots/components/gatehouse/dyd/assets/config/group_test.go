package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveGroupsDefaultsEnabledAndSorts(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{
			{
				Id: "zebra",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{
					{Id: "guests"},
					{
						Id:      "admins",
						Name:    stringPointer("Administrators"),
						Enabled: &disabled,
						Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{
							{Principal: "zebra"},
							{Principal: "alpha", Enabled: &disabled},
						},
						ToolGrants: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesToolGrantsValues{
							{Tool: "zulu"},
							{Tool: "alpha", Enabled: &disabled},
						},
						ResourceGrants: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesResourceGrantsValues{
							{Resource: "token"},
							{Resource: "endpoint", Enabled: &disabled},
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
			ID:          "admins",
			Name:        stringPointer("Administrators"),
			Enabled:     false,
			Members: []GroupMember{
				{PrincipalID: "alpha", Enabled: false},
				{PrincipalID: "zebra", Enabled: true},
			},
			ToolGrants: []GroupToolGrant{
				{ToolID: "alpha", Enabled: false},
				{ToolID: "zulu", Enabled: true},
			},
			ResourceGrants: []GroupResourceGrant{
				{ResourceID: "endpoint", Enabled: false},
				{ResourceID: "token", Enabled: true},
			},
		},
	{
		WorkspaceID:      "zebra",
		ID:               "guests",
		Enabled:          true,
		Members:          []GroupMember{},
		ToolGrants:       []GroupToolGrant{},
		ResourceGrants:   []GroupResourceGrant{},
	},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("ResolveGroups() = %#v, want %#v", groups, want)
	}
}

func TestResolveGroupsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name       string
		workspaces []configschema.GatehouseConfigWorkspacesValues
		contains   string
	}{
		{
			name: "invalid group ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Id: "Admins",
				}},
			}},
			contains: "must match",
		},
		{
			name: "duplicate group ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{
					{Id: "admins"},
					{Id: "admins"},
				},
			}},
			contains: "duplicated",
		},
		{
			name: "blank group name",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Id:   "admins",
					Name: stringPointer(" "),
				}},
			}},
			contains: "must not be blank",
		},
		{
			name: "invalid member principal ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Id: "admins",
					Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{{
						Principal: "Alice",
					}},
				}},
			}},
			contains: "must match",
		},
		{
			name: "duplicate member principal ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Id: "admins",
					Members: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesMembersValues{
						{Principal: "alice"},
						{Principal: "alice"},
					},
				}},
			}},
			contains: "duplicated",
		},
		{
			name: "duplicate tool grant",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Id: "admins",
					ToolGrants: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesToolGrantsValues{
						{Tool: "github"},
						{Tool: "github"},
					},
				}},
			}},
			contains: "duplicated",
		},
		{
			name: "invalid resource grant ID",
			workspaces: []configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{
					Id: "admins",
					ResourceGrants: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValuesResourceGrantsValues{{
						Resource: "GitHub",
					}},
				}},
			}},
			contains: "must match",
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
