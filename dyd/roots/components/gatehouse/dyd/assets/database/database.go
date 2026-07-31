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

func Open(ctx context.Context, configuration config.DatabaseConfig, workspaces []config.Workspace) (error, *sql.DB) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite:
		if err := prepareSQLitePath(configuration.Path); err != nil {
			return err, nil
		}
		return openSQLite(ctx, configuration.Path, workspaces)
	case config.DatabaseKindEphemeral:
		return openSQLite(ctx, ":memory:", workspaces)
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

func openSQLite(ctx context.Context, source string, workspaces []config.Workspace) (error, *sql.DB) {
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
	if err := migrateSQLite(ctx, database, sqliteMigrations, migrationValuesFor(workspaces)); err != nil {
		database.Close()
		return err, nil
	}
	return nil, database
}

type migrationValues struct {
	Workspaces []workspaceMigrationValue
}

type workspaceMigrationValue struct {
	ID      string
	Name    any
	Enabled bool
}

func migrationValuesFor(workspaces []config.Workspace) migrationValues {
	values := migrationValues{Workspaces: make([]workspaceMigrationValue, 0, len(workspaces))}
	for _, workspace := range workspaces {
		var name any
		if workspace.Name != nil {
			name = *workspace.Name
		}
		values.Workspaces = append(values.Workspaces, workspaceMigrationValue{
			ID:      workspace.ID,
			Name:    name,
			Enabled: workspace.Enabled,
		})
	}
	return values
}
