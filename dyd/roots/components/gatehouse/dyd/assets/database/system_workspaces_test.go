package database_test

import (
	"context"
	"errors"
	"testing"

	"gatehouse/authz"
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

func TestSystemWorkspacePrincipalManagementIsPaginatedAndRevisioned(t *testing.T) {
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
	workspace := workspaceRef(t, ctx, store, "engineering")

	type testPrincipal struct {
		id    string
		alias string
	}
	principals := []testPrincipal{}
	for _, alias := range []string{"alice", "bob", "carol"} {
		id, err := typed_id.New(typed_id.Principal)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_principals (id, alias, name, revision, enabled) VALUES (?, ?, ?, 1, TRUE)`, id, alias, alias); err != nil {
			t.Fatal(err)
		}
		principals = append(principals, testPrincipal{id: id, alias: alias})
	}
	for index, principal := range principals {
		role := authz.Member
		if index == 1 {
			role = authz.Contributor
		}
		if err, grant := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principal.id, role); err != nil || grant == nil || !grant.Enabled || grant.Revision != 1 || grant.Role != role {
			t.Fatalf("SystemWorkspacePrincipalGrantCreate(%q) = (%#v, %v)", principal.alias, grant, err)
		}
	}

	seen := map[string]bool{}
	cursor := ""
	for {
		err, page, next := store.SystemWorkspacePrincipalsSearch(ctx, database.SystemWorkspacePrincipalSearch{WorkspaceID: workspace.Id, Cursor: cursor, Limit: 1})
		if err != nil || len(page) != 1 || seen[page[0].ID] {
			t.Fatalf("SystemWorkspacePrincipalsSearch(cursor=%q) = (%#v, %q, %v)", cursor, page, next, err)
		}
		seen[page[0].ID] = true
		if page[0].ID == principals[1].id && (len(page[0].Roles) != 1 || page[0].Roles[0] != authz.Contributor) {
			t.Fatalf("workspace roles for bob = %#v", page[0].Roles)
		}
		cursor = next
		if cursor == "" {
			break
		}
	}
	if len(seen) != len(principals) {
		t.Fatalf("paginated workspace principal IDs = %#v, want %d principals", seen, len(principals))
	}

	err, options, optionCursor := store.SystemWorkspacePrincipalOptionsSearch(ctx, database.SystemWorkspacePrincipalSearch{WorkspaceID: workspace.Id, Name: "ali", Limit: 1})
	if err != nil || len(options) != 1 || options[0].ID != principals[0].id || options[0].Name == nil || *options[0].Name != "alice" || optionCursor != "" {
		t.Fatalf("SystemWorkspacePrincipalOptionsSearch() = (%#v, %q, %v)", options, optionCursor, err)
	}
	if err, _, _ := store.SystemWorkspacePrincipalsSearch(ctx, database.SystemWorkspacePrincipalSearch{WorkspaceID: workspace.Id, Cursor: "invalid", Limit: 1}); err == nil {
		t.Fatal("SystemWorkspacePrincipalsSearch() accepted an invalid cursor")
	}

	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principals[0].id, authz.Member); !errors.Is(err, database.ErrWorkspaceGrantAlreadyExists) {
		t.Fatalf("duplicate workspace grant error = %v", err)
	}
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principals[0].id, authz.Role("owner")); !errors.Is(err, database.ErrWorkspaceGrantRoleInvalid) {
		t.Fatalf("invalid workspace role error = %v", err)
	}

	if err, removed := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, principals[0].id); err != nil || !removed {
		t.Fatalf("SystemWorkspacePrincipalGrantsRemove() = (%v, %v)", err, removed)
	}
	var enabled bool
	var revision int
	if err := store.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_workspace_grants WHERE workspace = ? AND principal = ?`, workspace.Id, principals[0].id).Scan(&enabled, &revision); err != nil || enabled || revision != 2 {
		t.Fatalf("removed workspace grant state = (%v, %d, %v)", enabled, revision, err)
	}
	if err, grant := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principals[0].id, authz.Member); err != nil || grant == nil || !grant.Enabled || grant.Revision != 3 {
		t.Fatalf("re-add workspace grant = (%#v, %v)", grant, err)
	}

	// A runtime removal advances the configured revision, so an unchanged
	// repeatable config reconciliation does not silently restore access.
	configuredAlias := principals[2].alias
	state.WorkspaceGrants = []config.WorkspaceGrant{{WorkspaceID: "engineering", Role: string(authz.Member), PrincipalID: &configuredAlias, Revision: 8, Enabled: true}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err, removed := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, principals[2].id); err != nil || !removed {
		t.Fatalf("remove configured workspace principal = (%v, %v)", err, removed)
	}
	if err := store.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_workspace_grants WHERE workspace = ? AND principal = ?`, workspace.Id, principals[2].id).Scan(&enabled, &revision); err != nil || enabled || revision != 9 {
		t.Fatalf("configured removed grant state = (%v, %d, %v)", enabled, revision, err)
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_workspace_grants WHERE workspace = ? AND principal = ?`, workspace.Id, principals[2].id).Scan(&enabled, &revision); err != nil || enabled || revision != 9 {
		t.Fatalf("configured removed grant after unchanged reconciliation = (%v, %d, %v)", enabled, revision, err)
	}
	var activities int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE event IN ('workspace_grant.create', 'workspace_grant.update')`).Scan(&activities); err != nil || activities != 7 {
		t.Fatalf("workspace grant activity count = (%d, %v), want 7", activities, err)
	}
}
