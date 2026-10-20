package migrations

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"
)

type rejectWorkspaceGrantSnapshotQueryer struct{}

func (rejectWorkspaceGrantSnapshotQueryer) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("workspace grant reconciliation must resolve state during execution")
}

func TestPostgresWorkspaceGrantReconciliationBuildIsSnapshotIndependent(t *testing.T) {
	principal := "alice"
	migration := postgresMigrationR007ReconcileWorkspaceGrants(config.State{WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: "team", PrincipalID: &principal, Role: "contributor", Enabled: true, Revision: 3}}})
	session := &MigrationSession{queryer: rejectWorkspaceGrantSnapshotQueryer{}}
	err, first := migration.Builder(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	err, second := migration.Builder(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("identical workspace grant configuration produced different migration checksums")
	}
}

func TestPostgresWorkspaceGrantReconciliationUsesExecutionTimeState(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_WORKSPACE_GRANTS_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_WORKSPACE_GRANTS_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	if err := migratePostgres(ctx, store.DB, registry); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"two prepared sources", "admin creates grant after build", "admin advances revision after build"} {
		t.Run(scenario, func(t *testing.T) {
			id, err := typed_id.New(typed_id.Workspace)
			if err != nil {
				t.Fatal(err)
			}
			workspaceAlias, principalAlias := "grant-"+id, "member-"+id
			// Build before even the parent identities exist: configuration is the
			// complete source, and the database is consulted only when executing it.
			desired := config.WorkspaceGrant{WorkspaceID: workspaceAlias, PrincipalID: &principalAlias, Role: "contributor", Enabled: true, Revision: 2}
			if scenario == "admin advances revision after build" {
				desired.Enabled = false
			}
			builder := postgresMigrationR007ReconcileWorkspaceGrantsBuilder([]config.WorkspaceGrant{desired})
			session := &MigrationSession{queryer: rejectWorkspaceGrantSnapshotQueryer{}}
			err, first := builder(ctx, session)
			if err != nil {
				t.Fatal(err)
			}
			err, second := builder(ctx, session)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatal("prepared grant sources differ")
			}
			err, workspace := store.SystemWorkspaceCreate(ctx, workspaceAlias, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			err, principal := store.SystemPrincipalCreate(ctx, database.SystemPrincipalCreateInput{Alias: &principalAlias, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			var existingID string
			wantRevision, wantEvents := 2, 1
			if scenario != "two prepared sources" {
				err, grant := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.ID, principal.ID, "contributor")
				if err != nil {
					t.Fatal(err)
				}
				existingID = grant.ID
				wantEvents = 2
				if scenario == "admin advances revision after build" {
					disabled, enabled := false, true
					if err, _ := store.SystemWorkspaceGrantUpdate(ctx, workspace.ID, grant.ID, nil, &disabled, 1); err != nil {
						t.Fatal(err)
					}
					if err, _ := store.SystemWorkspaceGrantUpdate(ctx, workspace.ID, grant.ID, nil, &enabled, 2); err != nil {
						t.Fatal(err)
					}
					wantRevision, wantEvents = 3, 3
				}
			}
			for _, source := range []string{first, second} {
				tx, err := store.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tx.ExecContext(ctx, source); err != nil {
					tx.Rollback()
					t.Fatalf("execute prepared source: %v", err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				var actualID string
				var revision int
				var enabled bool
				if err := store.QueryRowContext(ctx, `SELECT id, revision, enabled FROM gatehouse_workspace_grants WHERE workspace = $1 AND principal = $2 AND role = 'contributor'`, workspace.ID, principal.ID).Scan(&actualID, &revision, &enabled); err != nil {
					t.Fatal(err)
				}
				if existingID == "" {
					existingID = actualID
				}
				if actualID != existingID || revision != wantRevision || !enabled {
					t.Fatalf("prepared source replaced current grant: id=%s revision=%d enabled=%t", actualID, revision, enabled)
				}
				var events, topics int
				if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_workspace_grant = $1`, actualID).Scan(&events); err != nil {
					t.Fatal(err)
				}
				if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_event_topics topics JOIN gatehouse_activity_events events ON events.id = topics.activity WHERE events.resource_workspace_grant = $1`, actualID).Scan(&topics); err != nil {
					t.Fatal(err)
				}
				if events != wantEvents || topics != wantEvents*2 {
					t.Fatalf("phantom or missing grant audit: events=%d topics=%d, want %d/%d", events, topics, wantEvents, wantEvents*2)
				}
			}
		})
	}
}
