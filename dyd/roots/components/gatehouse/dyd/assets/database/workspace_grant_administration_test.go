package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
)

func TestSystemWorkspaceGrantRoleUpdate(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	principalAlias, groupAlias := "alice", "developers"
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}, {Alias: "other", Enabled: true}},
		Principals: []config.Principal{{Alias: principalAlias, Enabled: true}},
		Groups:     []config.Group{{WorkspaceID: "engineering", Alias: groupAlias, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	otherWorkspace := workspaceRef(t, ctx, store, "other")
	var principal string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	err, member := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principal, authz.Guest)
	if err != nil {
		t.Fatal(err)
	}
	err, manager := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principal, authz.Admin)
	if err != nil {
		t.Fatal(err)
	}
	err, updated := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, member.ID, authz.Member, member.Revision)
	if err != nil || updated == nil || updated.ID != member.ID || updated.Workspace != workspace.Id || updated.Principal == nil || *updated.Principal != principal || !updated.Enabled || updated.Role != authz.Member || updated.Revision != 2 {
		t.Fatalf("role update = (%#v, %v)", updated, err)
	}
	err, page, _ := store.SystemWorkspacePrincipalsSearch(ctx, database.SystemWorkspacePrincipalSearch{WorkspaceID: workspace.Id, Limit: 1})
	if err != nil || len(page) != 1 || len(page[0].Grants) != 2 || len(page[0].Roles) != 2 {
		t.Fatalf("principal grants = (%#v, %v)", page, err)
	}
	for _, grant := range page[0].Grants {
		if grant.Principal != principal || grant.Workspace != workspace.Id || !grant.Enabled {
			t.Fatalf("incorrect grant scope: %#v", grant)
		}
		if grant.ID == member.ID && (grant.Role != authz.Member || grant.Revision != 2) {
			t.Fatalf("stale grant metadata: %#v", grant)
		}
	}
	if err, _ := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, member.ID, authz.Guest, 1); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale update = %v", err)
	}
	if err, _ := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, member.ID, authz.Role("owner"), 2); !errors.Is(err, database.ErrWorkspaceGrantRoleInvalid) {
		t.Fatalf("invalid role = %v", err)
	}
	if err, missing := store.SystemWorkspaceGrantRoleUpdate(ctx, otherWorkspace.Id, member.ID, authz.Guest, 2); err != nil || missing != nil {
		t.Fatalf("wrong workspace = (%#v, %v)", missing, err)
	}
	if err, unchanged := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, member.ID, authz.Member, 2); err != nil || unchanged == nil || unchanged.Revision != 2 {
		t.Fatalf("no-op = (%#v, %v)", unchanged, err)
	}
	for _, disabled := range []bool{false, true} {
		if disabled {
			if _, err := store.Exec(`UPDATE gatehouse_workspace_grants SET enabled = FALSE, revision = revision + 1 WHERE id = ?`, manager.ID); err != nil {
				t.Fatal(err)
			}
		}
		if err, _ := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, member.ID, authz.Admin, 2); !errors.Is(err, database.ErrWorkspaceGrantAlreadyExists) {
			t.Fatalf("role collision (disabled=%t) = %v", disabled, err)
		}
	}
	var role string
	var revision, updates int
	if err := store.QueryRow(`SELECT role, revision FROM gatehouse_workspace_grants WHERE id = ?`, member.ID).Scan(&role, &revision); err != nil || role != "member" || revision != 2 {
		t.Fatalf("collision changed source grant: %s %d %v", role, revision, err)
	}
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE event = 'workspace_grant.update' AND resource_workspace_grant = ?`, member.ID).Scan(&updates); err != nil || updates != 1 {
		t.Fatalf("update activity count = %d: %v", updates, err)
	}
	// Editing a disabled grant must not re-enable it or change its recipient.
	err, disabledGrant := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, manager.ID, authz.Guest, 2)
	if err != nil || disabledGrant == nil || disabledGrant.ID != manager.ID || disabledGrant.Enabled || disabledGrant.Role != authz.Guest || disabledGrant.Revision != 3 {
		t.Fatalf("disabled grant update = (%#v, %v)", disabledGrant, err)
	}
}

func TestSystemWorkspaceGrantRemovalPreservesInterveningRevision(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := migrateState(ctx, store, configuration, config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var principal string
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	err, member := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principal, authz.Guest)
	if err != nil {
		t.Fatal(err)
	}
	err, manager := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, principal, authz.Admin)
	if err != nil {
		t.Fatal(err)
	}
	// Removal snapshots both grants before updating either. On its first activity
	// event, simulate an intervening role edit of the other grant. This exercises
	// the stale-read/live-write interleaving deterministically on SQLite, whose
	// single-connection setup otherwise serializes concurrent transactions.
	_, err = store.Exec(fmt.Sprintf(`
		CREATE TRIGGER simulate_intervening_grant_role_edit
		AFTER INSERT ON gatehouse_activity_events
		WHEN NEW.event = 'workspace_grant.update'
			AND NEW.resource_workspace_grant IN ('%s', '%s')
			AND (SELECT COUNT(*) FROM gatehouse_activity_events
				WHERE event = 'workspace_grant.update'
				AND resource_workspace_grant IN ('%s', '%s')) = 1
		BEGIN
			UPDATE gatehouse_workspace_grants SET role = 'member', revision = revision + 1
			WHERE id IN ('%s', '%s') AND id <> NEW.resource_workspace_grant;
		END;
	`, member.ID, manager.ID, member.ID, manager.ID, member.ID, manager.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err, removed := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, principal); err != nil || !removed {
		t.Fatalf("remove grants = (%t, %v)", removed, err)
	}
	var editedID string
	var revision int
	var enabled bool
	if err := store.QueryRow(`SELECT id, revision, enabled FROM gatehouse_workspace_grants
		WHERE workspace = ? AND principal = ? AND role = 'member'`, workspace.Id, principal).Scan(&editedID, &revision, &enabled); err != nil || revision != 3 || enabled {
		t.Fatalf("grant after intervening edit and removal = (%s, %d, %t, %v), want revision 3 and disabled", editedID, revision, enabled, err)
	}
	if err, _ := store.SystemWorkspaceGrantRoleUpdate(ctx, workspace.Id, editedID, authz.Member, 2); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("edit with pre-removal revision = %v, want revision conflict", err)
	}
}
