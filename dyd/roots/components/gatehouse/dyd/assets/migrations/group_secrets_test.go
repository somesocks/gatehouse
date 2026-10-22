package migrations

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
)

func TestSQLiteGroupSecretsUpgrade(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		name := "versioned"
		if baseline { name = "baseline" }
		t.Run(name, func(t *testing.T) {
			db := openConfiguredMigrationTestDatabase(t)
			registry := testSQLiteBaselineRegistry(t)
			if !baseline { registry.Baseline = nil }
			testGroupSecretsUpgrade(t, db, registry, migrateSQLite)
			rows, err := db.Query(`PRAGMA foreign_key_check`)
			if err != nil { t.Fatal(err) }
			defer rows.Close()
			if rows.Next() { t.Fatal("group secrets upgrade left foreign key violations") }
			if err := rows.Err(); err != nil { t.Fatal(err) }
		})
	}
}

func TestPostgresGroupSecretsUpgrade(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_GROUP_SECRETS_UPGRADE_POSTGRES_URL") == "" { t.Skip("requires disposable PostgreSQL database") }
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_GROUP_SECRETS_UPGRADE_POSTGRES_URL"})
	if err != nil { t.Fatal(err) }
	defer store.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil { t.Fatal(err) }
	testGroupSecretsUpgrade(t, store.DB, registry, migratePostgres)
}

func testGroupSecretsUpgrade(t *testing.T, db *sql.DB, registry Registry, migrate func(context.Context, *sql.DB, Registry) error) {
	t.Helper()
	ctx := context.Background()
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 67)
	if err := migrate(ctx, db, old); err != nil { t.Fatal(err) }
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE), ('wsp_00000000000000000000000001', TRUE);
		INSERT INTO gatehouse_principals (id, enabled, revision) VALUES ('prn_00000000000000000000000000', TRUE, 1);
		INSERT INTO gatehouse_groups (workspace_id, id, enabled) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group, created_at) VALUES ('act_00000000000000000000000000', 'group.create', 'group', 'grp_00000000000000000000000000', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_event_topics (activity, topic) VALUES ('act_00000000000000000000000000', 'wsp_00000000000000000000000000/grp_00000000000000000000000000');
	`); err != nil { t.Fatal(err) }
	for range 2 {
		if err := migrate(ctx, db, registry); err != nil { t.Fatal(err) }
	}
	var topic string
	if err := db.QueryRow(`SELECT topic FROM gatehouse_activity_event_topics JOIN gatehouse_activity_events ON activity = id WHERE resource_kind = 'group'`).Scan(&topic); err != nil || topic != "wsp_00000000000000000000000000/grp_00000000000000000000000000" { t.Fatalf("existing history changed: %q %v", topic, err) }
	if _, err := db.Exec(`
		INSERT INTO gatehouse_group_secrets (workspace, "group", id, author_principal, description, ciphertext, enabled, revision, created_at, updated_at) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'gsc_00000000000000000000000000', 'prn_00000000000000000000000000', 'Credential', 'encrypted-value', TRUE, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_secret, created_at) VALUES ('act_00000000000000000000000001', 'group_secret.create', 'group_secret', 'gsc_00000000000000000000000000', '2026-01-01T00:00:00Z');
	`); err != nil { t.Fatal(err) }
	for _, invalid := range []string{
		`UPDATE gatehouse_group_secrets SET workspace = 'wsp_00000000000000000000000001'`,
		`UPDATE gatehouse_group_secrets SET revision = 0`,
		`UPDATE gatehouse_group_secrets SET ciphertext = ''`,
		`UPDATE gatehouse_group_secrets SET id = 'invalid'`,
		`UPDATE gatehouse_activity_events SET resource_group = 'grp_00000000000000000000000000' WHERE resource_kind = 'group_secret'`,
		`UPDATE gatehouse_activity_events SET resource_group_secret = NULL WHERE resource_kind = 'group_secret'`,
	} {
		if _, err := db.Exec(invalid); err == nil { t.Fatalf("upgrade accepted invalid data: %s", invalid) }
	}
	for _, table := range []string{"gatehouse_agent_tasks__session_event_reply", "gatehouse_agent_tasks__session_name", "gatehouse_session_approval_decisions"} {
		if rows, err := db.Query(`SELECT * FROM `+table+` LIMIT 0`); err == nil {
			rows.Close()
			t.Fatalf("group secrets restored the retired outbox %s", table)
		}
	}
}
