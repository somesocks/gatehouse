package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
)

const sqliteBusyTimeout = 30_000

func migrateSQLite(ctx context.Context, database *sql.DB, registry Registry) error {
	if err := validateRegistry(registry); err != nil {
		return err
	}

	cursor := migrationCursor{}
	for {
		connection, err := database.Conn(ctx)
		if err != nil {
			return fmt.Errorf("open migration connection: %w", err)
		}
		err, next, applied := migrateSQLiteOne(ctx, connection, registry, cursor)
		closeErr := connection.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close migration connection: %w", closeErr)
		}
		cursor = next
		if !applied {
			return nil
		}
	}
}

func migrateSQLiteOne(ctx context.Context, connection *sql.Conn, registry Registry, cursor migrationCursor) (error, migrationCursor, bool) {
	if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		return fmt.Errorf("set SQLite migration busy timeout: %w", err), cursor, false
	}
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate migration transaction: %w", err), cursor, false
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	session := &MigrationSession{connection: connection}
	init := cursor.init
	if init == "" {
		err, built := buildMigration(ctx, session, "init", 0, "initialize", registry.Init.Builder)
		if err != nil {
			return err, cursor, false
		}
		init = built
		cursor.init = init
	}
	if _, err := connection.ExecContext(ctx, init); err != nil {
		return fmt.Errorf("initialize migration history: %w", err), cursor, false
	}
	err, history := readMigrationHistory(ctx, connection)
	if err != nil {
		return err, cursor, false
	}
	if err := validateHistory(history, registry); err != nil {
		return err, cursor, false
	}
	err, migration, next, ok := nextMigration(ctx, session, history, registry, cursor)
	if err != nil {
		return err, cursor, false
	}
	if !ok {
		if _, err := connection.ExecContext(ctx, compactRepeatableHistorySQL); err != nil {
			return fmt.Errorf("compact repeatable migration history: %w", err), cursor, false
		}
		if err := commitSQLiteMigration(ctx, connection, "migration check"); err != nil {
			return err, cursor, false
		}
		committed = true
		return nil, next, false
	}

	if _, err := connection.ExecContext(ctx, migration.source); err != nil {
		return fmt.Errorf("execute %s migration %d (%s): %w", migration.migrationType, migration.index, migration.description, err), cursor, false
	}
	checksum := sha256.Sum256([]byte(migration.source))
	if _, err := connection.ExecContext(ctx, `
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum
		) VALUES (?, ?, ?, ?)
	`, migration.migrationType, migration.index, migration.description, checksum[:]); err != nil {
		return fmt.Errorf("record %s migration %d (%s): %w", migration.migrationType, migration.index, migration.description, err), cursor, false
	}
	if err := commitSQLiteMigration(ctx, connection, fmt.Sprintf("%s migration %d (%s)", migration.migrationType, migration.index, migration.description)); err != nil {
		return err, cursor, false
	}
	committed = true
	return nil, next, true
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
