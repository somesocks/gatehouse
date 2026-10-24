package migrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/typed_id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresMigrationV060BackfillsValidGrantIDsAfterFailedUpgrade(t *testing.T) {
	dsn := os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires a disposable PostgreSQL server with CREATE DATABASE permission")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	connection, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	control := stdlib.OpenDB(*connection)
	defer control.Close()
	id, err := typed_id.New(typed_id.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	name := "migration60_" + strings.TrimPrefix(id, "wsp_")
	if _, err := control.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := control.ExecContext(context.Background(), `DROP DATABASE "`+name+`" WITH (FORCE)`); err != nil {
			t.Errorf("clean up test database: %v", err)
		}
	}()
	connection = connection.Copy()
	connection.Database = name
	db := stdlib.OpenDB(*connection)
	defer db.Close()
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry = testSchemaAtVersion(registry, 60)
	old := testSchemaAtVersion(registry, 59)
	if err := migratePostgres(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled) VALUES ('amd_00000000000000000000000000', 1, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{}', TRUE);
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-01-01');
		INSERT INTO gatehouse_session_grants (id, workspace, session, principal, role, enabled, revision) VALUES ('sgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', 'manager', FALSE, 7);
		INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_session_grant, created_at) VALUES ('act_00000000000000000000000000', 'session_grant.create', 'session_grant', 'sgr_00000000000000000000000000', '2026-01-01');
	`); err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		if _, err := db.ExecContext(ctx, `INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, enabled, "default") VALUES ($1, $2, $3, $4, 1, TRUE, FALSE)`, "wsp_00000000000000000000000000", fmt.Sprintf("wag_%026d", index), fmt.Sprintf("bot-%d", index), "amd_00000000000000000000000000"); err != nil {
			t.Fatal(err)
		}
	}
	for index, payload := range []string{
		`{"agents":["wag_00000000000000000000000000","wag_00000000000000000000000000"]}`,
		`{"targets":[{"ref":{"kind":"agent","id":"wag_00000000000000000000000001"}}]}`,
		`{"agent":"wag_00000000000000000000000002"}`,
	} {
		kind := "message.text"
		if index == 2 {
			kind = "agent.request"
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at) VALUES ($1, $2, $3, $4, $5, $6, '2026-01-01')`, "wsp_00000000000000000000000000", "ses_00000000000000000000000000", fmt.Sprintf("sev_%026d", index), kind, "prn_00000000000000000000000000", payload); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_agent, payload, created_at) VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'sev_00000000000000000000000003', 'agent.success', 'wag_00000000000000000000000003', '{}', '2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	// Reproduce the reported constraint failure and verify that a retry can apply
	// the corrected migration to that same populated database.
	err, source := postgresMigrationV060ChatBotParticipants().Builder(ctx, &MigrationSession{queryer: db})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, bad := tx.ExecContext(ctx, strings.Replace(source, "FOR position IN 1..10 LOOP", "FOR position IN REVERSE 0..9 LOOP", 1))
	tx.Rollback()
	var constraint *pgconn.PgError
	if !errors.As(bad, &constraint) || constraint.Code != "23514" || constraint.ConstraintName != "gatehouse_session_grants_id_check" {
		t.Fatalf("expected reported ID constraint failure, got %v", bad)
	}
	for range 2 {
		if err := migratePostgres(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		rows, err := db.QueryContext(ctx, `SELECT id, role, enabled, revision FROM gatehouse_session_grants WHERE agent IS NOT NULL`)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for rows.Next() {
			var id, role string
			var enabled bool
			var revision int
			if err := rows.Scan(&id, &role, &enabled, &revision); err != nil {
				t.Fatal(err)
			}
			if !typed_id.Valid(typed_id.SessionGrant, id) || role != "contributor" || !enabled || revision != 1 {
				t.Fatalf("invalid backfill grant: %s %s %t %d", id, role, enabled, revision)
			}
			count++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if count != 4 {
			t.Fatalf("backfilled %d bots, expected 4", count)
		}
		var enabled bool
		var revision int
		if err := db.QueryRowContext(ctx, `SELECT revision, enabled FROM gatehouse_session_grants WHERE id = 'sgr_00000000000000000000000000'`).Scan(&revision, &enabled); err != nil || revision != 7 || enabled {
			t.Fatalf("existing grant changed: %d %t %v", revision, enabled, err)
		}
	}
}
