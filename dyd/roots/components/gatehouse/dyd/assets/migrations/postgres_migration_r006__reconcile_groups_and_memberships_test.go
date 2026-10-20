package migrations

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"
)

type rejectGroupSnapshotQueryer struct{}

// Exercise the metadata and post-admission membership phases together, in the
// same order as the registered reconciliation plan.
func postgresGroupReconciliationPlan(state config.State) RepeatableMigration {
	metadata := postgresMigrationR006ReconcileGroupsAndMemberships(state)
	members := postgresMigrationR016ReconcileGroupMembers(state.Groups)
	metadata.Builder = func(ctx context.Context, session *MigrationSession) (error, string) {
		err, groups := postgresMigrationR006ReconcileGroupsAndMembershipsBuilder(state.Groups)(ctx, session)
		if err != nil {
			return err, ""
		}
		err, memberships := members.Builder(ctx, session)
		return err, groups + "\n" + memberships
	}
	return metadata
}

func (rejectGroupSnapshotQueryer) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("group reconciliation must resolve database state during execution")
}

func TestPostgresGroupReconciliationBuildIsSnapshotIndependent(t *testing.T) {
	ctx := context.Background()
	migration := postgresGroupReconciliationPlan(config.State{Groups: []config.Group{{
		WorkspaceID: "team", Alias: "developers", Enabled: true, Revision: 3,
		Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true, Revision: 4}},
	}}})
	session := &MigrationSession{queryer: rejectGroupSnapshotQueryer{}}
	err, first := migration.Builder(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	err, second := migration.Builder(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("identical configuration changed the repeatable migration source")
	}
}

func TestPostgresGroupReconciliationExecutionTimeIdentity(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_GROUP_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_GROUP_POSTGRES_URL"})
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
	for _, scenario := range []string{"create after build", "rename after build", "replace alias after build"} {
		t.Run(scenario, func(t *testing.T) {
			unique, err := typed_id.New(typed_id.Workspace)
			if err != nil {
				t.Fatal(err)
			}
			workspaceAlias, principalAlias := "race-"+unique, "member-"+unique
			err, workspace := store.SystemWorkspaceCreate(ctx, workspaceAlias, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			err, principal := store.SystemPrincipalCreate(ctx, database.SystemPrincipalCreateInput{Alias: &principalAlias, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.ID, principal.ID, "member"); err != nil {
				t.Fatal(err)
			}
			alias, adminName, configuredName := "configured", "Admin group", "Configured group"
			var original, target *database.SystemGroup
			if scenario != "create after build" {
				err, original = store.SystemGroupCreate(ctx, workspace.ID, database.SystemGroupInput{Alias: &alias, Name: &adminName})
				if err != nil {
					t.Fatal(err)
				}
			}
			migration := postgresGroupReconciliationPlan(config.State{Groups: []config.Group{{
				WorkspaceID: workspaceAlias, Alias: alias, Name: &configuredName, Enabled: true, Revision: 3,
				Members: []config.GroupMember{{PrincipalID: principalAlias, Enabled: true, Revision: 4}},
			}}})
			// Reproduce the exact race window: prepare the migration, then commit
			// an admin change on another connection before executing its SQL.
			err, source := migration.Builder(ctx, &MigrationSession{queryer: store.DB})
			if err != nil {
				t.Fatal(err)
			}
			if original != nil {
				renamed := "renamed"
				if err, _ := store.SystemGroupUpdate(ctx, workspace.ID, original.ID, 1, database.SystemGroupInput{Alias: &renamed}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "rename after build" {
				err, target = store.SystemGroupCreate(ctx, workspace.ID, database.SystemGroupInput{Alias: &alias, Name: &adminName})
				if err != nil {
					t.Fatal(err)
				}
			}
			apply := func() {
				t.Helper()
				tx, err := store.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if _, err := tx.ExecContext(ctx, source); err != nil {
					t.Fatalf("apply prepared migration after admin change: %v", err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			apply()
			var actualID, actualName string
			var revision int
			if err := store.QueryRowContext(ctx, `SELECT id, name, revision FROM gatehouse_groups WHERE workspace_id = $1 AND alias = $2`, workspace.ID, alias).Scan(&actualID, &actualName, &revision); err != nil || actualName != configuredName || revision != 3 {
				t.Fatalf("reconciled group = %s %s %d %v", actualID, actualName, revision, err)
			}
			if target != nil && actualID != target.ID {
				t.Fatalf("replaced admin group identity: got %s, want %s", actualID, target.ID)
			}
			if original != nil {
				err, preserved := store.SystemGroupGet(ctx, workspace.ID, original.ID)
				if err != nil || preserved == nil || preserved.ID == actualID || preserved.Revision != 2 || preserved.Alias == nil || *preserved.Alias != "renamed" || preserved.Name == nil || *preserved.Name != adminName {
					t.Fatalf("renamed group changed: %#v %v", preserved, err)
				}
			}
			var memberGroup string
			if err := store.QueryRowContext(ctx, `SELECT group_id, revision FROM gatehouse_group_members WHERE workspace_id = $1 AND principal_id = $2`, workspace.ID, principal.ID).Scan(&memberGroup, &revision); err != nil || memberGroup != actualID || revision != 4 {
				t.Fatalf("membership used stale group identity: %s %d %v", memberGroup, revision, err)
			}
			var activities int
			groupEvent := "group.update"
			if target == nil {
				groupEvent = "group.create"
			}
			var activityID string
			var createdAt time.Time
			if err := store.QueryRowContext(ctx, `SELECT id, created_at FROM gatehouse_activity_events WHERE resource_group = $1 AND event = $2`, actualID, groupEvent).Scan(&activityID, &createdAt); err != nil {
				t.Fatalf("missing activity for the actual group: %v", err)
			}
			idTime, err := typed_id.Timestamp(typed_id.ActivityEvent, activityID)
			if err != nil || !idTime.Equal(createdAt) {
				t.Fatalf("activity timestamp does not match its typed ID: %s %s %v", idTime, createdAt, err)
			}
			if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_group_member_group = $1 AND resource_group_member_principal = $2 AND event = 'group_member.create'`, actualID, principal.ID).Scan(&activities); err != nil || activities != 1 {
				t.Fatalf("membership activity = %d %v", activities, err)
			}
			// Re-executing the same prepared source must be a no-op, including audit.
			apply()
			if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_group_member_group = $1 AND resource_group_member_principal = $2`, actualID, principal.ID).Scan(&activities); err != nil || activities != 1 {
				t.Fatalf("replay emitted activity = %d %v", activities, err)
			}
			if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_group = $1 AND event = $2`, actualID, groupEvent).Scan(&activities); err != nil || activities != 1 {
				t.Fatalf("replay emitted group activity = %d %v", activities, err)
			}
		})
	}
}

func TestPostgresGroupReconciliationWaitsForConcurrentCreation(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_GROUP_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_GROUP_POSTGRES_URL"})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); store.Close() }()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	if err := migratePostgres(ctx, store.DB, registry); err != nil {
		t.Fatal(err)
	}
	unique, err := typed_id.New(typed_id.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	workspaceAlias, principalAlias := "wait-"+unique, "member-"+unique
	err, workspace := store.SystemWorkspaceCreate(ctx, workspaceAlias, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	err, principal := store.SystemPrincipalCreate(ctx, database.SystemPrincipalCreateInput{Alias: &principalAlias, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.ID, principal.ID, "member"); err != nil {
		t.Fatal(err)
	}
	migration := postgresGroupReconciliationPlan(config.State{Groups: []config.Group{{
		WorkspaceID: workspaceAlias, Alias: "configured", Enabled: true, Revision: 3,
		Members: []config.GroupMember{{PrincipalID: principalAlias, Enabled: true, Revision: 1}},
	}}})
	err, source := migration.Builder(ctx, &MigrationSession{queryer: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var migrationPID, actorPID int
	if err := connection.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&migrationPID); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	actor, err := store.BeginTx(ctx, nil)
	if err != nil {
		connection.Close()
		t.Fatal(err)
	}
	defer func() { actor.Rollback(); cancel(); connection.Close() }()
	if err := actor.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&actorPID); err != nil {
		t.Fatal(err)
	}
	groupID, err := typed_id.New(typed_id.Group)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the same write lock as an admin group creation until reconciliation
	// is actually waiting for it, rather than relying on goroutine timing.
	if _, err := actor.ExecContext(ctx, `INSERT INTO gatehouse_groups (workspace_id, id, alias, enabled, revision) VALUES ($1, $2, 'configured', TRUE, 1)`, workspace.ID, groupID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		tx, err := connection.BeginTx(ctx, nil)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, source); err != nil {
			done <- err
			return
		}
		done <- tx.Commit()
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("reconciliation did not wait for the admin transaction: %v", err)
		default:
		}
		var blocked bool
		if err := store.QueryRowContext(ctx, `SELECT $1 = ANY(pg_blocking_pids($2))`, actorPID, migrationPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := actor.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("reconciliation after concurrent creation committed: %v", err)
	}
	var actualID string
	if err := store.QueryRowContext(ctx, `SELECT group_id FROM gatehouse_group_members WHERE workspace_id = $1 AND principal_id = $2`, workspace.ID, principal.ID).Scan(&actualID); err != nil || actualID != groupID {
		t.Fatalf("concurrent creation identity = %s %v, want %s", actualID, err, groupID)
	}
}
