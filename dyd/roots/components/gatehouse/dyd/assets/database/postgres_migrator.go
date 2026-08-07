package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
)

const postgresMigrationLockID int64 = 6_741_942_832_512_869_207

func migratePostgres(ctx context.Context, database *sql.DB, registry Registry) error {
	if err := validateRegistry(registry); err != nil {
		return err
	}

	for {
		connection, err := database.Conn(ctx)
		if err != nil {
			return fmt.Errorf("open migration connection: %w", err)
		}
		err, applied := migratePostgresOne(ctx, connection, registry)
		closeErr := connection.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close migration connection: %w", closeErr)
		}
		if !applied {
			return nil
		}
	}
}

func migratePostgresOne(ctx context.Context, connection *sql.Conn, registry Registry) (error, bool) {
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
	if _, err := connection.ExecContext(ctx, registry.Init.SQL); err != nil {
		return fmt.Errorf("initialize migration history: %w", err), false
	}
	err, history := readMigrationHistory(ctx, connection)
	if err != nil {
		return err, false
	}
	if err := validateHistory(history, registry); err != nil {
		return err, false
	}
	err, migration, ok := nextMigration(history, registry)
	if err != nil {
		return err, false
	}
	if !ok {
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
	if _, err := connection.ExecContext(ctx, `
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum
		) VALUES ($1, $2, $3, $4)
	`, migration.migrationType, migration.index, migration.description, checksum[:]); err != nil {
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
