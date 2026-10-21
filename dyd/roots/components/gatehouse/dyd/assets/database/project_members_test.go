package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestProjectMemberGrants(t *testing.T) {
	for _, kind := range []config.DatabaseKind{config.DatabaseKindEphemeral, config.DatabaseKindPostgres} {
		t.Run(string(kind), func(t *testing.T) {
			configuration := config.DatabaseConfig{Kind: kind}
			if kind == config.DatabaseKindPostgres {
				if os.Getenv("GATEHOUSE_TEST_PROJECT_MEMBERS_POSTGRES_URL") == "" {
					t.Skip("requires disposable PostgreSQL database")
				}
				configuration.URL = "env:GATEHOUSE_TEST_PROJECT_MEMBERS_POSTGRES_URL"
			}
			testProjectMemberGrants(t, configuration)
		})
	}
}

func testProjectMemberGrants(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}, {Alias: "elsewhere", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}, {Alias: "outsider", Enabled: true}, {Alias: "disabled", Enabled: false}},
		Groups: []config.Group{
			{WorkspaceID: "engineering", Alias: "developers", Enabled: true, Members: []config.GroupMember{{PrincipalID: "bob", Enabled: true}}},
			{WorkspaceID: "engineering", Alias: "empty", Enabled: true},
			{WorkspaceID: "engineering", Alias: "outside", Enabled: true, Members: []config.GroupMember{{PrincipalID: "outsider", Enabled: true}}},
			{WorkspaceID: "engineering", Alias: "disabled", Enabled: false},
			{WorkspaceID: "elsewhere", Alias: "other", Enabled: true},
		},
		WorkspaceGrants: []config.WorkspaceGrant{
			{WorkspaceID: "engineering", PrincipalID: stringPointer("alice"), Role: "manager", Enabled: true, Revision: 1},
			{WorkspaceID: "engineering", PrincipalID: stringPointer("bob"), Role: "member", Enabled: true, Revision: 1},
			{WorkspaceID: "engineering", PrincipalID: stringPointer("outsider"), Role: "member", Enabled: true, Revision: 1},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	p := func(index int) string {
		if configuration.Kind == config.DatabaseKindPostgres {
			return fmt.Sprintf("$%d", index)
		}
		return fmt.Sprintf("?%d", index)
	}
	lookup := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := store.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	person := func(alias string) model.PrincipalRef {
		return model.PrincipalRef{Id: lookup(`SELECT id FROM gatehouse_principals WHERE alias = `+p(1), alias)}
	}
	groupRef := func(workspaceAlias, alias string) string {
		return lookup(`SELECT g.id FROM gatehouse_groups g JOIN gatehouse_workspaces w ON w.id = g.workspace_id WHERE w.alias = `+p(1)+` AND g.alias = `+p(2), workspaceAlias, alias)
	}
	alice, bob := person("alice"), person("bob")
	outsider, disabled := person("outsider"), person("disabled")
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_workspace_grants SET enabled = FALSE WHERE principal = (SELECT id FROM gatehouse_principals WHERE alias = 'outsider')`); err != nil {
		t.Fatal(err)
	}
	group := groupRef("engineering", "developers")
	empty := groupRef("engineering", "empty")
	outside := groupRef("engineering", "outside")
	otherGroup := groupRef("elsewhere", "other")
	disabledGroup := groupRef("engineering", "disabled")
	id, _ := typed_id.New(typed_id.Project)
	project := model.ProjectRef{Workspace: workspace, Id: id}
	if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, alice, nil); err != nil {
		t.Fatal(err)
	}
	err, people, _ := store.ProjectMembersSearch(ctx, project, alice, database.ProjectMemberSearch{Kind: "principal", Limit: 50}, true)
	if err != nil || len(people) != 2 {
		t.Fatalf("admitted principal options: %#v %v", people, err)
	}
	for _, recipient := range []struct{ principal, group *string }{{&outsider.Id, nil}, {&disabled.Id, nil}, {nil, &otherGroup}, {nil, &disabledGroup}} {
		if err, _ := store.ProjectGrantCreate(ctx, project, alice, recipient.principal, recipient.group, authz.Member); !errors.Is(err, database.ErrProjectMemberNotFound) {
			t.Fatalf("unavailable recipient: %v", err)
		}
	}
	if err, _ := store.ProjectGrantCreate(ctx, project, alice, &bob.Id, &group, authz.Member); !errors.Is(err, database.ErrProjectMemberInput) {
		t.Fatalf("mixed subjects: %v", err)
	}
	err, direct := store.ProjectGrantCreate(ctx, project, alice, &bob.Id, nil, authz.Contributor)
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.ProjectGrantCreate(ctx, project, alice, &bob.Id, nil, authz.Contributor); !errors.Is(err, database.ErrProjectGrantExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if err, _ := store.ProjectGrantCreate(ctx, project, bob, nil, &group, authz.Manager); !errors.Is(err, database.ErrProjectMemberForbidden) {
		t.Fatalf("contributor escalated: %v", err)
	}
	if err, _, _ := store.ProjectMembersSearch(ctx, project, bob, database.ProjectMemberSearch{Kind: "group", Limit: 50}, true); !errors.Is(err, database.ErrProjectMemberForbidden) {
		t.Fatalf("non-manager options: %v", err)
	}
	err, grouped := store.ProjectGrantCreate(ctx, project, alice, nil, &group, authz.Member)
	if err != nil {
		t.Fatal(err)
	}
	err, viewer := store.ProjectGrantCreate(ctx, project, alice, &bob.Id, nil, authz.Member)
	if err != nil {
		t.Fatal(err)
	}
	err, people, _ = store.ProjectMembersSearch(ctx, project, bob, database.ProjectMemberSearch{Kind: "principal", Name: "bob", Limit: 1}, false)
	if err != nil || len(people) != 1 || len(people[0].Grants) != 2 {
		t.Fatalf("explicit roles: %#v %v", people, err)
	}
	member := authz.Member
	if err, _ := store.ProjectGrantUpdate(ctx, project, alice, direct.ID, &member, nil, 1); !errors.Is(err, database.ErrProjectGrantExists) {
		t.Fatalf("role collision: %v", err)
	}
	search := database.ProjectMemberSearch{Kind: "principal", Limit: 1}
	seen := map[string]bool{}
	var manager database.ProjectGrant
	for {
		err, page, next := store.ProjectMembersSearch(ctx, project, alice, search, false)
		if err != nil || len(page) != 1 || seen[page[0].ID] {
			t.Fatalf("page: %#v %q %v", page, next, err)
		}
		seen[page[0].ID] = true
		if page[0].ID == alice.Id {
			manager = page[0].Grants[0]
		}
		if next == "" {
			break
		}
		search.Cursor = next
	}
	if len(seen) != 2 || manager.Revision != 1 {
		t.Fatalf("migrated creator grant or pagination: %#v %#v", manager, seen)
	}
	for _, search := range []database.ProjectMemberSearch{{Kind: "agent", Limit: 50}, {Kind: "group", Cursor: bob.Id, Limit: 1}, {Kind: "principal", Limit: 101}} {
		if err, _, _ := store.ProjectMembersSearch(ctx, project, alice, search, false); !errors.Is(err, database.ErrProjectMemberInput) {
			t.Fatalf("invalid search: %v", err)
		}
	}
	removed := false
	if err, _ := store.ProjectGrantUpdate(ctx, project, alice, direct.ID, nil, &removed, 2); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	for _, grant := range []*database.ProjectGrant{direct, viewer} {
		if err, _ := store.ProjectGrantUpdate(ctx, project, alice, grant.ID, nil, &removed, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err, visible := store.ProjectGet(ctx, project, bob); err != nil || visible == nil {
		t.Fatalf("group access lost with direct grants: %#v %v", visible, err)
	}
	err, restored := store.ProjectGrantCreate(ctx, project, alice, &bob.Id, nil, authz.Contributor)
	if err != nil || restored.ID != direct.ID || restored.Revision != 3 {
		t.Fatalf("reactivation: %#v %v", restored, err)
	}
	otherID, _ := typed_id.New(typed_id.Project)
	other := model.ProjectRef{Workspace: workspace, Id: otherID}
	if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: other, Enabled: true}, alice, nil); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.ProjectGrantUpdate(ctx, other, alice, grouped.ID, nil, &removed, 1); !errors.Is(err, database.ErrProjectMemberNotFound) {
		t.Fatalf("cross-project mutation: %v", err)
	}
	for _, nonManager := range []string{empty, outside} {
		if err, _ := store.ProjectGrantCreate(ctx, project, alice, nil, &nonManager, authz.Manager); err != nil {
			t.Fatal(err)
		}
	}
	if err, _ := store.ProjectGrantUpdate(ctx, project, alice, manager.ID, nil, &removed, 1); !errors.Is(err, database.ErrProjectLastManager) {
		t.Fatalf("empty/non-admitted groups counted as effective managers: %v", err)
	}
	if err, _ := store.ProjectGrantCreate(ctx, project, alice, &bob.Id, nil, authz.Manager); err != nil {
		t.Fatal(err)
	}
	err, bobPage, _ := store.ProjectMembersSearch(ctx, project, alice, database.ProjectMemberSearch{Kind: "principal", Name: "bob", Limit: 50}, false)
	if err != nil {
		t.Fatal(err)
	}
	var bobManager database.ProjectGrant
	for _, grant := range bobPage[0].Grants {
		if grant.Role == authz.Manager {
			bobManager = grant
		}
	}
	results := make(chan error, 2)
	for _, attempt := range []struct {
		actor model.PrincipalRef
		grant database.ProjectGrant
	}{{alice, manager}, {bob, bobManager}} {
		go func() {
			err, _ := store.ProjectGrantUpdate(ctx, project, attempt.actor, attempt.grant.ID, nil, &removed, attempt.grant.Revision)
			results <- err
		}()
	}
	succeeded, blocked := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, database.ErrProjectLastManager) {
			blocked++
		} else {
			t.Fatalf("competing manager removals: %v", err)
		}
	}
	if succeeded != 1 || blocked != 1 {
		t.Fatalf("manager removals: succeeded=%d blocked=%d", succeeded, blocked)
	}
}
