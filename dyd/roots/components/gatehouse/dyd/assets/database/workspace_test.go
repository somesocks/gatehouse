package database

import (
	"context"
	"reflect"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
)

func TestWorkspaceToolsAndResourcesGet(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{ID: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{ID: "alice", Enabled: true},
			{ID: "bob", Enabled: false},
		},
		Tools: []config.Tool{
			{WorkspaceID: "engineering", ID: "git", Source: "file:./git.lisp", Enabled: true},
			{WorkspaceID: "engineering", ID: "legacy", Source: "file:./legacy.lisp", Enabled: false},
		},
		Resources: []config.Resource{
			{WorkspaceID: "engineering", ID: "docs", Source: "file:./docs", Secret: false, Enabled: true},
			{WorkspaceID: "engineering", ID: "token", Source: "env:TOKEN", Secret: true, Enabled: true},
			{WorkspaceID: "engineering", ID: "retired", Source: "env:RETIRED", Secret: true, Enabled: false},
		},
		Groups: []config.Group{
			{
				WorkspaceID: "engineering",
				ID:          "developers",
				Enabled:     true,
				Members: []config.GroupMember{
					{PrincipalID: "alice", Enabled: true},
					{PrincipalID: "bob", Enabled: true},
				},
				ToolGrants: []config.GroupToolGrant{{ToolID: "git", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{
					{ResourceID: "token", Enabled: true},
					{ResourceID: "retired", Enabled: true},
				},
			},
			{
				WorkspaceID: "engineering",
				ID:          "reviewers",
				Enabled:     true,
				Members:     []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
				ToolGrants:  []config.GroupToolGrant{{ToolID: "git", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{
					{ResourceID: "docs", Enabled: true},
				},
			},
			{
				WorkspaceID: "engineering",
				ID:          "disabled",
				Enabled:     false,
				Members:     []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{
					{ResourceID: "docs", Enabled: true},
				},
			},
		},
	}
	err, migrations := BuildMigrations(configuration, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), store, migrations); err != nil {
		t.Fatal(err)
	}

	workspace := model.WorkspaceRef{Id: "engineering"}
	alice := model.PrincipalRef{Id: "alice"}
	err, tools := store.WorkspaceToolsGet(context.Background(), workspace, alice)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := []model.Tool{{
		Ref:     model.ToolRef{Workspace: workspace, Id: "git"},
		Source:  "file:./git.lisp",
		Enabled: true,
	}}
	if !reflect.DeepEqual(tools, wantTools) {
		t.Fatalf("WorkspaceToolsGet() = %#v, want %#v", tools, wantTools)
	}
	err, resources := store.WorkspaceResourcesGet(context.Background(), workspace, alice)
	if err != nil {
		t.Fatal(err)
	}
	wantResources := []model.Resource{
		{Ref: model.ResourceRef{Workspace: workspace, Id: "docs"}, Source: "file:./docs", Secret: false, Enabled: true},
		{Ref: model.ResourceRef{Workspace: workspace, Id: "token"}, Source: "env:TOKEN", Secret: true, Enabled: true},
	}
	if !reflect.DeepEqual(resources, wantResources) {
		t.Fatalf("WorkspaceResourcesGet() = %#v, want %#v", resources, wantResources)
	}

	bob := model.PrincipalRef{Id: "bob"}
	err, tools = store.WorkspaceToolsGet(context.Background(), workspace, bob)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("WorkspaceToolsGet() for disabled principal = %#v, want no tools", tools)
	}
	if _, err := store.Exec(`UPDATE gatehouse_workspaces SET enabled = FALSE WHERE id = 'engineering'`); err != nil {
		t.Fatal(err)
	}
	err, resources = store.WorkspaceResourcesGet(context.Background(), workspace, alice)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("WorkspaceResourcesGet() for disabled workspace = %#v, want no resources", resources)
	}
}
