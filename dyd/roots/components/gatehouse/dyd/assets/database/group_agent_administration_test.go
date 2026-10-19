package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"
)

func groupAgentTestState(t *testing.T) config.State {
	t.Helper()
	suffix := newTypedID(t, typed_id.Workspace)
	workspace, other, principal, provider, model := "bots-"+suffix, "other-"+suffix, "member-"+suffix, "provider-"+suffix, "model-"+suffix
	return config.State{
		Workspaces:     []config.Workspace{{Alias: workspace, Enabled: true}, {Alias: other, Enabled: true}},
		Principals:     []config.Principal{{Alias: principal, Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: provider, Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:    []config.AgentModel{{Alias: model, ProviderAlias: provider, Revision: 1, Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: workspace, Alias: "assistant", ModelAlias: model, Revision: 1, Enabled: true},
			{WorkspaceID: workspace, Alias: "helper", ModelAlias: model, Revision: 1, Enabled: true},
			{WorkspaceID: other, Alias: "assistant", ModelAlias: model, Revision: 1, Enabled: true},
		},
		Groups: []config.Group{
			{WorkspaceID: workspace, Alias: "developers", Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: principal, Enabled: true, Revision: 1}}},
			{WorkspaceID: workspace, Alias: "support", Enabled: true, Revision: 1},
			{WorkspaceID: other, Alias: "developers", Enabled: true, Revision: 1},
		},
	}
}

func groupAgentTestIDs(t *testing.T, store *database.Store, workspace, group string) (string, map[string]string) {
	t.Helper()
	err, groups, _ := store.SystemGroupsSearch(context.Background(), database.SystemGroupSearch{Workspace: workspace, Name: group, Limit: 10})
	if err != nil || len(groups) != 1 {
		t.Fatalf("find group = %#v %v", groups, err)
	}
	err, agents, _ := store.SystemWorkspaceAgentsSearch(context.Background(), database.SystemWorkspaceAgentSearch{WorkspaceID: workspace, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, agent := range agents {
		ids[agent.Alias] = agent.ID
	}
	return groups[0].ID, ids
}

func TestGroupAgentAdministration(t *testing.T) {
	testGroupAgentAdministration(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresGroupAgentAdministration(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testGroupAgentAdministration(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testGroupAgentAdministration(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := groupAgentTestState(t)
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, state.Workspaces[0].Alias)
	other := workspaceRef(t, ctx, store, state.Workspaces[1].Alias)
	group, bots := groupAgentTestIDs(t, store, workspace.Id, "developers")
	support, _ := groupAgentTestIDs(t, store, workspace.Id, "support")
	_, outside := groupAgentTestIDs(t, store, other.Id, "developers")
	bot := bots["assistant"]
	if err := store.SystemGroupAgentCreate(ctx, other.Id, group, bot); !errors.Is(err, database.ErrGroupNotFound) {
		t.Fatalf("foreign group = %v", err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, outside["assistant"]); !errors.Is(err, database.ErrGroupAgentNotFound) {
		t.Fatalf("foreign bot = %v", err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, "bad-id"); !errors.Is(err, database.ErrGroupInput) {
		t.Fatalf("invalid bot = %v", err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, bot); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, support, bot); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, bot); !errors.Is(err, database.ErrGroupMemberExists) {
		t.Fatalf("duplicate membership = %v", err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, true, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, false, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, true, 1); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale membership = %v", err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, bot); !errors.Is(err, database.ErrGroupMemberExists) {
		t.Fatalf("disabled duplicate = %v", err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, true, 0); !errors.Is(err, database.ErrGroupInput) {
		t.Fatalf("invalid revision = %v", err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, other.Id, group, bot, true, 2); !errors.Is(err, database.ErrGroupNotFound) {
		t.Fatalf("foreign update = %v", err)
	}
	search := database.SystemGroupSearch{Workspace: workspace.Id, Group: group, Limit: 1, Status: "disabled"}
	err, members, _ := store.SystemGroupAgentsSearch(ctx, search)
	if err != nil || len(members) != 1 || members[0].Agent != bot || members[0].Revision != 2 || members[0].Enabled || !members[0].AgentEnabled {
		t.Fatalf("disabled membership = %#v %v", members, err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, true, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bot, true, 3); err != nil {
		t.Fatal(err)
	}
	p := func(index int) string {
		if configuration.Kind == config.DatabaseKindPostgres {
			return fmt.Sprintf("$%d", index)
		}
		return "?"
	}
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_group_agent_group = `+p(1)+` AND resource_group_agent_agent = `+p(2), group, bot).Scan(&count); err != nil || count != 3 {
		t.Fatalf("membership activity/no-ops = %d %v", count, err)
	}
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events e JOIN gatehouse_activity_event_topics t ON t.activity = e.id JOIN gatehouse_group_members m ON m.group_id = e.resource_group_agent_group AND t.topic = m.principal_id || '/' || m.group_id WHERE e.resource_group_agent_group = `+p(1), group).Scan(&count); err != nil || count != 3 {
		t.Fatalf("principal notifications = %d %v", count, err)
	}
	err, supportMembers, _ := store.SystemGroupAgentsSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: support, Limit: 10})
	if err != nil || len(supportMembers) != 1 || !supportMembers[0].Enabled || supportMembers[0].Revision != 1 {
		t.Fatalf("other group membership changed: %#v %v", supportMembers, err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, bots["helper"]); err != nil {
		t.Fatal(err)
	}
	search.Status = ""
	seen := map[string]bool{}
	for {
		err, page, next := store.SystemGroupAgentsSearch(ctx, search)
		if err != nil || len(page) != 1 || seen[page[0].Agent] {
			t.Fatalf("membership page = %#v %v", page, err)
		}
		seen[page[0].Agent] = true
		if next == "" {
			break
		}
		search.Cursor = next
	}
	if len(seen) != 2 {
		t.Fatalf("paged %d memberships", len(seen))
	}
	search.Cursor, search.Name = "", "helper"
	err, members, _ = store.SystemGroupAgentsSearch(ctx, search)
	if err != nil || len(members) != 1 || members[0].Alias != "helper" {
		t.Fatalf("alias search = %#v %v", members, err)
	}
	label := "Special 100%_ bot"
	if _, err := store.Exec(`UPDATE gatehouse_workspace_agents SET label = `+p(1)+`, enabled = FALSE WHERE workspace_id = `+p(2)+` AND id = `+p(3), label, workspace.Id, bots["helper"]); err != nil {
		t.Fatal(err)
	}
	search.Name = "%_"
	err, members, _ = store.SystemGroupAgentsSearch(ctx, search)
	if err != nil || len(members) != 1 || members[0].AgentEnabled || !members[0].Enabled || members[0].Label == nil || *members[0].Label != label {
		t.Fatalf("label search/independent availability = %#v %v", members, err)
	}
	if err, record := store.SystemGroupGet(ctx, workspace.Id, group); err != nil || record.Revision != 1 {
		t.Fatalf("membership changed group revision: %#v %v", record, err)
	}
	if err, humans, _ := store.SystemGroupMembersSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: group, Limit: 10}); err != nil || len(humans) != 1 || humans[0].Revision != 1 {
		t.Fatalf("human memberships changed = %#v %v", humans, err)
	}
	if _, err := store.Exec(`INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, TRUE)`, workspace.Id, support, outside["assistant"]); err == nil {
		t.Fatal("database accepted cross-workspace bot membership")
	}
}

func TestGroupAgentReconciliation(t *testing.T) {
	testGroupAgentReconciliation(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresGroupAgentReconciliation(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testGroupAgentReconciliation(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testGroupAgentReconciliation(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := groupAgentTestState(t)
	state.Groups[0].Agents = []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 4}}
	state.Groups[1].Agents = []config.GroupAgent{{AgentID: "assistant", Enabled: false, Revision: 2}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, state.Workspaces[0].Alias)
	group, bots := groupAgentTestIDs(t, store, workspace.Id, "developers")
	assertMembership := func(revision int, enabled bool) {
		t.Helper()
		err, agents, _ := store.SystemGroupAgentsSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: group, Name: "assistant", Limit: 10})
		if err != nil || len(agents) != 1 || agents[0].Agent != bots["assistant"] || agents[0].Revision != revision || agents[0].Enabled != enabled {
			t.Fatalf("membership = %#v %v, want revision %d enabled %t", agents, err, revision, enabled)
		}
	}
	assertMembership(4, true)
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bots["assistant"], false, 4); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int{5, 1} {
		state.Groups[0].Agents[0].Revision = revision
		if err := migrateState(ctx, store, configuration, state); err != nil {
			t.Fatal(err)
		}
		assertMembership(5, false)
	}
	state.Groups[0].Revision = 10
	state.Groups[0].Agents[0].Revision = 3
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertMembership(5, false)
	state.Groups[0].Revision = 1
	state.Groups[0].Agents[0].Revision = 7
	state.Groups[0].Agents = append(state.Groups[0].Agents, config.GroupAgent{AgentID: "helper", Enabled: true, Revision: 9})
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertMembership(7, true)
	state.Groups[0].Agents[0].Revision = 8
	for range 2 {
		if err := migrateState(ctx, store, configuration, state); err != nil {
			t.Fatal(err)
		}
		assertMembership(8, true)
	}
	state.Groups[0].Agents = nil
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertMembership(8, true)
	p := "?"
	if configuration.Kind == config.DatabaseKindPostgres {
		p = "$1"
	}
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_group_agent_group = `+p, group).Scan(&count); err != nil || count != 5 {
		t.Fatalf("reconciliation activity/no-ops = %d %v", count, err)
	}
	if err, record := store.SystemGroupGet(ctx, workspace.Id, group); err != nil || record.Revision != 10 {
		t.Fatalf("bot reconciliation changed group revision = %#v %v", record, err)
	}
	if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group, bots["assistant"], false, 7); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("reconciliation did not invalidate draft: %v", err)
	}
	// Resolve in the group's workspace even when another workspace has this alias.
	state.WorkspaceAgents = append(state.WorkspaceAgents, config.WorkspaceAgent{WorkspaceID: state.Workspaces[1].Alias, Alias: "only-elsewhere", ModelAlias: state.AgentModels[0].Alias, Enabled: true, Revision: 1})
	state.Groups[0].Agents = []config.GroupAgent{{AgentID: "only-elsewhere", Enabled: true, Revision: 1}}
	if err := migrateState(ctx, store, configuration, state); err == nil {
		t.Fatal("reconciled a bot alias from another workspace")
	}
	assertMembership(8, true)
}

func TestGroupAgentActivityFailureRollsBackMembership(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := groupAgentTestState(t)
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, state.Workspaces[0].Alias)
	group, bots := groupAgentTestIDs(t, store, workspace.Id, "developers")
	if _, err := store.Exec(`CREATE TRIGGER fail_group_agent_activity BEFORE INSERT ON gatehouse_activity_events WHEN NEW.resource_kind = 'group_agent' BEGIN SELECT RAISE(ABORT, 'activity unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group, bots["assistant"]); err == nil {
		t.Fatal("membership committed without activity")
	}
	if err, agents, _ := store.SystemGroupAgentsSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: group, Limit: 10}); err != nil || len(agents) != 0 {
		t.Fatalf("membership escaped rollback = %#v %v", agents, err)
	}
}
