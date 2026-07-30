package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gatehouse/config"
	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, configuration config.DatabaseConfig) (*sql.DB, error) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite:
		if err := prepareSQLitePath(configuration.Path); err != nil {
			return nil, err
		}
		return openSQLite(ctx, configuration.Path)
	case config.DatabaseKindEphemeral:
		return openSQLite(ctx, ":memory:")
	case config.DatabaseKindPostgres:
		return nil, fmt.Errorf("PostgreSQL databases are not supported yet")
	default:
		return nil, fmt.Errorf("unsupported database kind %q", configuration.Kind)
	}
}

func prepareSQLitePath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create SQLite database directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create SQLite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close SQLite database file: %w", err)
	}
	return nil
}

func openSQLite(ctx context.Context, source string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", source)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}
	if err := migrate(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func migrate(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin database migration: %w", err)
	}
	defer transaction.Rollback()

	if _, err := transaction.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS gatehouse_schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_schema_migrations (version)
		VALUES (1)
		ON CONFLICT (version) DO NOTHING
	`); err != nil {
		return fmt.Errorf("record baseline migration: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit database migration: %w", err)
	}
	return nil
}
