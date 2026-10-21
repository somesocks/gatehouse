package migrations

import (
	"context"
	"testing"
)

func TestSQLiteWorkflowOutboxesUpgrade(t *testing.T) {
	ctx := context.Background()
	db := openConfiguredMigrationTestDatabase(t)
	registry := testSQLiteBaselineRegistry(t)
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 67 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrateSQLite(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateSQLite(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'
		AND name IN ('gatehouse_agent_tasks__session_event_reply', 'gatehouse_agent_tasks__session_name', 'gatehouse_session_approval_decisions')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy outbox tables survived upgrade: %d (%v)", count, err)
	}
}
