package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

const sqliteBusyTimeout = 30_000

func migrateSQLite(ctx context.Context, database *sql.DB, registry Registry) error {
	if err := validateRegistry(registry); err != nil {
		return err
	}
	initialized, err := initializeSQLite(ctx, database, registry.Init)
	if err != nil {
		return err
	}
	initializationPending := initialized
	defer func() {
		if initializationPending {
			_ = dropSQLiteMigrationHistory(context.Background(), database)
		}
	}()
	cursor := migrationCursor{init: "initialized"}
	for {
		err, history := readMigrationHistory(ctx, database)
		if err != nil {
			return err
		}
		if err := validateHistory(history, registry); err != nil {
			return err
		}
		err, migration, next, ok := nextMigration(ctx, &MigrationSession{queryer: database}, history, registry, cursor)
		if err != nil {
			return err
		}
		if !ok {
			err := compactSQLiteMigrations(ctx, database)
			if err == nil {
				initializationPending = false
			}
			return err
		}

		connection, err := database.Conn(ctx)
		if err != nil {
			return fmt.Errorf("open migration connection: %w", err)
		}
		err, applied := migrateSQLiteOne(ctx, connection, registry, migration)
		closeErr := connection.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close migration connection: %w", closeErr)
		}
		cursor = next
		if applied {
			initializationPending = false
		}
	}
}

func compactSQLiteMigrations(ctx context.Context, database *sql.DB) error {
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		return fmt.Errorf("set SQLite migration busy timeout: %w", err)
	}
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin SQLite migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := connection.ExecContext(ctx, compactRepeatableHistorySQL); err != nil {
		return fmt.Errorf("compact repeatable migration history: %w", err)
	}
	if err := commitSQLiteMigration(ctx, connection, "migration check"); err != nil {
		return err
	}
	committed = true
	return nil
}

func initializeSQLite(ctx context.Context, database *sql.DB, init InitMigration) (bool, error) {
	connection, err := database.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("open initialization connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		return false, fmt.Errorf("set SQLite initialization busy timeout: %w", err)
	}
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, fmt.Errorf("begin SQLite initialization transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	var existing int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'gatehouse_schema_migrations'`).Scan(&existing); err != nil {
		return false, fmt.Errorf("check migration history: %w", err)
	}
	err, source := buildMigration(ctx, &MigrationSession{queryer: connection}, "init", 0, "initialize", init.Builder)
	if err != nil {
		return false, err
	}
	if _, err := connection.ExecContext(ctx, source); err != nil {
		return false, fmt.Errorf("initialize migration history: %w", err)
	}
	if err := commitSQLiteMigration(ctx, connection, "initialization"); err != nil {
		return false, err
	}
	committed = true
	return existing == 0, nil
}

func dropSQLiteMigrationHistory(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx, `DROP TABLE IF EXISTS gatehouse_schema_migrations`)
	return err
}

func migrateSQLiteOne(ctx context.Context, connection *sql.Conn, registry Registry, migration resolvedMigration) (error, bool) {
	if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		return fmt.Errorf("set SQLite migration busy timeout: %w", err), false
	}
	if migration.options.SQLite.DisableForeignKeys {
		if _, err := connection.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
			return fmt.Errorf("disable SQLite foreign keys for migration: %w", err), false
		}
		defer connection.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
	}
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate migration transaction: %w", err), false
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	err, history := readMigrationHistory(ctx, connection)
	if err != nil {
		return err, false
	}
	if err := validateHistory(history, registry); err != nil {
		return err, false
	}
	err, required := migrationRequired(history, migration)
	if err != nil {
		return err, false
	}
	if !required {
		if err := commitSQLiteMigration(ctx, connection, "migration check"); err != nil {
			return err, false
		}
		committed = true
		return nil, false
	}

	if _, err := connection.ExecContext(ctx, migration.source); err != nil {
		return fmt.Errorf("execute %s migration %d (%s): %w", migration.migrationType, migration.index, migration.description, err), false
	}
	checksum := sha256.Sum256([]byte(migration.source))
	appliedAt := time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	if _, err := connection.ExecContext(ctx, `
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum, applied_at
		) VALUES (?, ?, ?, ?, ?)
	`, migration.migrationType, migration.index, migration.description, checksum[:], appliedAt); err != nil {
		return fmt.Errorf("record %s migration %d (%s): %w", migration.migrationType, migration.index, migration.description, err), false
	}
	if err := commitSQLiteMigration(ctx, connection, fmt.Sprintf("%s migration %d (%s)", migration.migrationType, migration.index, migration.description)); err != nil {
		return err, false
	}
	committed = true
	return nil, true
}

func commitSQLiteMigration(ctx context.Context, connection *sql.Conn, operation string) error {
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit %s: %w", operation, err)
	}
	return nil
}

type migrationHistoryReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readMigrationHistory(ctx context.Context, database migrationHistoryReader) (error, []appliedMigration) {
	rows, err := database.QueryContext(ctx, `
		SELECT installed_rank, migration_type, migration_index, description, checksum
		FROM gatehouse_schema_migrations
		ORDER BY installed_rank
	`)
	if err != nil {
		return fmt.Errorf("read migration history: %w", err), nil
	}
	defer rows.Close()

	var history []appliedMigration
	for rows.Next() {
		var migration appliedMigration
		var checksum []byte
		if err := rows.Scan(
			&migration.installedRank,
			&migration.migrationType,
			&migration.index,
			&migration.description,
			&checksum,
		); err != nil {
			return fmt.Errorf("scan migration history: %w", err), nil
		}
		if len(checksum) != sha256.Size {
			return fmt.Errorf("migration history rank %d has an invalid checksum", migration.installedRank), nil
		}
		copy(migration.checksum[:], checksum)
		history = append(history, migration)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate migration history: %w", err), nil
	}
	return nil, history
}
