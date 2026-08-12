package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

const postgresMigrationLockID int64 = 6_741_942_832_512_869_207

func migratePostgres(ctx context.Context, database *sql.DB, registry Registry) error {
	if err := validateRegistry(registry); err != nil {
		return err
	}
	initialized, err := initializePostgres(ctx, database, registry.Init)
	if err != nil {
		return err
	}
	initializationPending := initialized
	defer func() {
		if initializationPending {
			_ = dropPostgresMigrationHistory(context.Background(), database)
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
			err := compactPostgresMigrations(ctx, database)
			if err == nil {
				initializationPending = false
			}
			return err
		}

		connection, err := database.Conn(ctx)
		if err != nil {
			return fmt.Errorf("open migration connection: %w", err)
		}
		err, applied := migratePostgresOne(ctx, connection, registry, migration)
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

func compactPostgresMigrations(ctx context.Context, database *sql.DB) error {
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN"); err != nil {
		return fmt.Errorf("begin PostgreSQL migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := connection.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", postgresMigrationLockID); err != nil {
		return fmt.Errorf("acquire PostgreSQL migration lock: %w", err)
	}
	if _, err := connection.ExecContext(ctx, compactRepeatableHistorySQL); err != nil {
		return fmt.Errorf("compact repeatable migration history: %w", err)
	}
	if err := commitPostgresMigration(ctx, connection, "migration check"); err != nil {
		return err
	}
	committed = true
	return nil
}

func initializePostgres(ctx context.Context, database *sql.DB, init InitMigration) (bool, error) {
	connection, err := database.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("open initialization connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN"); err != nil {
		return false, fmt.Errorf("begin PostgreSQL initialization transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := connection.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", postgresMigrationLockID); err != nil {
		return false, fmt.Errorf("acquire PostgreSQL initialization lock: %w", err)
	}
	var existing bool
	if err := connection.QueryRowContext(ctx, `SELECT to_regclass('gatehouse_schema_migrations') IS NOT NULL`).Scan(&existing); err != nil {
		return false, fmt.Errorf("check migration history: %w", err)
	}
	err, source := buildMigration(ctx, &MigrationSession{queryer: connection}, "init", 0, "initialize", init.Builder)
	if err != nil {
		return false, err
	}
	if _, err := connection.ExecContext(ctx, source); err != nil {
		return false, fmt.Errorf("initialize migration history: %w", err)
	}
	if err := commitPostgresMigration(ctx, connection, "initialization"); err != nil {
		return false, err
	}
	committed = true
	return !existing, nil
}

func dropPostgresMigrationHistory(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx, `DROP TABLE IF EXISTS gatehouse_schema_migrations`)
	return err
}

func migratePostgresOne(ctx context.Context, connection *sql.Conn, registry Registry, migration resolvedMigration) (error, bool) {
	if _, err := connection.ExecContext(ctx, "BEGIN"); err != nil {
		return fmt.Errorf("begin migration transaction: %w", err), false
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := connection.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", postgresMigrationLockID); err != nil {
		return fmt.Errorf("acquire PostgreSQL migration lock: %w", err), false
	}
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
		if err := commitPostgresMigration(ctx, connection, "migration check"); err != nil {
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
		) VALUES ($1, $2, $3, $4, $5)
	`, migration.migrationType, migration.index, migration.description, checksum[:], appliedAt); err != nil {
		return fmt.Errorf("record %s migration %d (%s): %w", migration.migrationType, migration.index, migration.description, err), false
	}
	if err := commitPostgresMigration(ctx, connection, fmt.Sprintf("%s migration %d (%s)", migration.migrationType, migration.index, migration.description)); err != nil {
		return err, false
	}
	committed = true
	return nil, true
}

func commitPostgresMigration(ctx context.Context, connection *sql.Conn, operation string) error {
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit %s: %w", operation, err)
	}
	return nil
}
