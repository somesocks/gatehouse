package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestGroupAdministrationRequiresExplicitWorkspaceAdmission(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := migrateState(ctx, store, configuration, config.State{Workspaces: []config.Workspace{{Alias: "team", Enabled: true}, {Alias: "other", Enabled: true}}, Principals: []config.Principal{{Alias: "alice", Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	workspace, other := workspaceRef(t, ctx, store, "team"), workspaceRef(t, ctx, store, "other")
	var principal string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	alias, name := "developers", "Developers"
	err, group := store.SystemGroupCreate(ctx, workspace.Id, database.SystemGroupInput{Alias: &alias, Name: &name})
	if err != nil || group == nil || group.Revision != 1 || !group.Enabled {
		t.Fatalf("create group = %#v %v", group, err)
	}
	if err, _ := store.SystemGroupCreate(ctx, workspace.Id, database.SystemGroupInput{Alias: &alias}); !errors.Is(err, database.ErrGroupAliasExists) {
		t.Fatalf("duplicate alias = %v", err)
	}
	if err, _ := store.SystemGroupCreate(ctx, other.Id, database.SystemGroupInput{Alias: &alias}); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberCreate(ctx, other.Id, group.ID, principal); !errors.Is(err, database.ErrGroupNotFound) {
		t.Fatalf("cross-workspace membership = %v", err)
	}
	if err := store.SystemGroupMemberCreate(ctx, workspace.Id, group.ID, principal); !errors.Is(err, database.ErrGroupInput) {
		t.Fatalf("unadmitted member = %v", err)
	}
	err, grant := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principal, authz.Member)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberCreate(ctx, workspace.Id, group.ID, principal); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberCreate(ctx, workspace.Id, group.ID, principal); !errors.Is(err, database.ErrGroupMemberExists) {
		t.Fatalf("duplicate membership = %v", err)
	}
	assertAccess := func(expected bool) {
		t.Helper()
		err, roles := store.WorkspaceRolesGet(ctx, workspace, model.PrincipalRef{Id: principal})
		if err != nil || authz.WorkspaceAllows(roles, authz.WorkspaceProjectCreate) != expected {
			t.Fatalf("inherited access = %v %v, want %t", roles, err, expected)
		}
	}
	assertAccess(true)
	disabled, enabled := false, true
	err, changed := store.SystemGroupUpdate(ctx, workspace.Id, group.ID, 1, database.SystemGroupInput{Enabled: &disabled})
	if err != nil || changed.Revision != 2 {
		t.Fatalf("disable group = %#v %v", changed, err)
	}
	assertAccess(true)
	if err, _ := store.SystemGroupUpdate(ctx, workspace.Id, group.ID, 1, database.SystemGroupInput{Enabled: &enabled}); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale group update = %v", err)
	}
	if err, _ := store.SystemGroupUpdate(ctx, workspace.Id, group.ID, 2, database.SystemGroupInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	assertAccess(true)
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, group.ID, principal, false, 1); err != nil {
		t.Fatal(err)
	}
	assertAccess(true)
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, group.ID, principal, true, 1); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale membership = %v", err)
	}
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, group.ID, principal, true, 2); err != nil {
		t.Fatal(err)
	}
	assertAccess(true)
	err, updatedGrant := store.SystemWorkspaceGrantUpdate(ctx, workspace.Id, grant.ID, nil, &disabled, 1)
	if err != nil || updatedGrant.Enabled || updatedGrant.Role != authz.Member || updatedGrant.Revision != 2 {
		t.Fatalf("revoke grant = %#v %v", updatedGrant, err)
	}
	assertAccess(false)
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, group.ID, principal, false, 3); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, group.ID, principal, true, 4); !errors.Is(err, database.ErrGroupInput) {
		t.Fatalf("restoration without admission = %v", err)
	}
	if err, _ := store.SystemWorkspaceGrantUpdate(ctx, workspace.Id, grant.ID, nil, &enabled, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, group.ID, principal, true, 4); err != nil {
		t.Fatal(err)
	}
	assertAccess(true)
	renamed := "engineering"
	err, renamedGroup := store.SystemGroupUpdate(ctx, workspace.Id, group.ID, 3, database.SystemGroupInput{Alias: &renamed})
	if err != nil || renamedGroup.ID != group.ID || *renamedGroup.Alias != renamed {
		t.Fatalf("rename group = %#v %v", renamedGroup, err)
	}
	assertAccess(true)
	err, members, _ := store.SystemGroupMembersSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: group.ID, Limit: 1})
	if err != nil || len(members) != 1 || members[0].Revision != 5 || !members[0].Enabled {
		t.Fatalf("members = %#v %v", members, err)
	}
	// Notifications are rooted at the principal, so losing workspace access does
	// not prevent their browser from noticing the revocation.
	for _, event := range []string{"group.update", "group_member.update", "workspace_grant.update"} {
		var count int
		if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events AS events JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id WHERE events.event = ? AND topics.topic LIKE ?`, event, principal+"/%").Scan(&count); err != nil || count == 0 {
			t.Fatalf("missing %s principal notifications: %d %v", event, count, err)
		}
	}
	err, matches, _ := store.SystemGroupsSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Name: "engineering", Limit: 1})
	if err != nil || len(matches) != 1 || matches[0].ID != group.ID {
		t.Fatalf("group search = %#v %v", matches, err)
	}
	if err, hidden := store.SystemGroupGet(ctx, other.Id, group.ID); err != nil || hidden != nil {
		t.Fatalf("group escaped workspace scope: %#v %v", hidden, err)
	}
}

func TestGroupReconciliationHonorsIndependentRevisions(t *testing.T) {
	verifyGroupReconciliationRevisions(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestConfiguredGroupMembershipRequiresExplicitAdmission(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alias := "alice"
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "team", Enabled: true}},
		Principals: []config.Principal{{Alias: alias, Enabled: true}},
		Groups:     []config.Group{{WorkspaceID: "team", Alias: "members", Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: alias, Enabled: true, Revision: 1}}}},
	}
	if err := migrateState(ctx, store, configuration, state); err == nil {
		t.Fatal("configuration admitted a person through group membership")
	}
	state.WorkspaceGrants = []config.WorkspaceGrant{{WorkspaceID: "team", PrincipalID: &alias, Role: "guest", Enabled: true, Revision: 1}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "team")
	principal := principalRef(t, ctx, store, alias)
	if err, _ := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, principal.Id); err != nil {
		t.Fatal(err)
	}
	state.Groups[0].Members[0].Enabled = false
	state.Groups[0].Members[0].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	state.Groups[0].Members[0].Enabled = true
	state.Groups[0].Members[0].Revision = 3
	if err := migrateState(ctx, store, configuration, state); err == nil {
		t.Fatal("configuration restored membership without admission")
	}
	state.WorkspaceGrants[0].Revision = 3
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	err, groups := store.WorkspaceGroupsGet(ctx, workspace, principal)
	if err != nil || len(groups) != 1 {
		t.Fatalf("restored configured membership: %#v %v", groups, err)
	}
}

func TestPostgresGroupReconciliationHonorsIndependentRevisions(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	verifyGroupReconciliationRevisions(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func verifyGroupReconciliationRevisions(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	placeholder := "?"
	if configuration.Kind == config.DatabaseKindPostgres {
		placeholder = "$1"
	}
	workspaceAlias := "group-test-" + newTypedID(t, typed_id.Workspace)
	principalAlias := "group-test-" + newTypedID(t, typed_id.Principal)
	name := "Configured group"
	state := config.State{Workspaces: []config.Workspace{{Alias: workspaceAlias, Enabled: true}}, Principals: []config.Principal{{Alias: principalAlias, Enabled: true}}, Groups: []config.Group{{WorkspaceID: workspaceAlias, Alias: "developers", Name: &name, Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: principalAlias, Enabled: true, Revision: 1}}}}}
	state.WorkspaceGrants = []config.WorkspaceGrant{{WorkspaceID: workspaceAlias, PrincipalID: &principalAlias, Role: "guest", Revision: 1, Enabled: true}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, workspaceAlias)
	var id, principal string
	if err := store.QueryRow(`SELECT id FROM gatehouse_groups WHERE workspace_id = `+placeholder+` AND alias = 'developers'`, workspace.Id).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = `+placeholder, principalAlias).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if err, _ := store.SystemGroupUpdate(ctx, workspace.Id, id, 1, database.SystemGroupInput{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, id, principal, false, 1); err != nil {
		t.Fatal(err)
	}
	assertRecords := func(groupRevision, memberRevision int, groupEnabled, memberEnabled bool, expectedName string) {
		t.Helper()
		err, group := store.SystemGroupGet(ctx, workspace.Id, id)
		if err != nil || group == nil || group.Enabled != groupEnabled || group.Revision != groupRevision || group.Name == nil || *group.Name != expectedName {
			t.Fatalf("reconciled group = %#v %v, want revision %d enabled %t name %q", group, err, groupRevision, groupEnabled, expectedName)
		}
		err, members, _ := store.SystemGroupMembersSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: id, Name: principalAlias, Limit: 10})
		if err != nil || len(members) != 1 || members[0].Enabled != memberEnabled || members[0].Revision != memberRevision {
			t.Fatalf("reconciled members = %#v %v, want revision %d enabled %t", members, err, memberRevision, memberEnabled)
		}
	}
	assertRecords(2, 2, false, false, name)
	// Change declarations so SQLite actually reruns its repeatable migration.
	// Neither equal nor lower config revisions may undo the admin changes.
	for _, revision := range []int{2, 1} {
		ignoredName := "Ignored config name"
		state.Groups[0].Name = &ignoredName
		state.Groups[0].Revision = revision
		state.Groups[0].Members[0].Revision = revision
		if err := migrateState(ctx, store, configuration, state); err != nil {
			t.Fatal(err)
		}
		assertRecords(2, 2, false, false, name)
	}
	reconfiguredName := "Updated configured group"
	state.Groups[0].Name = &reconfiguredName
	state.Groups[0].Revision = 5
	state.Groups[0].Members[0].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertRecords(5, 2, true, false, reconfiguredName)
	// Membership revisions apply independently of the enclosing group revision.
	state.Groups[0].Revision = 1
	state.Groups[0].Name = &name
	state.Groups[0].Members[0].Revision = 4
	state.Groups[0].Members[0].Role = "admin"
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertRecords(5, 4, true, true, reconfiguredName)
	if err, role := store.GroupRoleGet(ctx, workspace.Id, id, model.PrincipalRef{Id: principal}); err != nil || role != authz.Admin {
		t.Fatalf("configured manager role: %s %v", role, err)
	}
	// A new membership still gets created when group metadata has a newer DB revision.
	newPrincipalAlias := "group-test-" + newTypedID(t, typed_id.Principal)
	state.Principals = append(state.Principals, config.Principal{Alias: newPrincipalAlias, Enabled: true})
	state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{WorkspaceID: workspaceAlias, PrincipalID: &newPrincipalAlias, Role: "guest", Revision: 1, Enabled: true})
	state.Groups[0].Members = append(state.Groups[0].Members, config.GroupMember{PrincipalID: newPrincipalAlias, Enabled: true, Revision: 9})
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	err, newMembers, _ := store.SystemGroupMembersSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: id, Name: newPrincipalAlias, Limit: 10})
	if err != nil || len(newMembers) != 1 || newMembers[0].Revision != 9 {
		t.Fatalf("new membership revision = %#v %v", newMembers, err)
	}
	assertRecords(5, 4, true, true, reconfiguredName)
	// Higher revisions take effect even if the configured values are unchanged.
	state.Groups[0].Name = &reconfiguredName
	state.Groups[0].Revision = 7
	state.Groups[0].Members[0].Revision = 6
	for range 2 {
		if err := migrateState(ctx, store, configuration, state); err != nil {
			t.Fatal(err)
		}
		assertRecords(7, 6, true, true, reconfiguredName)
	}
	if err := store.SystemGroupMemberUpdate(ctx, workspace.Id, id, principal, false, 4); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("reconciliation did not invalidate membership draft: %v", err)
	}
	if err, _ := store.SystemGroupUpdate(ctx, workspace.Id, id, 5, database.SystemGroupInput{Enabled: &disabled}); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("reconciliation did not invalidate group draft: %v", err)
	}
	for _, resource := range []string{"resource_group", "resource_group_member_group"} {
		var count int
		if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE `+resource+` = `+placeholder+` AND event IN ('group.update', 'group_member.update')`, id).Scan(&count); err != nil || count != 3 {
			t.Fatalf("reconciliation emitted redundant %s updates: %d %v", resource, count, err)
		}
	}
}
