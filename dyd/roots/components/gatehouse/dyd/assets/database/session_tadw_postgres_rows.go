package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	tadwPostgresHistoryCursorName = "gatehouse_tadw_history"
	tadwPostgresToolingCursorName = "gatehouse_tadw_tooling"
)

// PostgreSQL's wire connection cannot service a point read while a streaming
// result is open. A transaction-scoped portal retains the ID query on the server;
// each FETCH result is closed by Scan before event/file hydration uses the tx.
// No IDs or payloads are materialized into an application-side history cache.
type tadwPostgresRows struct {
	ctx               context.Context
	tx                *sql.Tx
	name              string
	current           *sql.Rows
	err, closeErr     error
	closed, exhausted bool
}

// name is a fixed internal identifier for the result shape. Cursor names are
// connection-local; callers close each cursor before reusing its name.
func tadwPostgresQuery(ctx context.Context, tx *sql.Tx, name, query string, args ...any) (sessionEventRows, error) {
	if _, err := tx.ExecContext(ctx, `DECLARE `+name+` NO SCROLL CURSOR WITHOUT HOLD FOR `+query, args...); err != nil {
		return nil, err
	}
	return &tadwPostgresRows{ctx: ctx, tx: tx, name: name}, nil
}

func (rows *tadwPostgresRows) Next() bool {
	if rows.closed || rows.exhausted || rows.err != nil {
		return false
	}
	if rows.current != nil {
		rows.err = rows.current.Close()
		rows.current = nil
	}
	if rows.err != nil {
		return false
	}
	current, err := rows.tx.QueryContext(rows.ctx, `FETCH FORWARD 1 FROM `+rows.name)
	if err != nil {
		rows.err = err
		return false
	}
	rows.current = current
	if current.Next() {
		return true
	}
	rows.err = errors.Join(current.Err(), current.Close())
	rows.current = nil
	rows.exhausted = true
	return false
}
func (rows *tadwPostgresRows) Scan(dest ...any) error {
	if rows.current == nil {
		return fmt.Errorf("scan TADW PostgreSQL cursor without a current row")
	}
	err := rows.current.Scan(dest...)
	rows.err = errors.Join(rows.err, err, rows.current.Close())
	rows.current = nil
	return rows.err
}
func (rows *tadwPostgresRows) Err() error { return rows.err }
func (rows *tadwPostgresRows) Close() error {
	if rows.closed {
		return rows.closeErr
	}
	rows.closed = true
	if rows.current != nil {
		rows.closeErr = rows.current.Close()
		rows.current = nil
	}
	// A canceled/aborted transaction releases its portals when the owner rolls it
	// back. Do not replace the caller's cancellation context to issue more work.
	if rows.ctx.Err() == nil {
		_, err := rows.tx.ExecContext(rows.ctx, `CLOSE `+rows.name)
		rows.closeErr = errors.Join(rows.closeErr, err)
	}
	return rows.closeErr
}
