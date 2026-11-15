package migrations

import (
	"context"
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
