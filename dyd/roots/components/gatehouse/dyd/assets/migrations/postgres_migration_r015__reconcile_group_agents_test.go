package migrations

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"
)

func TestPostgresGroupAgentReconciliationSourceIsSnapshotIndependent(t *testing.T) {
	migration := postgresMigrationR015ReconcileGroupAgents(config.State{Groups: []config.Group{{WorkspaceID: "engineering", Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 3}}}}})
	session := &MigrationSession{}
	err, first := migration.Builder(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	err, second := migration.Builder(context.Background(), session)
	if err != nil || first != second {
		t.Fatalf("unstable reconciliation source: %v", err)
	}
}

type postgresGroupAgentMigrationFixture struct {
	store                                        *database.Store
	ctx                                          context.Context
	workspace, workspaceAlias, group, bot, model string
}

func newPostgresGroupAgentMigrationFixture(t *testing.T, subjects bool) *postgresGroupAgentMigrationFixture {
	t.Helper()
	if os.Getenv("GATEHOUSE_TEST_GROUP_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_GROUP_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	if err := migratePostgres(ctx, store.DB, registry); err != nil {
		t.Fatal(err)
	}
	newID := func(kind string) string {
		t.Helper()
		id, err := typed_id.New(kind)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	alias := "bot-migration-" + newID(typed_id.Workspace)
	err, workspace := store.SystemWorkspaceCreate(ctx, alias, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	provider, model := newID(typed_id.AgentProvider), newID(typed_id.AgentModel)
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ($1, 1, 'builtin', TRUE)`, provider); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES ($1, 1, $2, 'dummy.fixed-reply', '{}', TRUE)`, model, provider); err != nil {
		t.Fatal(err)
	}
	fixture := &postgresGroupAgentMigrationFixture{store: store, ctx: ctx, workspace: workspace.ID, workspaceAlias: alias, model: model}
	if subjects {
		groupAlias := "developers"
		err, group := store.SystemGroupCreate(ctx, workspace.ID, database.SystemGroupInput{Alias: &groupAlias})
		if err != nil {
			t.Fatal(err)
		}
		err, bot := store.SystemWorkspaceAgentCreate(ctx, workspace.ID, database.SystemWorkspaceAgentInput{Alias: "assistant", ModelID: model, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		fixture.group, fixture.bot = group.ID, bot.ID
	}
	return fixture
}

func (fixture *postgresGroupAgentMigrationFixture) apply(source string) error {
	tx, err := fixture.store.BeginTx(fixture.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(fixture.ctx, source); err != nil {
		return err
	}
	return tx.Commit()
}

func (fixture *postgresGroupAgentMigrationFixture) assertMembership(t *testing.T, group, bot string, expectedRevision int, expectedEnabled bool) {
	t.Helper()
	var revision int
	var enabled bool
	err := fixture.store.QueryRowContext(fixture.ctx, `SELECT revision, enabled FROM gatehouse_group_agents WHERE workspace_id = $1 AND group_id = $2 AND agent_id = $3`, fixture.workspace, group, bot).Scan(&revision, &enabled)
	if err != nil || revision != expectedRevision || enabled != expectedEnabled {
		t.Fatalf("membership = revision %d enabled %t, %v; want revision %d enabled %t", revision, enabled, err, expectedRevision, expectedEnabled)
	}
}

func (fixture *postgresGroupAgentMigrationFixture) activityCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := fixture.store.QueryRowContext(fixture.ctx, `SELECT COUNT(*) FROM gatehouse_activity_events events JOIN gatehouse_groups groups ON groups.id = events.resource_group_agent_group WHERE groups.workspace_id = $1`, fixture.workspace).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPostgresGroupAgentReconciliationReportsUnresolvedAliases(t *testing.T) {
	for _, scenario := range []string{"workspace", "group", "bot", "foreign bot"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPostgresGroupAgentMigrationFixture(t, true)
			workspace, group, bot, reason := fixture.workspaceAlias, "developers", "assistant", ""
			switch scenario {
			case "workspace":
				workspace, reason = "missing", "workspace is unavailable"
			case "group":
				group, reason = "missing", "group is unavailable in this workspace"
			case "bot":
				bot, reason = "missing", "bot is unavailable in this workspace"
			case "foreign bot":
				err, foreign := fixture.store.SystemWorkspaceCreate(fixture.ctx, "other-"+fixture.workspaceAlias, nil, true)
				if err != nil {
					t.Fatal(err)
				}
				if err, _ := fixture.store.SystemWorkspaceAgentCreate(fixture.ctx, foreign.ID, database.SystemWorkspaceAgentInput{Alias: "elsewhere", ModelID: fixture.model, Enabled: true}); err != nil {
					t.Fatal(err)
				}
				bot, reason = "elsewhere", "bot is unavailable in this workspace"
			}
			migration := postgresMigrationR015ReconcileGroupAgents(config.State{Groups: []config.Group{
				{WorkspaceID: fixture.workspaceAlias, Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 3}}},
				{WorkspaceID: workspace, Alias: group, Agents: []config.GroupAgent{{AgentID: bot, Enabled: true, Revision: 3}}},
			}})
			err, source := migration.Builder(fixture.ctx, &MigrationSession{})
			if err != nil {
				t.Fatal(err)
			}
			executionErr := fixture.apply(source)
			for _, detail := range []string{"cannot reconcile group bot membership", "workspace '" + workspace + "'", "group '" + group + "'", "bot '" + bot + "'", reason} {
				if executionErr == nil || !strings.Contains(executionErr.Error(), detail) {
					t.Fatalf("migration error = %v, want %q", executionErr, detail)
				}
			}
			var count int
			if err := fixture.store.QueryRowContext(fixture.ctx, `SELECT COUNT(*) FROM gatehouse_group_agents WHERE workspace_id = $1`, fixture.workspace).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed validation wrote memberships: %d %v", count, err)
			}
			if count := fixture.activityCount(t); count != 0 {
				t.Fatalf("failed validation wrote %d activity events", count)
			}
		})
	}
}

func TestPostgresGroupAgentReconciliationExecutionTimeIdentity(t *testing.T) {
	for _, scenario := range []string{"create after preparation", "replace group alias", "replace bot alias"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPostgresGroupAgentMigrationFixture(t, scenario != "create after preparation")
			if fixture.group != "" {
				if err := fixture.store.SystemGroupAgentCreate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot); err != nil {
					t.Fatal(err)
				}
				if err := fixture.store.SystemGroupAgentUpdate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot, false, 1); err != nil {
					t.Fatal(err)
				}
			}
			migration := postgresMigrationR015ReconcileGroupAgents(config.State{Groups: []config.Group{{WorkspaceID: fixture.workspaceAlias, Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 3}}}}})
			err, source := migration.Builder(fixture.ctx, &MigrationSession{})
			if err != nil {
				t.Fatal(err)
			}
			targetGroup, targetBot := fixture.group, fixture.bot
			if scenario == "replace group alias" {
				renamed := "renamed"
				if err, _ := fixture.store.SystemGroupUpdate(fixture.ctx, fixture.workspace, fixture.group, 1, database.SystemGroupInput{Alias: &renamed}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "replace bot alias" {
				if _, err := fixture.store.ExecContext(fixture.ctx, `UPDATE gatehouse_workspace_agents SET alias = 'renamed', revision = revision + 1 WHERE workspace_id = $1 AND id = $2`, fixture.workspace, fixture.bot); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "replace bot alias" {
				alias := "developers"
				err, group := fixture.store.SystemGroupCreate(fixture.ctx, fixture.workspace, database.SystemGroupInput{Alias: &alias})
				if err != nil {
					t.Fatal(err)
				}
				targetGroup = group.ID
			}
			if scenario != "replace group alias" {
				err, bot := fixture.store.SystemWorkspaceAgentCreate(fixture.ctx, fixture.workspace, database.SystemWorkspaceAgentInput{Alias: "assistant", ModelID: fixture.model, Enabled: true})
				if err != nil {
					t.Fatal(err)
				}
				targetBot = bot.ID
			}
			before := fixture.activityCount(t)
			for range 2 {
				if err := fixture.apply(source); err != nil {
					t.Fatal(err)
				}
				fixture.assertMembership(t, targetGroup, targetBot, 3, true)
				if fixture.group != "" {
					fixture.assertMembership(t, fixture.group, fixture.bot, 2, false)
				}
				if count := fixture.activityCount(t); count != before+1 {
					t.Fatalf("migration/replay activity = %d, want %d", count, before+1)
				}
			}
			var activity string
			var at time.Time
			if err := fixture.store.QueryRowContext(fixture.ctx, `SELECT id, created_at FROM gatehouse_activity_events WHERE resource_group_agent_group = $1 AND resource_group_agent_agent = $2`, targetGroup, targetBot).Scan(&activity, &at); err != nil {
				t.Fatal(err)
			}
			idTime, err := typed_id.Timestamp(typed_id.ActivityEvent, activity)
			if err != nil || !idTime.Equal(at) {
				t.Fatalf("activity time = %s, ID time = %s, %v", at, idTime, err)
			}
		})
	}
}

func TestPostgresGroupAgentReconciliationHonorsPostPreparationAdminRevisions(t *testing.T) {
	for _, scenario := range []string{"equal revision", "higher revision", "membership created after preparation"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPostgresGroupAgentMigrationFixture(t, true)
			if scenario != "membership created after preparation" {
				if err := fixture.store.SystemGroupAgentCreate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot); err != nil {
					t.Fatal(err)
				}
			}
			migration := postgresMigrationR015ReconcileGroupAgents(config.State{Groups: []config.Group{{WorkspaceID: fixture.workspaceAlias, Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 2}}}}})
			err, source := migration.Builder(fixture.ctx, &MigrationSession{})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "membership created after preparation" {
				if err := fixture.store.SystemGroupAgentCreate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot); err != nil {
					t.Fatal(err)
				}
				before := fixture.activityCount(t)
				for range 2 {
					if err := fixture.apply(source); err != nil {
						t.Fatal(err)
					}
					fixture.assertMembership(t, fixture.group, fixture.bot, 2, true)
					if count := fixture.activityCount(t); count != before+1 {
						t.Fatalf("post-creation update/replay activity = %d, want %d", count, before+1)
					}
				}
				var creates int
				if err := fixture.store.QueryRowContext(fixture.ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_group_agent_group = $1 AND event = 'group_agent.create'`, fixture.group).Scan(&creates); err != nil || creates != 1 {
					t.Fatalf("migration duplicated creation activity: %d %v", creates, err)
				}
				return
			}
			if err := fixture.store.SystemGroupAgentUpdate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot, false, 1); err != nil {
				t.Fatal(err)
			}
			expectedRevision := 2
			if scenario == "higher revision" {
				if err := fixture.store.SystemGroupAgentUpdate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot, true, 2); err != nil {
					t.Fatal(err)
				}
				if err := fixture.store.SystemGroupAgentUpdate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot, false, 3); err != nil {
					t.Fatal(err)
				}
				expectedRevision = 4
			}
			before := fixture.activityCount(t)
			for range 2 {
				if err := fixture.apply(source); err != nil {
					t.Fatal(err)
				}
				fixture.assertMembership(t, fixture.group, fixture.bot, expectedRevision, false)
				if count := fixture.activityCount(t); count != before {
					t.Fatalf("stale configuration emitted activity: %d, want %d", count, before)
				}
			}
		})
	}
}

func TestPostgresGroupAgentReconciliationDiagnosesAliasRemovedAfterPreparation(t *testing.T) {
	fixture := newPostgresGroupAgentMigrationFixture(t, true)
	migration := postgresMigrationR015ReconcileGroupAgents(config.State{Groups: []config.Group{{WorkspaceID: fixture.workspaceAlias, Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 1}}}}})
	err, source := migration.Builder(fixture.ctx, &MigrationSession{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ExecContext(fixture.ctx, `UPDATE gatehouse_workspace_agents SET alias = 'renamed' WHERE workspace_id = $1 AND id = $2`, fixture.workspace, fixture.bot); err != nil {
		t.Fatal(err)
	}
	if err := fixture.apply(source); err == nil || !strings.Contains(err.Error(), "bot 'assistant': bot is unavailable in this workspace") {
		t.Fatalf("missing post-preparation alias error = %v", err)
	}
	if count := fixture.activityCount(t); count != 0 {
		t.Fatalf("failed validation emitted %d membership events", count)
	}
}

func TestPostgresGroupAgentReconciliationWaitsForConcurrentMembershipUpdate(t *testing.T) {
	fixture := newPostgresGroupAgentMigrationFixture(t, true)
	if err := fixture.store.SystemGroupAgentCreate(fixture.ctx, fixture.workspace, fixture.group, fixture.bot); err != nil {
		t.Fatal(err)
	}
	migration := postgresMigrationR015ReconcileGroupAgents(config.State{Groups: []config.Group{{WorkspaceID: fixture.workspaceAlias, Alias: "developers", Agents: []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 2}}}}})
	err, source := migration.Builder(fixture.ctx, &MigrationSession{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(fixture.ctx, 30*time.Second)
	defer cancel()
	connection, err := fixture.store.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var migrationPID, actorPID int
	if err := connection.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&migrationPID); err != nil {
		t.Fatal(err)
	}
	actor, err := fixture.store.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Rollback()
	if err := actor.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&actorPID); err != nil {
		t.Fatal(err)
	}
	if _, err := actor.ExecContext(ctx, `UPDATE gatehouse_group_agents SET enabled = FALSE, revision = 3 WHERE workspace_id = $1 AND group_id = $2 AND agent_id = $3`, fixture.workspace, fixture.group, fixture.bot); err != nil {
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
			t.Fatalf("migration did not wait for the writer: %v", err)
		default:
		}
		var blocked bool
		if err := fixture.store.QueryRowContext(ctx, `SELECT $1 = ANY(pg_blocking_pids($2))`, actorPID, migrationPID).Scan(&blocked); err != nil {
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
		t.Fatal(err)
	}
	fixture.assertMembership(t, fixture.group, fixture.bot, 3, false)
	if count := fixture.activityCount(t); count != 1 {
		t.Fatalf("migration overwrote concurrent admin state or emitted activity: %d", count)
	}
}
