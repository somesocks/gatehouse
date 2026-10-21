package database_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestGroupMutationsRequireCurrentManagerMembership(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, bob := "alice", "bob"
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "team", Enabled: true}},
		Principals:      []config.Principal{{Alias: alice, Enabled: true}, {Alias: bob, Enabled: true}},
		WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "team", PrincipalID: &alice, Role: "manager", Enabled: true, Revision: 1}, {WorkspaceID: "team", PrincipalID: &bob, Role: "member", Enabled: true, Revision: 1}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "team")
	actor, target := principalRef(t, ctx, store, alice), principalRef(t, ctx, store, bob)
	alias := "delegated"
	err, group := store.GroupCreate(ctx, workspace.Id, database.SystemGroupInput{Alias: &alias}, &actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.GroupMemberCreate(ctx, workspace.Id, group.ID, target.Id, authz.Manager, &actor); err != nil {
		t.Fatal(err)
	}
	member := authz.Member
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group.ID, actor.Id, nil, &member, 1, &actor); err != nil {
		t.Fatal(err)
	}
	name := "Denied"
	if err, _ := store.GroupUpdate(ctx, workspace.Id, group.ID, 1, database.SystemGroupInput{Name: &name}, &actor); !errors.Is(err, database.ErrGroupForbidden) {
		t.Fatalf("demoted manager edited group: %v", err)
	}
	manager := authz.Manager
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group.ID, actor.Id, nil, &manager, 2, &actor); !errors.Is(err, database.ErrGroupForbidden) {
		t.Fatalf("demoted manager promoted self: %v", err)
	}
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group.ID, actor.Id, nil, &manager, 2, nil); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, actor.Id); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.GroupUpdate(ctx, workspace.Id, group.ID, 1, database.SystemGroupInput{Name: &name}, &actor); !errors.Is(err, database.ErrGroupForbidden) {
		t.Fatalf("unadmitted manager edited group: %v", err)
	}
}

func TestGroupRequiresEnabledManager(t *testing.T) {
	verifyGroupRequiresEnabledManager(t, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "groups.db")})
}

func TestPostgresGroupRequiresEnabledManager(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	verifyGroupRequiresEnabledManager(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func verifyGroupRequiresEnabledManager(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workspaceAlias := "managers-" + newTypedID(t, typed_id.Workspace)
	aliceAlias, bobAlias := "alice-"+workspaceAlias, "bob-"+workspaceAlias
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: workspaceAlias, Enabled: true}},
		Principals:      []config.Principal{{Alias: aliceAlias, Enabled: true}, {Alias: bobAlias, Enabled: true}},
		WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: workspaceAlias, PrincipalID: &aliceAlias, Role: "member", Enabled: true, Revision: 1}, {WorkspaceID: workspaceAlias, PrincipalID: &bobAlias, Role: "member", Enabled: true, Revision: 1}},
		Groups:          []config.Group{{WorkspaceID: workspaceAlias, Alias: "team", Enabled: true, Members: []config.GroupMember{{PrincipalID: aliceAlias, Role: "manager", Enabled: true, Revision: 1}, {PrincipalID: bobAlias, Role: "manager", Enabled: false, Revision: 1}}}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, workspaceAlias)
	group := groupID(t, ctx, store, workspaceAlias, "team")
	alice, bob := principalRef(t, ctx, store, aliceAlias), principalRef(t, ctx, store, bobAlias)
	member, manager := authz.Member, authz.Manager
	enabled, disabled := true, false
	for _, actor := range []*model.PrincipalRef{&alice, nil} {
		for _, change := range []struct {
			enabled *bool
			role    *authz.Role
		}{{nil, &member}, {&disabled, nil}, {&disabled, &member}} {
			if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, change.enabled, change.role, 1, actor); !errors.Is(err, database.ErrGroupLastManager) {
				t.Fatalf("last manager change = %v", err)
			}
		}
	}
	err, members, _ := store.SystemGroupMembersSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: group, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range members {
		if person.Principal == alice.Id && (person.Revision != 1 || !person.Enabled || person.Role != authz.Manager) {
			t.Fatalf("rejected change modified manager: %#v", person)
		}
	}
	// An unchanged save is still valid, and a disabled manager did not count.
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, &enabled, &manager, 1, &alice); err != nil {
		t.Fatal(err)
	}
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, bob.Id, &enabled, nil, 1, &alice); err != nil {
		t.Fatal(err)
	}
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, nil, &member, 1, &alice); err != nil {
		t.Fatal(err)
	}
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, bob.Id, nil, &member, 2, &bob); !errors.Is(err, database.ErrGroupLastManager) {
		t.Fatalf("new sole manager demotion = %v", err)
	}
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, &disabled, nil, 2, &bob); err != nil {
		t.Fatal(err)
	}
	if err := store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, &enabled, &manager, 3, &bob); err != nil {
		t.Fatal(err)
	}
	// Separate connections cannot both see two managers and demote both.
	err, peer := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- store.GroupMemberUpdate(ctx, workspace.Id, group, alice.Id, nil, &member, 4, &alice)
	}()
	go func() {
		<-start
		results <- peer.GroupMemberUpdate(ctx, workspace.Id, group, bob.Id, nil, &member, 2, &bob)
	}()
	close(start)
	succeeded, blocked := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, database.ErrGroupLastManager) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || blocked != 1 {
		t.Fatalf("concurrent demotions: succeeded %d blocked %d", succeeded, blocked)
	}
}

func TestGroupConfigurationRetainsManager(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, bob := "alice", "bob"
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "team", Enabled: true}},
		Principals:      []config.Principal{{Alias: alice, Enabled: true}, {Alias: bob, Enabled: true}},
		WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "team", PrincipalID: &alice, Role: "member", Enabled: true, Revision: 1}, {WorkspaceID: "team", PrincipalID: &bob, Role: "member", Enabled: true, Revision: 1}},
		Groups:          []config.Group{{WorkspaceID: "team", Alias: "team", Enabled: true, Members: []config.GroupMember{{PrincipalID: alice, Role: "manager", Enabled: true, Revision: 1}, {PrincipalID: bob, Role: "member", Enabled: true, Revision: 1}}}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "team")
	group := groupID(t, ctx, store, "team", "team")
	assertRejected := func() {
		t.Helper()
		if err := migrateState(ctx, store, configuration, state); err == nil || !strings.Contains(err.Error(), "group_requires_enabled_manager") {
			t.Fatalf("managerless configuration = %v", err)
		}
	}
	state.Groups[0].Members[0].Role = "member"
	state.Groups[0].Members[0].Revision = 2
	assertRejected()
	// A replacement must actually take effect, rather than be ignored at an equal revision.
	state.Groups[0].Members[1].Role = "manager"
	assertRejected()
	state.Groups[0].Members[1].Revision = 2
	state.Groups[0].Members[1].Enabled = false
	assertRejected()
	state.Groups[0].Members[1].Enabled = true
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatalf("atomic manager handoff: %v", err)
	}
	state.Groups[0].Members[1].Enabled = false
	state.Groups[0].Members[1].Revision = 3
	assertRejected()
	err, members, _ := store.SystemGroupMembersSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: group, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range members {
		if person.Alias != nil && *person.Alias == bob && (!person.Enabled || person.Role != authz.Manager || person.Revision != 2) {
			t.Fatalf("rejected config changed manager: %#v", person)
		}
	}
}
