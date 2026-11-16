package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestPostgresMigrationV070TADWHistoryIndexes(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL server with CREATE DATABASE permission")
	}
	ctx := context.Background()
	db := newPostgresBaselineTestDatabase(t)
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	if err := migratePostgres(ctx, db, testSchemaAtVersion(registry, 69)); err != nil {
		t.Fatal(err)
	}
	postgresTADWMigrationFixture(t, db)
	for range 2 {
		if err := migratePostgres(ctx, db, testSchemaAtVersion(registry, 70)); err != nil {
			t.Fatal(err)
		}
	}
	var text string
	if err := db.QueryRow(`SELECT payload->>'text' FROM gatehouse_session_events`).Scan(&text); err != nil || text != "preserved" {
		t.Fatal("migration changed the existing event", text, err)
	}
	for _, test := range []struct {
		name  string
		parts []string
	}{
		{"gatehouse_session_events_thread_order", []string{"(workspace, session, parent, created_at DESC, id DESC)"}},
		{"gatehouse_session_events_agent_terminals", []string{"(workspace, session, parent, created_at, id) INCLUDE (kind) WHERE", "'agent.success'", "'agent.failure'"}},
	} {
		var definition string
		if err := db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE schemaname='public' AND indexname=$1`, test.name).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		for _, part := range test.parts {
			if !strings.Contains(definition, part) {
				t.Fatal("incorrect history index", test.name, definition)
			}
		}
	}
}

func postgresTADWMigrationFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`
 INSERT INTO gatehouse_workspaces(id,enabled) VALUES('wsp_00000000000000000000000000',TRUE);
 INSERT INTO gatehouse_principals(id,revision,enabled) VALUES('prn_00000000000000000000000000',1,TRUE);
 INSERT INTO gatehouse_sessions(workspace,id,author_principal,enabled,created_at)
 VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000','prn_00000000000000000000000000',TRUE,'2026-11-01T00:00:00Z');
 INSERT INTO gatehouse_session_events(workspace,session,id,kind,author_principal,payload,created_at)
 VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000','sev_00000000000000000000000001','message.text','prn_00000000000000000000000000','{"text":"preserved"}','2026-11-01T00:00:00Z');
 `); err != nil {
		t.Fatal(err)
	}
}
