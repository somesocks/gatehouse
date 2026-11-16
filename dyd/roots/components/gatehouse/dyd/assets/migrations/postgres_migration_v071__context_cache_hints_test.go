package migrations

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"gatehouse/config"
)

// Schema generation/registration checks run without a PostgreSQL server. They
// do not assert live PostgreSQL DDL execution or planner/locking behavior.
func TestPostgresTADWMigrationsRegistered(t *testing.T) {
	err, registry := postgresMigrations(config.State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tail []int64
	for _, migration := range registry.Versioned {
		if migration.Index >= 69 {
			tail = append(tail, migration.Index)
		}
	}
	if !slices.Equal(tail, []int64{69, 70, 71}) {
		t.Fatal("missing PostgreSQL TADW migrations", tail)
	}
	var indexes, hints string
	for _, migration := range registry.Versioned {
		if migration.Index != 70 && migration.Index != 71 {
			continue
		}
		err, source := migration.Builder(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if migration.Index == 70 {
			indexes = source
		} else {
			hints = source
		}
	}
	if strings.Count(indexes, "CREATE INDEX") != 2 {
		t.Fatal("history needs exactly two indexes", indexes)
	}
	for _, clause := range []string{"gatehouse_session_events_thread_order", "(workspace,session,parent,created_at DESC,id DESC)", "gatehouse_session_events_agent_terminals", "(workspace,session,parent,created_at,id) INCLUDE (kind)", "WHERE kind IN ('agent.success','agent.failure')"} {
		if !strings.Contains(indexes, clause) {
			t.Fatal("missing history index contract", clause)
		}
	}
	for _, clause := range []string{"CREATE TABLE gatehouse_context_cache_hints", "octet_length(cache_scope)<=1024", "octet_length(algorithm)<=1024", "octet_length(profile)<=1024", "octet_length(hint_key)<=1024", "octet_length(hint)<=4096", "json_typeof(hint::json)='object'", "last_used_at BIGINT NOT NULL", "PRIMARY KEY(workspace,session,cache_scope,algorithm,profile,hint_key)", "REFERENCES gatehouse_sessions(workspace,id) ON DELETE CASCADE", "(workspace,session,cache_scope,algorithm,profile,last_used_at DESC,hint_key DESC)"} {
		if !strings.Contains(hints, clause) {
			t.Fatal("missing hint schema contract", clause)
		}
	}
	for _, sqlite := range []string{" STRICT", "json_valid(", " AS BLOB"} {
		if strings.Contains(hints, sqlite) {
			t.Fatal("SQLite-only schema in PostgreSQL migration", sqlite)
		}
	}
}

func TestPostgresMigrationV071ContextCacheHints(t *testing.T) {
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
	if err := migratePostgres(ctx, db, testSchemaAtVersion(registry, 70)); err != nil {
		t.Fatal(err)
	}
	postgresTADWMigrationFixture(t, db)
	if err := migratePostgres(ctx, db, registry); err != nil {
		t.Fatal(err)
	}
	var definition string
	if err := db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE schemaname='public' AND indexname='gatehouse_context_cache_hints_recent'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "(workspace, session, cache_scope, algorithm, profile, last_used_at DESC, hint_key DESC)") {
		t.Fatal("incorrect hint recency index", definition)
	}
	const insertHint = `INSERT INTO gatehouse_context_cache_hints(workspace,session,cache_scope,algorithm,profile,hint_key,hint,last_used_at)
	 VALUES('wsp_00000000000000000000000000','ses_00000000000000000000000000','scope','tadw','profile','base',$1,$2)`
	const hint = `{"version":1,"base_event_id":"stale-is-allowed"}`
	const usedAt int64 = 9007199254740993
	if _, err := db.Exec(insertHint, hint, usedAt); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migratePostgres(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	var text string
	if err := db.QueryRow(`SELECT payload->>'text' FROM gatehouse_session_events`).Scan(&text); err != nil || text != "preserved" {
		t.Fatal("migration changed the existing event", text, err)
	}
	var payload string
	var at int64
	if err := db.QueryRow(`SELECT hint,last_used_at FROM gatehouse_context_cache_hints`).Scan(&payload, &at); err != nil || payload != hint || at != usedAt {
		t.Fatal("hint payload or native bigint changed", payload, at, err)
	}
	if _, err := db.Exec(insertHint, hint, usedAt); err == nil {
		t.Fatal("duplicate hint key accepted")
	}
	// Exercise representative constraints directly, bypassing API validation.
	for _, bad := range []string{`[]`, `invalid JSON`, `{"x":"` + strings.Repeat("é", 2044) + `x"}`} {
		if _, err := db.Exec(`UPDATE gatehouse_context_cache_hints SET hint=$1`, bad); err == nil {
			t.Fatal("invalid hint accepted", bad)
		}
	}
	for _, bad := range []string{" ", strings.Repeat("é", 513)} {
		if _, err := db.Exec(`UPDATE gatehouse_context_cache_hints SET profile=$1`, bad); err == nil {
			t.Fatal("invalid profile accepted")
		}
	}
	validHint := `{"x":"` + strings.Repeat("é", 2044) + `"}`
	if _, err := db.Exec(`UPDATE gatehouse_context_cache_hints SET profile=$1,hint=$2`, strings.Repeat("é", 512), validHint); err != nil {
		t.Fatal("valid UTF-8 byte boundaries rejected", err)
	}
	if _, err := db.Exec(`UPDATE gatehouse_context_cache_hints SET session='ses_00000000000000000000000001'`); err == nil {
		t.Fatal("foreign session accepted")
	}
	if _, err := db.Exec(`DELETE FROM gatehouse_session_events`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM gatehouse_context_cache_hints`).Scan(&count); err != nil || count != 1 {
		t.Fatal("event deletion must not remove or invalidate stale-base hints", count, err)
	}
	if _, err := db.Exec(`DELETE FROM gatehouse_sessions`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM gatehouse_context_cache_hints`).Scan(&count); err != nil || count != 0 {
		t.Fatal("session deletion did not cascade to hints", count, err)
	}
}
