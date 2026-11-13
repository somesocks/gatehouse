package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

// Sort IDs/metadata only, not payloads. Work is scoped to this request's owned
// subtree before the thinking-ID boundary, never historical requests/session
// history. Nested agent requests and foreign-agent subtrees are not ours.
const tadwToolingIDsSQL = `WITH RECURSIVE active(id) AS (
 SELECT :request
 UNION
 SELECT child.id FROM active CROSS JOIN gatehouse_session_events AS child
 WHERE child.workspace=:workspace AND child.session=:session AND child.parent=active.id
 AND child.id<:thinking AND child.kind<>'agent.request'
 AND (child.author_agent IS NULL OR child.author_agent=:agent)
)
 SELECT events.id FROM active CROSS JOIN gatehouse_session_events AS events
 WHERE events.workspace=:workspace AND events.session=:session AND events.id=active.id
 AND events.kind IN ('agent.request','tool.request','tool.success','tool.failure')
 ORDER BY events.created_at ASC,events.id ASC`

// TADWToolingCursor emits the active request and its tool events in prompt order.
// SQLite may collect/sort request-subtree metadata before emitting IDs; payloads
// are point-read only on Next. It is not a bounded-frontier history cursor and
// is not concurrency-safe. Close releases rows, never the caller's transaction.
type TADWToolingCursor struct {
	ctx           context.Context
	store         *Store
	tx            *sql.Tx
	rows          *sql.Rows
	root          model.SessionEvent
	current       model.SessionEvent
	err, closeErr error
	closed        bool
}

// SessionTADWToolingCursor validates the persisted request/thinking ownership and
// opens an ID cursor on the caller's live SQLite transaction. The thinking ID
// is an exclusive boundary, as in SessionAgentRoundEventsGet. This is a live
// snapshot, not durable reconstruction of visibility at thinking creation.
// Callers must Close before ending the transaction. Authorization is external.
func (store *Store) SessionTADWToolingCursor(ctx context.Context, tx *sql.Tx, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef) (error, *TADWToolingCursor) {
	if store.kind != config.DatabaseKindSQLite {
		return fmt.Errorf("TADW tooling cursor currently supports SQLite only"), nil
	}
	if tx == nil || request.Session.Id == "" || request.Session.Workspace.Id == "" || agent.Id == "" || agent.Workspace != request.Session.Workspace || thinking.Session != request.Session || !typed_id.Valid(typed_id.SessionEvent, request.Id) || !typed_id.Valid(typed_id.SessionEvent, thinking.Id) || thinking.Id <= request.Id {
		return fmt.Errorf("open TADW tooling cursor: invalid transaction, request scope or thinking boundary"), nil
	}
	if err := ctx.Err(); err != nil {
		return err, nil
	}
	err, root := store.SessionEventGetInSQLTransaction(ctx, tx, request)
	if err != nil {
		return err, nil
	}
	if root == nil || root.Kind != model.SessionEventKindAgentRequest || root.Payload["agent"] != agent.Id {
		return fmt.Errorf("open TADW tooling cursor: owning agent request is unavailable or mismatched"), nil
	}
	err, boundary := store.SessionEventGetInSQLTransaction(ctx, tx, thinking)
	if err != nil {
		return err, nil
	}
	if boundary == nil || boundary.Kind != model.SessionEventKindThinkingRequest || boundary.Parent == nil || *boundary.Parent != request || boundary.AuthorAgent == nil || *boundary.AuthorAgent != agent {
		return fmt.Errorf("open TADW tooling cursor: thinking request is unavailable or mismatched"), nil
	}
	rows, err := tx.QueryContext(ctx, tadwToolingIDsSQL,
		sql.Named("request", request.Id), sql.Named("workspace", request.Session.Workspace.Id),
		sql.Named("session", request.Session.Id), sql.Named("thinking", thinking.Id), sql.Named("agent", agent.Id))
	if err != nil {
		return fmt.Errorf("open TADW tooling cursor: %w", err), nil
	}
	return nil, &TADWToolingCursor{ctx: ctx, store: store, tx: tx, rows: rows, root: *root}
}

// Next reads one renderable tooling event. Exhaustion, failure and cancellation
// close the cursor automatically. Early consumers must Close explicitly.
func (cursor *TADWToolingCursor) Next() bool {
	if cursor.closed {
		return false
	}
	if err := cursor.ctx.Err(); err != nil {
		cursor.err = err
	} else if cursor.rows.Next() {
		var id string
		if err := cursor.rows.Scan(&id); err != nil {
			cursor.err = err
		} else if id == cursor.root.Ref.Id {
			cursor.current = cursor.root // Reuse the validated root's payload.
			return true
		} else {
			err, event := cursor.store.SessionEventGetInSQLTransaction(cursor.ctx, cursor.tx, model.SessionEventRef{Session: cursor.root.Ref.Session, Id: id})
			if err != nil {
				cursor.err = err
			} else if event == nil {
				cursor.err = fmt.Errorf("selected TADW tooling event %q is unavailable", id)
			} else {
				cursor.current = *event
				return true
			}
		}
	} else {
		cursor.err = cursor.rows.Err()
	}
	cursor.Close()
	return false
}

func (cursor *TADWToolingCursor) Event() model.SessionEvent { return cursor.current }
func (cursor *TADWToolingCursor) Err() error                { return cursor.err }
func (cursor *TADWToolingCursor) Close() error {
	if !cursor.closed {
		cursor.closed = true
		cursor.closeErr = cursor.rows.Close()
		cursor.err = errors.Join(cursor.err, cursor.closeErr)
		cursor.current, cursor.root = model.SessionEvent{}, model.SessionEvent{}
	}
	return cursor.closeErr
}
