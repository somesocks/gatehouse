package benchmark

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed results.sql
var resultsSchema string

// OpenResults explicitly creates an isolated in-memory capture database. The
// caller owns its lifetime. Schema setup is scoped to this connection and is
// independent of Gatehouse's operational database and migration registry.
func OpenResults(ctx context.Context) (error, *sql.DB) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return fmt.Errorf("open benchmark results: %w", err), nil
	}
	// :memory: belongs to a connection, so retain exactly one connection for
	// the database's lifetime rather than allowing the pool to open another.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return fmt.Errorf("enable benchmark foreign keys: %w", err), nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("begin benchmark schema initialization: %w", err), nil
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, resultsSchema); err != nil {
		_ = tx.Rollback()
		_ = db.Close()
		return fmt.Errorf("initialize benchmark schema: %w", err), nil
	}
	if err := tx.Commit(); err != nil {
		_ = db.Close()
		return fmt.Errorf("commit benchmark schema: %w", err), nil
	}
	return nil, db
}
