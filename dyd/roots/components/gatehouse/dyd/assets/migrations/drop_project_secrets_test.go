package migrations

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
)

func TestSQLiteDropProjectSecrets(t *testing.T) {
	for _, baseline := range []bool{true, false} {
		name := "versioned"
		if baseline {
			name = "baseline"
		}
		t.Run(name, func(t *testing.T) {
			db := openConfiguredMigrationTestDatabase(t)
			registry := testSQLiteBaselineRegistry(t)
			if !baseline {
				registry.Baseline = nil
			}
			testDropProjectSecrets(t, db, registry, migrateSQLite)
			rows, err := db.Query(`PRAGMA foreign_key_check`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if rows.Next() {
				t.Fatal("upgrade left a foreign key violation")
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresDropProjectSecrets(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_DROP_PROJECT_SECRETS_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_DROP_PROJECT_SECRETS_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	testDropProjectSecrets(t, store.DB, registry, migratePostgres)
}

func testDropProjectSecrets(t *testing.T, db *sql.DB, registry Registry, migrate func(context.Context, *sql.DB, Registry) error) {
	t.Helper()
	ctx := context.Background()
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 65 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrate(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, enabled, revision) VALUES ('prn_00000000000000000000000000', TRUE, 1);
		INSERT INTO gatehouse_projects (workspace, id, enabled, created_at) VALUES ('wsp_00000000000000000000000000', 'prj_00000000000000000000000000', TRUE, '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_sessions (workspace, project, id, author_principal, enabled, created_at) VALUES ('wsp_00000000000000000000000000', 'prj_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_project_secrets (workspace, project, id, author_principal, description, ciphertext, enabled, created_at, updated_at) VALUES
			('wsp_00000000000000000000000000', 'prj_00000000000000000000000000', 'psc_00000000000000000000000000', 'prn_00000000000000000000000000', 'Active project credential', 'encrypted-project-value', TRUE, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
			('wsp_00000000000000000000000000', 'prj_00000000000000000000000000', 'psc_00000000000000000000000001', 'prn_00000000000000000000000000', 'Removed project credential', '', FALSE, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_session_secrets (workspace, session, id, author_principal, description, ciphertext, enabled, created_at, updated_at) VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'ssc_00000000000000000000000000', 'prn_00000000000000000000000000', 'Session credential', 'encrypted-session-value', TRUE, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_project_secret, created_at) VALUES
			('act_00000000000000000000000000', 'project_secret.create', 'project_secret', 'psc_00000000000000000000000000', '2026-01-01T00:00:00Z'),
			('act_00000000000000000000000001', 'project_secret.remove', 'project_secret', 'psc_00000000000000000000000001', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_session_secret, created_at) VALUES ('act_00000000000000000000000002', 'session_secret.create', 'session_secret', 'ssc_00000000000000000000000000', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_project, created_at) VALUES ('act_00000000000000000000000003', 'project.create', 'project', 'prj_00000000000000000000000000', '2026-01-01T00:00:00Z');
		INSERT INTO gatehouse_activity_event_topics (activity, topic) VALUES
			('act_00000000000000000000000000', 'project-credential'),
			('act_00000000000000000000000001', 'removed-project-credential'),
			('act_00000000000000000000000002', 'session-credential'),
			('act_00000000000000000000000003', 'project');
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrate(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gatehouse_project_secrets`).Scan(&count); err == nil {
		t.Fatal("project secret table still exists")
	}
	if rows, err := db.Query(`SELECT resource_project_secret FROM gatehouse_activity_events LIMIT 0`); err == nil {
		rows.Close()
		t.Fatal("project secret activity column still exists")
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM gatehouse_activity_events`,
		`SELECT COUNT(*) FROM gatehouse_activity_event_topics`,
	} {
		if err := db.QueryRow(query).Scan(&count); err != nil || count != 2 {
			t.Fatalf("%s = %d, %v; want 2", query, count, err)
		}
	}
	var ciphertext, description, topic string
	if err := db.QueryRow(`SELECT ciphertext, description FROM gatehouse_session_secrets`).Scan(&ciphertext, &description); err != nil || ciphertext != "encrypted-session-value" || description != "Session credential" {
		t.Fatalf("session secret changed: %q %q %v", ciphertext, description, err)
	}
	if err := db.QueryRow(`SELECT topic FROM gatehouse_activity_event_topics JOIN gatehouse_activity_events ON activity = id WHERE resource_kind = 'session_secret'`).Scan(&topic); err != nil || topic != "session-credential" {
		t.Fatalf("session secret history changed: %q %v", topic, err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_session_secrets SET ciphertext = 'rotated-session-value'`); err != nil {
		t.Fatalf("session secrets no longer writable: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_session_secret, created_at) VALUES ('act_00000000000000000000000004', 'session_secret.update', 'session_secret', 'ssc_00000000000000000000000000', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("session secret activity no longer writable: %v", err)
	}
	for _, query := range []string{
		`UPDATE gatehouse_activity_events SET resource_kind = 'project_secret' WHERE resource_kind = 'session_secret'`,
		`UPDATE gatehouse_activity_events SET resource_project = 'prj_00000000000000000000000000' WHERE resource_kind = 'session_secret'`,
		`UPDATE gatehouse_activity_events SET resource_session_secret = NULL WHERE resource_kind = 'session_secret'`,
	} {
		if _, err := db.Exec(query); err == nil {
			t.Fatalf("activity accepted invalid targets: %s", query)
		}
	}
}
