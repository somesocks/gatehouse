package database_test

import (
	"context"
	"reflect"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestWorkspaceToolsAndResourcesGet(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{Alias: "alice", Enabled: true},
			{Alias: "bob", Enabled: false},
		},
		Tools: []config.Tool{
			{WorkspaceID: "engineering", Alias: "git", Source: "file:./git.lisp", Enabled: true},
			{WorkspaceID: "engineering", Alias: "legacy", Source: "file:./legacy.lisp", Enabled: false},
		},
		Resources: []config.Resource{
			{WorkspaceID: "engineering", Alias: "docs", Source: "file:./docs", Secret: false, Enabled: true},
			{WorkspaceID: "engineering", Alias: "token", Source: "env:TOKEN", Secret: true, Enabled: true},
			{WorkspaceID: "engineering", Alias: "retired", Source: "env:RETIRED", Secret: true, Enabled: false},
		},
		Groups: []config.Group{
			{
				WorkspaceID: "engineering",
				Alias:       "developers",
				Enabled:     true,
				Members: []config.GroupMember{
					{PrincipalID: "alice", Enabled: true},
					{PrincipalID: "bob", Enabled: true},
				},
				ToolGrants: []config.GroupToolGrant{{ToolAlias: "git", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{
					{ResourceAlias: "token", Enabled: true},
					{ResourceAlias: "retired", Enabled: true},
				},
			},
			{
				WorkspaceID: "engineering",
				Alias:       "reviewers",
				Enabled:     true,
				Members:     []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
				ToolGrants:  []config.GroupToolGrant{{ToolAlias: "git", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{
					{ResourceAlias: "docs", Enabled: true},
				},
			},
			{
				WorkspaceID: "engineering",
				Alias:       "disabled",
				Enabled:     false,
				Members:     []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
				ResourceGrants: []config.GroupResourceGrant{
					{ResourceAlias: "docs", Enabled: true},
				},
			},
		},
	}
	if err := migrateState(context.Background(), store, configuration, state); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, context.Background(), store, "engineering")
	gitAlias := "git"
	docsAlias := "docs"
	tokenAlias := "token"
	var gitID, docsID, tokenID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_tools WHERE workspace_id = ? AND alias = ?`, workspace.Id, gitAlias).Scan(&gitID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_resources WHERE workspace_id = ? AND alias = ?`, workspace.Id, docsAlias).Scan(&docsID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_resources WHERE workspace_id = ? AND alias = ?`, workspace.Id, tokenAlias).Scan(&tokenID); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, context.Background(), store, "alice")
	err, tools := store.WorkspaceToolsGet(context.Background(), workspace, alice)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := []model.Tool{{
		Ref:     model.ToolRef{Workspace: workspace, Id: gitID, Alias: &gitAlias},
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
		{Ref: model.ResourceRef{Workspace: workspace, Id: docsID, Alias: &docsAlias}, Source: "file:./docs", Secret: false, Enabled: true},
		{Ref: model.ResourceRef{Workspace: workspace, Id: tokenID, Alias: &tokenAlias}, Source: "env:TOKEN", Secret: true, Enabled: true},
	}
	if tokenID < docsID {
		wantResources[0], wantResources[1] = wantResources[1], wantResources[0]
	}
	if !reflect.DeepEqual(resources, wantResources) {
		t.Fatalf("WorkspaceResourcesGet() = %#v, want %#v", resources, wantResources)
	}

	bob := principalRef(t, context.Background(), store, "bob")
	err, tools = store.WorkspaceToolsGet(context.Background(), workspace, bob)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("WorkspaceToolsGet() for disabled principal = %#v, want no tools", tools)
	}
	if _, err := store.Exec(`UPDATE gatehouse_workspaces SET enabled = FALSE WHERE id = ?`, workspace.Id); err != nil {
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
