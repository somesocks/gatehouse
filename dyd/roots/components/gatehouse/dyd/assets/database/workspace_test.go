package database_test

import (
	"context"
	"reflect"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
)

func TestWorkspaceRolesGetUsesDirectAndGroupGrants(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	developers := "developers"
	alice := "alice"
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{
			{Alias: alice, Enabled: true},
			{Alias: "bob", Enabled: true},
			{Alias: "carol", Enabled: true},
		},
		Groups: []config.Group{{
			WorkspaceID: "engineering", Alias: developers, Enabled: true,
			Members: []config.GroupMember{{PrincipalID: "bob", Enabled: true}},
		}},
		WorkspaceGrants: []config.WorkspaceGrant{
			{WorkspaceID: "engineering", Role: string(authz.Member), PrincipalID: &alice, Revision: 1, Enabled: true},
			{WorkspaceID: "engineering", Role: string(authz.Contributor), GroupID: &developers, Revision: 1, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	for _, test := range []struct {
		principal string
		roles     []authz.Role
	}{
		{principal: "alice", roles: []authz.Role{authz.Member}},
		{principal: "bob", roles: []authz.Role{authz.Contributor}},
		{principal: "carol", roles: []authz.Role{}},
	} {
		t.Run(test.principal, func(t *testing.T) {
			principal := principalRef(t, ctx, store, test.principal)
			err, roles := store.WorkspaceRolesGet(ctx, workspace, principal)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(roles, test.roles) {
				t.Fatalf("WorkspaceRolesGet() = %#v, want %#v", roles, test.roles)
			}
		})
	}

	bob := principalRef(t, ctx, store, "bob")
	err, workspaces := store.WorkspacesGet(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 || workspaces[0].Ref != workspace {
		t.Fatalf("WorkspacesGet() = %#v", workspaces)
	}

	developersID := groupID(t, ctx, store, "engineering", developers)
	if _, err := store.ExecContext(ctx, `
		UPDATE gatehouse_group_members
		SET enabled = FALSE
		WHERE workspace_id = ? AND group_id = ? AND principal_id = ?
	`, workspace.Id, developersID, bob.Id); err != nil {
		t.Fatal(err)
	}
	err, roles := store.WorkspaceRolesGet(ctx, workspace, bob)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 0 {
		t.Fatalf("WorkspaceRolesGet() after membership revocation = %#v, want no roles", roles)
	}
	err, workspaces = store.WorkspacesGet(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("WorkspacesGet() after membership revocation = %#v, want no workspaces", workspaces)
	}
}
