package config

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestValidateGroupAgentConfigurationFiles(t *testing.T) {
	for _, test := range []struct {
		name, yaml, json, contains string
		want                       []GroupAgent
	}{
		{name: "defaults", yaml: "[{agent: assistant}]", json: `[{"agent":"assistant"}]`, want: []GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 1}}},
		{name: "disabled and revisions", yaml: "[{agent: zebra/assistant, revision: 2}, {agent: alpha, enabled: false, revision: 4}]", json: `[{"agent":"zebra/assistant","revision":2},{"agent":"alpha","enabled":false,"revision":4}]`, want: []GroupAgent{{AgentID: "alpha", Enabled: false, Revision: 4}, {AgentID: "zebra/assistant", Enabled: true, Revision: 2}}},
		{name: "empty", yaml: "[]", json: "[]"},
		{name: "missing agent", yaml: "[{enabled: true}]", json: `[{"enabled":true}]`, contains: "missing property 'agent'"},
		{name: "unknown field", yaml: "[{agent: assistant, unexpected: true}]", json: `[{"agent":"assistant","unexpected":true}]`, contains: "additional properties 'unexpected'"},
		{name: "duplicate", yaml: "[{agent: assistant}, {agent: assistant}]", json: `[{"agent":"assistant"},{"agent":"assistant"}]`, contains: "duplicated"},
		{name: "zero revision", yaml: "[{agent: assistant, revision: 0}]", json: `[{"agent":"assistant","revision":0}]`, contains: "revision must be positive"},
		{name: "negative revision", yaml: "[{agent: assistant, revision: -1}]", json: `[{"agent":"assistant","revision":-1}]`, contains: "revision must be positive"},
		{name: "fractional revision", yaml: "[{agent: assistant, revision: 1.5}]", json: `[{"agent":"assistant","revision":1.5}]`, contains: "integer"},
		{name: "invalid enabled type", yaml: `[{agent: assistant, enabled: "false"}]`, json: `[{"agent":"assistant","enabled":"false"}]`, contains: "boolean"},
	} {
		for _, extension := range []string{"yaml", "json"} {
			t.Run(test.name+"/"+extension, func(t *testing.T) {
				contents := fmt.Sprintf("api_version: v1\nworkspaces:\n  - alias: engineering\n    groups:\n      - alias: developers\n        agents: %s\n", test.yaml)
				if extension == "json" {
					contents = fmt.Sprintf(`{"api_version":"v1","workspaces":[{"alias":"engineering","groups":[{"alias":"developers","agents":%s}]}]}`, test.json)
				}
				path := filepath.Join(t.TempDir(), "groups."+extension)
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
				err, document := ValidateFile(path)
				if test.contains != "" {
					if err == nil || !strings.Contains(err.Error(), test.contains) {
						t.Fatalf("ValidateFile() error = %v, want %q", err, test.contains)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				err, state := ResolveState(document)
				if err != nil || len(state.Groups) != 1 {
					t.Fatalf("resolved state = %#v, %v", state.Groups, err)
				}
				if !reflect.DeepEqual(state.Groups[0].Agents, test.want) {
					t.Fatalf("resolved bot memberships = %#v, want %#v", state.Groups[0].Agents, test.want)
				}
			})
		}
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

func TestResolveGroupAgents(t *testing.T) {
	disabled, revision := false, 4
	for _, test := range []struct {
		name     string
		agents   []configschema.GatehouseConfigWorkspacesValuesGroupsValuesAgentsValues
		contains string
	}{
		{name: "defaults and slash aliases", agents: []configschema.GatehouseConfigWorkspacesValuesGroupsValuesAgentsValues{{Agent: "zebra/assistant"}, {Agent: "alpha", Enabled: &disabled, Revision: &revision}}},
		{name: "duplicate", agents: []configschema.GatehouseConfigWorkspacesValuesGroupsValuesAgentsValues{{Agent: "assistant"}, {Agent: "assistant"}}, contains: "duplicated"},
		{name: "invalid alias", agents: []configschema.GatehouseConfigWorkspacesValuesGroupsValuesAgentsValues{{Agent: "Bad Alias"}}, contains: "agent is invalid"},
		{name: "blank alias", agents: []configschema.GatehouseConfigWorkspacesValuesGroupsValuesAgentsValues{{Agent: ""}}, contains: "agent is invalid"},
		{name: "revision", agents: []configschema.GatehouseConfigWorkspacesValuesGroupsValuesAgentsValues{{Agent: "assistant", Revision: new(int)}}, contains: "revision must be positive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := configschema.GatehouseConfig{Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{{Alias: "engineering", Groups: &[]configschema.GatehouseConfigWorkspacesValuesGroupsValues{{Alias: "developers", Agents: &test.agents}}}}}
			err, groups := ResolveGroups(document)
			if test.contains != "" {
				if err == nil || !strings.Contains(err.Error(), test.contains) {
					t.Fatalf("resolve error = %v, want %q", err, test.contains)
				}
				return
			}
			if err != nil || len(groups) != 1 {
				t.Fatalf("resolve groups = %#v %v", groups, err)
			}
			want := []GroupAgent{{AgentID: "alpha", Enabled: false, Revision: 4}, {AgentID: "zebra/assistant", Enabled: true, Revision: 1}}
			if !reflect.DeepEqual(groups[0].Agents, want) {
				t.Fatalf("bot memberships = %#v, want %#v", groups[0].Agents, want)
			}
		})
	}
}
