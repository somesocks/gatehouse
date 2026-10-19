package database_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func workspaceAgentDefaultsFixture(t *testing.T, configuration config.DatabaseConfig) (*database.Store, config.State, model.WorkspaceRef, string) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	suffix := newTypedID(t, typed_id.Workspace)
	workspace, provider, agentModel := "defaults-"+suffix, "provider-"+suffix, "model-"+suffix
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: workspace, Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: provider, Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:    []config.AgentModel{{Alias: agentModel, ProviderAlias: provider, Revision: 1, Model: "dummy.fixed-reply", Parameters: `{"text":"Done"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: workspace, Alias: "assistant", ModelAlias: agentModel, Revision: 3, Default: true, Enabled: true},
			{WorkspaceID: workspace, Alias: "reviewer", ModelAlias: agentModel, Revision: 5, Default: true, Enabled: true},
			{WorkspaceID: workspace, Alias: "private", ModelAlias: agentModel, Revision: 2, Enabled: true},
		},
		Groups: []config.Group{{WorkspaceID: workspace, Alias: "team", Enabled: true, Revision: 1, Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 4}}}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	ref := workspaceRef(t, ctx, store, workspace)
	err, bots, _ := store.SystemWorkspaceAgentsSearch(ctx, database.SystemWorkspaceAgentSearch{WorkspaceID: ref.Id, Limit: 10})
	if err != nil || len(bots) != 3 {
		t.Fatalf("initial bots = %#v, %v", bots, err)
	}
	return store, state, ref, bots[0].ModelID
}

func workspaceAgentDefaultsSnapshot(t *testing.T, store *database.Store, workspace model.WorkspaceRef) map[string]database.SystemWorkspaceAgent {
	t.Helper()
	err, bots, _ := store.SystemWorkspaceAgentsSearch(context.Background(), database.SystemWorkspaceAgentSearch{WorkspaceID: workspace.Id, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]database.SystemWorkspaceAgent{}
	for _, bot := range bots {
		result[bot.Alias] = bot
	}
	return result
}

func TestWorkspaceAgentDefaultsAdministration(t *testing.T) {
	testWorkspaceAgentDefaultsAdministration(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresWorkspaceAgentDefaultsAdministration(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testWorkspaceAgentDefaultsAdministration(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testWorkspaceAgentDefaultsAdministration(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	store, _, workspace, modelID := workspaceAgentDefaultsFixture(t, configuration)
	initial := workspaceAgentDefaultsSnapshot(t, store, workspace)
	if !initial["assistant"].Default || !initial["reviewer"].Default {
		t.Fatal("initial configuration did not retain both defaults")
	}
	err, welcome := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "welcome", ModelID: modelID, Default: true, Enabled: true})
	if err != nil || welcome == nil || !welcome.Default {
		t.Fatalf("create default = %#v, %v", welcome, err)
	}
	peers := map[string]database.SystemWorkspaceAgent{"assistant": initial["assistant"], "reviewer": initial["reviewer"]}
	assertPeers := func() {
		t.Helper()
		current := workspaceAgentDefaultsSnapshot(t, store, workspace)
		for alias, peer := range peers {
			if !reflect.DeepEqual(current[alias], peer) {
				t.Fatalf("changing another bot modified %s: got %#v, want %#v", alias, current[alias], peer)
			}
		}
	}
	assertPeers()
	peers["welcome"] = workspaceAgentDefaultsSnapshot(t, store, workspace)["welcome"]
	private := initial["private"]
	for _, value := range []bool{true, false, true} {
		err, changed := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, private.ID, private.Revision, database.SystemWorkspaceAgentInput{ModelID: modelID, Default: value, Enabled: true})
		if err != nil || changed == nil || changed.Default != value || changed.Revision != private.Revision+1 {
			t.Fatalf("update default = %#v, %v", changed, err)
		}
		private = *changed
		assertPeers()
	}
	err, disabled := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, private.ID, private.Revision, database.SystemWorkspaceAgentInput{ModelID: modelID, Default: true, Enabled: false})
	if err != nil || disabled == nil || disabled.Default || disabled.Enabled {
		t.Fatalf("disable own default = %#v, %v", disabled, err)
	}
	assertPeers()
	if err, _ := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "assistant", ModelID: modelID, Default: true, Enabled: true}); !errors.Is(err, database.ErrWorkspaceAgentExists) {
		t.Fatalf("duplicate default creation = %v", err)
	}
	if err, _ := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, private.ID, 2, database.SystemWorkspaceAgentInput{ModelID: modelID, Default: true, Enabled: true}); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale default update = %v", err)
	}
	assertPeers()
}

func TestWorkspaceAgentDefaultsReconciliation(t *testing.T) {
	testWorkspaceAgentDefaultsReconciliation(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresWorkspaceAgentDefaultsReconciliation(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testWorkspaceAgentDefaultsReconciliation(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testWorkspaceAgentDefaultsReconciliation(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	store, state, workspace, modelID := workspaceAgentDefaultsFixture(t, configuration)
	initial := workspaceAgentDefaultsSnapshot(t, store, workspace)
	err, unmanaged := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "unmanaged", ModelID: modelID, Default: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	err, reviewer := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, initial["reviewer"].ID, 5, database.SystemWorkspaceAgentInput{ModelID: modelID, Enabled: true})
	if err != nil || reviewer == nil || reviewer.Default || reviewer.Revision != 6 {
		t.Fatalf("admin override = %#v, %v", reviewer, err)
	}
	assertBots := func(assistantDefault bool, assistantRevision int, reviewerDefault bool, reviewerRevision, addedRevision int) {
		t.Helper()
		bots := workspaceAgentDefaultsSnapshot(t, store, workspace)
		for alias, want := range map[string]struct {
			defaultValue bool
			revision     int
		}{
			"assistant": {assistantDefault, assistantRevision}, "reviewer": {reviewerDefault, reviewerRevision},
			"private": {false, 2}, "unmanaged": {true, 1}, "added": {true, addedRevision},
		} {
			got, exists := bots[alias]
			if !exists || got.Default != want.defaultValue || got.Revision != want.revision {
				t.Fatalf("%s = %#v, want default %t revision %d", alias, got, want.defaultValue, want.revision)
			}
		}
		if bots["assistant"].ID != initial["assistant"].ID || bots["reviewer"].ID != reviewer.ID || bots["unmanaged"].ID != unmanaged.ID {
			t.Fatal("reconciliation replaced bot identities")
		}
	}
	// A new configured default must not clear either an undeclared default or
	// an existing default whose declaration does not have a newer revision.
	state.WorkspaceAgents[1].Revision = 6
	state.WorkspaceAgents = append(state.WorkspaceAgents, config.WorkspaceAgent{WorkspaceID: state.Workspaces[0].Alias, Alias: "added", ModelAlias: state.AgentModels[0].Alias, Revision: 1, Default: true, Enabled: true})
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertBots(true, 3, false, 6, 1)
	state.WorkspaceAgents[3].Revision = 2
	label := "Updated configured bot"
	state.WorkspaceAgents[3].Label = &label
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertBots(true, 3, false, 6, 2)
	state.WorkspaceAgents[1].Revision = 7
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertBots(true, 3, true, 7, 2)
	state.WorkspaceAgents[0].Default, state.WorkspaceAgents[0].Revision = false, 4
	for range 2 {
		if err := migrateState(ctx, store, configuration, state); err != nil {
			t.Fatal(err)
		}
		assertBots(false, 4, true, 7, 2)
	}
	state.WorkspaceAgents[1].Default, state.WorkspaceAgents[1].Revision = false, 3
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertBots(false, 4, true, 7, 2)
	state.WorkspaceAgents = nil
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertBots(false, 4, true, 7, 2)
	err, groups, _ := store.SystemGroupsSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Name: "team", Limit: 10})
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %#v %v", groups, err)
	}
	err, members, _ := store.SystemGroupAgentsSearch(ctx, database.SystemGroupSearch{Workspace: workspace.Id, Group: groups[0].ID, Limit: 10})
	if err != nil || len(members) != 1 || members[0].Agent != initial["assistant"].ID || members[0].Revision != 4 || !members[0].Enabled {
		t.Fatalf("default edits changed group membership: %#v, %v", members, err)
	}
	p := "?"
	if configuration.Kind == config.DatabaseKindPostgres {
		p = "$1"
	}
	for alias, want := range map[string]int{"assistant": 1, "reviewer": 2, "private": 0, "unmanaged": 0, "added": 1} {
		bots := workspaceAgentDefaultsSnapshot(t, store, workspace)
		var count int
		if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'workspace_agent.update' AND resource_workspace_agent_id = `+p, bots[alias].ID).Scan(&count); err != nil || count != want {
			t.Fatalf("%s update activity = %d, want %d: %v", alias, count, want, err)
		}
	}
}
