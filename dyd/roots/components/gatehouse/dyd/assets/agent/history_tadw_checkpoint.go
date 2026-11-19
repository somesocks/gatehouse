package agent

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"gatehouse/database"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const tadwHistoryCheckpointName = "gatehouse.tadw-initial-history"

// checkpointTADWHistory belongs once at request-workflow entry, outside the
// thinking/tool loop. Only its ordered IDs are durable; the returned fragments
// remain call-local working data.
//
// Fresh execution reuses the selection's rendered buffer. Replay skips selection
// altogether and rehydrates the checkpointed IDs, including an empty selection.
func checkpointTADWHistory(ctx dbos.Context, store *database.Store, anchor model.SessionEventRef, cachedBaseIDs []string, config tadwWindowConfig) (tadwInitialHistory, error) {
	return checkpointTADWHistorySelection(ctx, store, anchor, config, func(stepCtx context.Context) (tadwInitialHistory, error) {
		return loadTADWHistory(stepCtx, store, anchor, cachedBaseIDs, config)
	})
}

// selectFresh runs only inside the original named step's callback. Alternate
// hint sources must not do reads outside it, especially on checkpoint replay.
func checkpointTADWHistorySelection(ctx dbos.Context, store *database.Store, anchor model.SessionEventRef, config tadwWindowConfig, selectFresh func(context.Context) (tadwInitialHistory, error)) (tadwInitialHistory, error) {
	if err := config.validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	if anchor.Id == "" || anchor.Session.Id == "" || anchor.Session.Workspace.Id == "" {
		return tadwInitialHistory{}, fmt.Errorf("checkpoint TADW history: invalid anchor scope")
	}
	var fresh *tadwInitialHistory
	ids, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) ([]string, error) {
		selected, err := selectFresh(stepCtx)
		if err != nil {
			return nil, err
		}
		fresh = &selected
		return slices.Clone(selected.Window.EventIDs), nil
	}, dbos.WithStepName(tadwHistoryCheckpointName))
	if err != nil {
		return tadwInitialHistory{}, fmt.Errorf("checkpoint TADW history: %w", err)
	}
	if fresh != nil {
		return *fresh, nil
	}
	return rehydrateTADWHistory(ctx, store, anchor.Session, ids, config)
}

// rehydrateTADWHistory restores precisely the recorded membership and order.
// It never traverses, reverses, consults cache hints, or reruns the window policy.
// Events and file metadata share one live read snapshot, not the original
// selection snapshot. Changed data/configuration that no longer fits is an
// error, never a reason to silently trim or choose a different history.
func rehydrateTADWHistory(ctx context.Context, store *database.Store, session model.SessionRef, ids []string, config tadwWindowConfig) (tadwInitialHistory, error) {
	if err := config.validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwInitialHistory{}, err
	}
	if session.Workspace.Id == "" || session.Id == "" {
		return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW history: invalid session scope")
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW history: empty event ID")
		}
		if _, duplicate := seen[id]; duplicate {
			return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW history: duplicate event ID %q", id)
		}
		seen[id] = struct{}{}
	}
	result := tadwInitialHistory{Window: tadwHistoryWindow{
		EventIDs: slices.Clone(ids), ToolingBudgetBytes: config.ContextMaxBytes,
	}}
	if len(ids) == 0 {
		return result, nil
	}
	tx, err := store.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return tadwInitialHistory{}, fmt.Errorf("begin TADW history rehydration: %w", err)
	}
	defer tx.Rollback()
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return tadwInitialHistory{}, err
		}
		err, event := store.SessionTADWEventGetInSQLTransaction(ctx, tx, model.SessionEventRef{Session: session, Id: id})
		if err != nil {
			return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW event %q: %w", id, err)
		}
		if event == nil {
			return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW event %q: event not found", id)
		}
		if !tadwEventIncluded(*event) {
			return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW event %q: event is no longer renderable", id)
		}
		err, projected := store.SessionEventFilesHydrateInTransaction(ctx, tx, *event)
		if err != nil {
			return tadwInitialHistory{}, fmt.Errorf("rehydrate TADW event %q: %w", id, err)
		}
		fragment, _, err := renderTADWEvent(projected)
		if err != nil {
			return tadwInitialHistory{}, err
		}
		if fragment.Bytes() > config.HistoryMaxBytes-result.Window.HistoryBytes {
			return tadwInitialHistory{}, fmt.Errorf("rehydrated TADW history exceeds history_max_bytes at event %q", id)
		}
		result.History = append(result.History, fragment)
		result.Window.HistoryBytes += fragment.Bytes()
	}
	if err := ctx.Err(); err != nil {
		return tadwInitialHistory{}, err
	}
	result.Window.BaseEventID = ids[0]
	result.Window.ToolingBudgetBytes -= result.Window.HistoryBytes
	// CacheBaseMatched describes initial hint selection only. It is deliberately
	// not persisted or reconstructed during recovery.
	return result, nil
}
