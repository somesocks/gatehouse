package benchmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var resultsTables = []string{
	"executions", "providers", "models", "system_prompts", "questions", "lisp_environments", "runs", "session_events",
}

// ExportResults writes the complete capture to a SQLite file. Existing files
// require appendOutput; appends use plain INSERTs in one transaction, preserving
// captured IDs and encoded JSON. Duplicate identities fail instead of replacing
// records. The caller retains ownership of the source database.
func ExportResults(ctx context.Context, source *sql.DB, path string, appendOutput bool) error {
	if path == "" {
		return fmt.Errorf("benchmark output path must not be empty")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve benchmark output path: %w", err)
	}
	read, err := source.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin benchmark export snapshot: %w", err)
	}
	defer read.Rollback()
	var version int
	if err := read.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read benchmark capture version: %w", err)
	}
	if version != 2 {
		return fmt.Errorf("unsupported benchmark capture version %d", version)
	}
	created := false
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		created = true
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return fmt.Errorf("close new benchmark output %q: %w", path, err)
		}
	} else if !appendOutput || !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create benchmark output %q: %w", path, err)
	}
	committed := false
	defer func() {
		if created && !committed {
			_ = os.Remove(path)
		}
	}()
	// A file URI quotes path characters such as ?, #, and spaces for the driver.
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	destination, err := sql.Open("sqlite", uri)
	if err != nil {
		return fmt.Errorf("open benchmark output %q: %w", path, err)
	}
	defer destination.Close()
	destination.SetMaxOpenConns(1)
	destination.SetMaxIdleConns(1)
	if _, err := destination.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable benchmark output foreign keys: %w", err)
	}
	write, err := destination.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin benchmark output transaction: %w", err)
	}
	defer write.Rollback()
	if created {
		if _, err := write.ExecContext(ctx, resultsSchema); err != nil {
			return fmt.Errorf("initialize benchmark output schema: %w", err)
		}
	} else {
		if err := write.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return fmt.Errorf("read benchmark output version: %w", err)
		}
		if version != 2 {
			return fmt.Errorf("unsupported benchmark output version %d", version)
		}
	}
	for _, table := range resultsTables {
		if err := copyResultsTable(ctx, read, write, table); err != nil {
			return fmt.Errorf("export benchmark table %s: %w", table, err)
		}
	}
	if err := write.Commit(); err != nil {
		return fmt.Errorf("commit benchmark output %q: %w", path, err)
	}
	committed = true
	return nil
}

func copyResultsTable(ctx context.Context, source, destination *sql.Tx, table string) error {
	rows, err := source.QueryContext(ctx, "SELECT * FROM "+quoteResultsIdentifier(table))
	if err != nil {
		return fmt.Errorf("read captured records: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("read captured columns: %w", err)
	}
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for index, column := range columns {
		quoted[index], placeholders[index] = quoteResultsIdentifier(column), "?"
	}
	// Validate the destination's table/column layout even for empty captures.
	probe, err := destination.QueryContext(ctx, "SELECT "+strings.Join(quoted, ", ")+" FROM "+quoteResultsIdentifier(table)+" LIMIT 0")
	if err != nil {
		return fmt.Errorf("incompatible output columns: %w", err)
	}
	if err := probe.Close(); err != nil {
		return err
	}
	insert := "INSERT INTO " + quoteResultsIdentifier(table) + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	statement, err := destination.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("prepare captured record insert: %w", err)
	}
	defer statement.Close()
	values, pointers := make([]any, len(columns)), make([]any, len(columns))
	for index := range values {
		pointers[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return fmt.Errorf("read captured record: %w", err)
		}
		if _, err := statement.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("insert captured record: %w", err)
		}
	}
	return rows.Err()
}

func quoteResultsIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
