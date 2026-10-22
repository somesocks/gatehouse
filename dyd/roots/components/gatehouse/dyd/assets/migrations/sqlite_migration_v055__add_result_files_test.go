package migrations

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLiteMigrationV055PreservesHistoryAndAttachments(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := testSchemaAtVersion(registry, 54)
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO gatehouse_workspaces (id, enabled) VALUES ('wsp_00000000000000000000000000', TRUE);
		INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('prn_00000000000000000000000000', 1, TRUE);
		INSERT INTO gatehouse_agent_providers (id, revision, protocol, enabled) VALUES ('apr_00000000000000000000000000', 1, 'builtin', TRUE);
		INSERT INTO gatehouse_agent_models (id, revision, provider_id, model, parameters, enabled)
		VALUES ('amd_00000000000000000000000000', 7, 'apr_00000000000000000000000000', 'dummy.fixed-reply', '{"text":"done"}', FALSE);
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at)
		VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, '2026-10-14T00:00:00Z');
		INSERT INTO gatehouse_storage_providers (id, alias, revision, protocol, enabled)
		VALUES ('stp_00000000000000000000000000', 'existing', 9, 'embedded', FALSE);
		INSERT INTO gatehouse_storage_objects (id, provider, object, state, created_at)
		VALUES ('obj_00000000000000000000000000', 'stp_00000000000000000000000000', 'original-key', 'pending', '2026-10-14T00:00:00Z');
		INSERT INTO gatehouse_session_files (workspace, session, id, storage_object, name, enabled, created_at)
		VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'sfi_00000000000000000000000000', 'obj_00000000000000000000000000', 'original.txt', FALSE, '2026-10-14T00:00:00Z');
		INSERT INTO gatehouse_session_events (workspace, session, id, kind, author_principal, payload, created_at)
		VALUES ('wsp_00000000000000000000000000', 'ses_00000000000000000000000000', 'sev_00000000000000000000000000', 'tool.success', 'prn_00000000000000000000000000', '{"output": "existing output"}', '2026-10-14T00:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		var cutoff, revision int
		var enabled bool
		if err := db.QueryRow(`SELECT result_inline_bytes, revision, enabled FROM gatehouse_agent_models`).Scan(&cutoff, &revision, &enabled); err != nil || cutoff != 4096 || revision != 7 || enabled {
			t.Fatalf("model migration: %d %d %v %v", cutoff, revision, enabled, err)
		}
		var name, object, payload string
		var principal, agent sql.NullString
		if err := db.QueryRow(`SELECT name, storage_object, enabled, author_principal, author_agent FROM gatehouse_session_files`).Scan(&name, &object, &enabled, &principal, &agent); err != nil || principal.Valid || agent.Valid || name != "original.txt" || object != "obj_00000000000000000000000000" || enabled {
			t.Fatalf("file migration changed metadata or invented authorship: %s %s %v %v", name, object, enabled, err)
		}
		if err := db.QueryRow(`SELECT payload FROM gatehouse_session_events`).Scan(&payload); err != nil || payload != `{"output": "existing output"}` {
			t.Fatal("migration rewrote event history")
		}
		if err := db.QueryRow(`SELECT revision, enabled FROM gatehouse_storage_providers WHERE alias = 'existing'`).Scan(&revision, &enabled); err != nil || revision != 9 || enabled {
			t.Fatal("migration changed configured storage provider")
		}
	}
	if _, err := db.Exec(`UPDATE gatehouse_agent_models SET result_inline_bytes = 0`); err == nil {
		t.Fatal("database accepted nonpositive cutoff")
	}
}
