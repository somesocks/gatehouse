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

type Store struct {
	*sql.DB
	kind config.DatabaseKind
}

const sqliteBusyTimeout = 30_000

func Open(ctx context.Context, configuration config.DatabaseConfig) (error, *Store) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite:
		if err := prepareSQLitePath(configuration.Path); err != nil {
			return err, nil
		}
		err, database := openSQLite(ctx, configuration.Path)
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindSQLite}
	case config.DatabaseKindEphemeral:
		err, database := openSQLite(ctx, ":memory:")
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindSQLite}
	case config.DatabaseKindPostgres:
		return fmt.Errorf("PostgreSQL databases are not supported yet"), nil
	default:
		return fmt.Errorf("unsupported database kind %q", configuration.Kind), nil
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

func openSQLite(ctx context.Context, source string) (error, *sql.DB) {
	database, err := sql.Open("sqlite", source)
	if err != nil {
		return fmt.Errorf("open SQLite database: %w", err), nil
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return fmt.Errorf("ping SQLite database: %w", err), nil
	}
	if _, err := database.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		database.Close()
		return fmt.Errorf("set SQLite busy timeout: %w", err), nil
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		database.Close()
		return fmt.Errorf("enable SQLite foreign keys: %w", err), nil
	}
	return nil, database
}
