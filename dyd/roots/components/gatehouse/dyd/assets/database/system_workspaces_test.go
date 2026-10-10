package database_test

import (
	"context"
	"errors"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"
)

func TestSystemWorkspacesSearchAndCreateWithoutMembership(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	err, workspaces, cursor := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Limit: 50})
	if err != nil || cursor != "" || len(workspaces) < 1 {
		t.Fatalf("SystemWorkspacesSearch() = (%#v, %q, %v)", workspaces, cursor, err)
	}
	initialWorkspaceCount := len(workspaces)
	foundEngineering := false
	var engineeringID string
	for _, workspace := range workspaces {
		if workspace.Alias != nil && *workspace.Alias == "engineering" {
			foundEngineering = true
			engineeringID = workspace.ID
		}
	}
	if !foundEngineering {
		t.Fatalf("SystemWorkspacesSearch() omitted configured workspace: %#v", workspaces)
	}

	name := "  New team  "
	err, created := store.SystemWorkspaceCreate(ctx, "new-team", &name, false)
	if err != nil || created == nil || !typed_id.Valid(typed_id.Workspace, created.ID) || created.Name == nil || *created.Name != "New team" || created.Enabled {
		t.Fatalf("SystemWorkspaceCreate() = (%#v, %v)", created, err)
	}
	if err, _ := store.SystemWorkspaceCreate(ctx, "new-team", nil, true); !errors.Is(err, database.ErrWorkspaceAliasExists) {
		t.Fatalf("duplicate SystemWorkspaceCreate() error = %v", err)
	}
	if err, _ := store.SystemWorkspaceCreate(ctx, "gatehouse", nil, true); !errors.Is(err, database.ErrWorkspaceAliasInvalid) {
		t.Fatalf("reserved-alias SystemWorkspaceCreate() error = %v", err)
	}
	if err, _ := store.SystemWorkspaceCreate(ctx, "Invalid Alias", nil, true); !errors.Is(err, database.ErrWorkspaceAliasInvalid) {
		t.Fatalf("invalid-alias SystemWorkspaceCreate() error = %v", err)
	}
	if err, _ := store.SystemWorkspaceCreate(ctx, "other", stringPointer("  "), true); !errors.Is(err, database.ErrWorkspaceNameInvalid) {
		t.Fatalf("blank-name SystemWorkspaceCreate() error = %v", err)
	}
	for _, query := range []string{"NEW TEAM", "new-team", created.ID} {
		err, matches, next := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Name: query, Limit: 50})
		if err != nil || next != "" || len(matches) != 1 || matches[0].ID != created.ID {
			t.Fatalf("SystemWorkspacesSearch(%q) = (%#v, %q, %v)", query, matches, next, err)
		}
	}
	if err, matches, _ := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Name: "missing", Limit: 50}); err != nil || len(matches) != 0 {
		t.Fatalf("SystemWorkspacesSearch() with no matches = (%#v, %v)", matches, err)
	}
	if err, matches, _ := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Name: "%", Limit: 50}); err != nil || len(matches) != 0 {
		t.Fatalf("SystemWorkspacesSearch() did not escape wildcard characters = (%#v, %v)", matches, err)
	}
	if err, _, _ := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Cursor: "invalid", Limit: 50}); err == nil {
		t.Fatal("SystemWorkspacesSearch() accepted an invalid cursor")
	}
	if err, _, _ := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Limit: 101}); err == nil {
		t.Fatal("SystemWorkspacesSearch() accepted an invalid limit")
	}

	seen := map[string]bool{}
	pageCursor := ""
	for len(seen) < initialWorkspaceCount+1 {
		err, page, next := store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Cursor: pageCursor, Limit: 1})
		if err != nil || len(page) != 1 || seen[page[0].ID] {
			t.Fatalf("keyset SystemWorkspacesSearch() = (%#v, %q, %v)", page, next, err)
		}
		seen[page[0].ID] = true
		pageCursor = next
	}
	if pageCursor != "" {
		t.Fatalf("final workspace search cursor = %q, want empty", pageCursor)
	}
	updatedName := "Updated team"
	enabled := true
	err, updated := store.SystemWorkspaceUpdate(ctx, created.ID, &updatedName, &enabled)
	if err != nil || updated == nil || updated.Alias == nil || *updated.Alias != "new-team" || updated.Name == nil || *updated.Name != updatedName || !updated.Enabled {
		t.Fatalf("SystemWorkspaceUpdate() = (%#v, %v)", updated, err)
	}
	configuredName := "Changed in system administration"
	disabled := false
	if err, configuredWorkspace := store.SystemWorkspaceUpdate(ctx, engineeringID, &configuredName, &disabled); err != nil || configuredWorkspace == nil || configuredWorkspace.Name == nil || *configuredWorkspace.Name != configuredName || configuredWorkspace.Enabled {
		t.Fatalf("configured SystemWorkspaceUpdate() = (%#v, %v)", configuredWorkspace, err)
	}

	var grants int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_workspace_grants WHERE workspace = ?`, created.ID).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("new workspace grants = (%d, %v), want no membership", grants, err)
	}

	configName := "Configuration wins on reconciliation"
	state.Workspaces[0].Name = &configName
	state.Workspaces[0].Enabled = false
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	err, workspaces, cursor = store.SystemWorkspacesSearch(ctx, database.SystemWorkspaceSearch{Limit: 50})
	foundCreated := false
	foundConfigRestored := false
	for _, workspace := range workspaces {
		if workspace.ID == created.ID {
			foundCreated = workspace.Name != nil && *workspace.Name == updatedName && workspace.Enabled
		}
		if workspace.ID == engineeringID {
			foundConfigRestored = workspace.Name != nil && *workspace.Name == configName && !workspace.Enabled
		}
	}
	if err != nil || cursor != "" || len(workspaces) != initialWorkspaceCount+1 || !foundCreated || !foundConfigRestored {
		t.Fatalf("SystemWorkspacesSearch() after config reconciliation = (%#v, %q, %v)", workspaces, cursor, err)
	}
}
