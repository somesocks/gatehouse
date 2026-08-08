package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveToolsAndResourcesDefaultsEnabledAndSorts(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{{
			Id: "engineering",
			Tools: &[]configschema.GatehouseConfigWorkspacesValuesToolsValues{
				{Id: "zulu", Ref: "file:./tools/zulu.lisp"},
				{Id: "alpha", Ref: "file:./tools/alpha.lisp", Enabled: &disabled},
			},
			Resources: &[]configschema.GatehouseConfigWorkspacesValuesResourcesValues{
				{Id: "token", Ref: "env:GITHUB_TOKEN", Secret: true},
				{Id: "endpoint", Ref: "file:./resources/endpoint", Secret: false, Enabled: &disabled},
			},
		}},
	}

	err, tools := ResolveTools(document)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := []Tool{
		{WorkspaceID: "engineering", ID: "alpha", Ref: "file:./tools/alpha.lisp", Enabled: false},
		{WorkspaceID: "engineering", ID: "zulu", Ref: "file:./tools/zulu.lisp", Enabled: true},
	}
	if !reflect.DeepEqual(tools, wantTools) {
		t.Fatalf("ResolveTools() = %#v, want %#v", tools, wantTools)
	}

	err, resources := ResolveResources(document)
	if err != nil {
		t.Fatal(err)
	}
	wantResources := []Resource{
		{WorkspaceID: "engineering", ID: "endpoint", Ref: "file:./resources/endpoint", Secret: false, Enabled: false},
		{WorkspaceID: "engineering", ID: "token", Ref: "env:GITHUB_TOKEN", Secret: true, Enabled: true},
	}
	if !reflect.DeepEqual(resources, wantResources) {
		t.Fatalf("ResolveResources() = %#v, want %#v", resources, wantResources)
	}
}

func TestResolveToolsAndResourcesRejectInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		document configschema.GatehouseConfig
		resolve  func(configschema.GatehouseConfig) (error, any)
		contains string
	}{
		{
			name: "duplicate tool ID",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Tools: &[]configschema.GatehouseConfigWorkspacesValuesToolsValues{
					{Id: "github", Ref: "file:./github.lisp"},
					{Id: "github", Ref: "file:./other.lisp"},
				},
			}}},
			resolve: func(document configschema.GatehouseConfig) (error, any) {
				err, values := ResolveTools(document)
				return err, values
			},
			contains: "duplicated",
		},
		{
			name: "blank resource ref",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Workspaces: &[]configschema.GatehouseConfigWorkspacesValues{{
				Id: "engineering",
				Resources: &[]configschema.GatehouseConfigWorkspacesValuesResourcesValues{{
					Id: "token", Ref: " ", Secret: true,
				}},
			}}},
			resolve: func(document configschema.GatehouseConfig) (error, any) {
				err, values := ResolveResources(document)
				return err, values
			},
			contains: "must not be blank",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, _ := test.resolve(test.document)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("resolve() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestValidateFileRequiresResourceSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
workspaces:
  - id: engineering
    resources:
      - id: github-token
        ref: env:GITHUB_TOKEN
`), 0o600); err != nil {
		t.Fatal(err)
	}

	err, _ := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("ValidateFile() error = %v, want missing secret error", err)
	}
}
