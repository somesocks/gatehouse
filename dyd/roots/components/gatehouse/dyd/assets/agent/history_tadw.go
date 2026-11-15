package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"gatehouse/database"
	"gatehouse/model"
)

// tadwInitialHistory contains rendered history and byte accounting for one
// selection or assembly call. checkpointTADWHistory persists Window.EventIDs.
type tadwInitialHistory struct {
	Window  tadwHistoryWindow
	History []tadwRenderedEvent
}

// loadTADWHistory connects the history cursor, metadata projection,
// renderer and window policy. Cache hints are supplied/scoped by the caller.
// It neither activates TADW nor implements durable request-creation visibility.
func loadTADWHistory(ctx context.Context, store *database.Store, anchor model.SessionEventRef, cachedBaseIDs []string, config tadwWindowConfig) (tadwInitialHistory, error) {
	if err := config.validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	tx, err := store.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return tadwInitialHistory{}, fmt.Errorf("begin TADW history: %w", err)
	}
	defer tx.Rollback()
	err, cursor := store.SessionTADWHistoryCursor(ctx, tx, anchor)
	if err != nil {
		return tadwInitialHistory{}, err
	}
	defer cursor.Close()
	hydrate := func(event model.SessionEvent) (model.SessionEvent, error) {
		err, projected := store.SessionEventFilesHydrateInTransaction(ctx, tx, event)
		return projected, err
	}
	result, err := collectTADWHistory(ctx, cursor, hydrate, cachedBaseIDs, config)
	if err = errors.Join(err, cursor.Close()); err != nil {
		return tadwInitialHistory{}, fmt.Errorf("load TADW history: %w", err)
	}
	return result, nil
}

type tadwHistorySource interface {
	Next() bool
	Event() database.TADWHistoryEvent
	Err() error
}

// collectTADWHistory accepts reverse-prompt-order events, stops at the first
// crossing renderable event, and reverses the entire accepted list once.
func collectTADWHistory(ctx context.Context, source tadwHistorySource, hydrate func(model.SessionEvent) (model.SessionEvent, error), cachedBaseIDs []string, config tadwWindowConfig) (tadwInitialHistory, error) {
	if err := config.validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	var candidate []tadwRenderedEvent
	var used int64
	for used < config.HistoryMaxBytes {
		if err := ctx.Err(); err != nil {
			return tadwInitialHistory{}, err
		}
		if !source.Next() {
			break
		}
		event := source.Event().Event
		// Share the renderer's allowlist. Omitted kinds must not trigger file
		// lookups, validation of irrelevant payloads, or durable selected IDs.
		if !tadwEventIncluded(event) {
			continue
		}
		projected, err := hydrate(event)
		if err != nil {
			return tadwInitialHistory{}, err
		}
		rendered, _, err := renderTADWEvent(projected)
		if err != nil {
			return tadwInitialHistory{}, err
		}
		if rendered.Bytes() > config.HistoryMaxBytes-used {
			break
		}
		candidate = append(candidate, rendered)
		used += rendered.Bytes()
	}
	if err := source.Err(); err != nil {
		return tadwInitialHistory{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwInitialHistory{}, err
	}
	slices.Reverse(candidate)
	window, err := selectTADWHistoryWindow(candidate, cachedBaseIDs, config)
	if err != nil {
		return tadwInitialHistory{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwInitialHistory{}, err
	}
	// Release discarded candidate fragments, rather than retaining them through
	// the backing array of a subslice. The selected XML itself is reused.
	history := slices.Clone(candidate[len(candidate)-len(window.EventIDs):])
	return tadwInitialHistory{Window: window, History: history}, nil
}
