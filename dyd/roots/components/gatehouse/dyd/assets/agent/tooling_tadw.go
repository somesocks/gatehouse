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

// tadwToolingBudgetError reports the first crossing event, not a usable partial
// context or the size of all remaining tooling. Separate counters avoid overflow.
type tadwToolingBudgetError struct {
	BudgetBytes, UsedBytes, EventBytes int64
	EventID                            string
}

func (err *tadwToolingBudgetError) Error() string {
	return fmt.Sprintf("TADW tooling exceeds budget at event %q (budget %d, accepted %d, next event %d bytes)", err.EventID, err.BudgetBytes, err.UsedBytes, err.EventBytes)
}

// loadTADWContext assembles fixed worker-local history followed by this request's
// tooling before the current thinking request. It never reselects/re-renders
// history, checkpoints context, or silently trims tooling.
func loadTADWContext(ctx context.Context, store *database.Store, history tadwInitialHistory, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef, config tadwWindowConfig) (tadwTranscript, error) {
	return loadTADWContextFrom(ctx, store, history, request, agent, thinking, config, "")
}

// loadTADWContextFrom is strict checkpoint rehydration: it never rolls a recorded
// boundary forward if changed data/configuration no longer fits.
func loadTADWContextFrom(ctx context.Context, store *database.Store, history tadwInitialHistory, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef, config tadwWindowConfig, from string) (tadwTranscript, error) {
	budget, err := tadwFixedHistoryBudget(history, config)
	if err != nil {
		return tadwTranscript{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwTranscript{}, err
	}
	tx, err := store.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return tadwTranscript{}, fmt.Errorf("begin TADW tooling: %w", err)
	}
	defer tx.Rollback()
	transcript, _, err := loadTADWToolingInTransaction(ctx, store, tx, history, request, agent, thinking, from, budget)
	return transcript, err
}

// The root is returned only for an in-memory rollover within this same snapshot.
func loadTADWToolingInTransaction(ctx context.Context, store *database.Store, tx *sql.Tx, history tadwInitialHistory, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef, from string, budget int64) (tadwTranscript, model.SessionEvent, error) {
	err, cursor := store.SessionTADWToolingCursor(ctx, tx, request, agent, thinking, database.TADWToolingReadOptions{FromEventID: from})
	if err != nil {
		return tadwTranscript{}, model.SessionEvent{}, err
	}
	defer cursor.Close()
	root := cursor.RequestEvent()
	source := &tadwPinnedRequestSource{cursor: cursor, root: root}
	hydrate := func(event model.SessionEvent) (model.SessionEvent, error) {
		err, projected := store.SessionEventFilesHydrateInTransaction(ctx, tx, event)
		return projected, err
	}
	transcript, err := collectTADWTooling(ctx, source, hydrate, history.History, budget)
	if closeErr := cursor.Close(); closeErr != nil {
		return tadwTranscript{}, model.SessionEvent{}, fmt.Errorf("close TADW tooling: %w", errors.Join(err, closeErr))
	}
	if err != nil {
		return tadwTranscript{}, root, fmt.Errorf("load TADW tooling: %w", err)
	}
	return transcript, root, nil
}

// Pin the owning request before chronologically ordered tool events, even if
// malformed/backdated timestamps place a tool before it in the raw cursor.
type tadwPinnedRequestSource struct {
	cursor        *database.TADWToolingCursor
	root, current model.SessionEvent
	started       bool
}

func (source *tadwPinnedRequestSource) Next() bool {
	if !source.started {
		source.started = true
		source.current = source.root
		return true
	}
	for source.cursor.Next() {
		event := source.cursor.Event()
		if event.Ref.Id != source.root.Ref.Id {
			source.current = event
			return true
		}
	}
	source.current = model.SessionEvent{}
	return false
}
func (source *tadwPinnedRequestSource) Event() model.SessionEvent { return source.current }
func (source *tadwPinnedRequestSource) Err() error                { return source.cursor.Err() }

// loadTADWToolingWindow grows the prior retained window, rolling only on a byte
// overflow. Both the fit check and newest-first rollover share one read snapshot.
// It is deliberately separate from strict checkpoint rehydration.
func loadTADWToolingWindow(ctx context.Context, store *database.Store, history tadwInitialHistory, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef, config tadwWindowConfig, from string) (tadwToolingWindow, error) {
	budget, err := tadwFixedHistoryBudget(history, config)
	if err != nil {
		return tadwToolingWindow{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwToolingWindow{}, err
	}
	tx, err := store.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return tadwToolingWindow{}, fmt.Errorf("begin TADW tooling window: %w", err)
	}
	defer tx.Rollback()
	transcript, root, err := loadTADWToolingInTransaction(ctx, store, tx, history, request, agent, thinking, from, budget)
	if err == nil {
		return tadwToolingWindow{FromEventID: from, Transcript: transcript}, nil
	}
	var overflow *tadwToolingBudgetError
	if !errors.As(err, &overflow) || root.Ref.Id != request.Id || overflow.EventID == request.Id {
		return tadwToolingWindow{}, err
	}
	hydrate := func(event model.SessionEvent) (model.SessionEvent, error) {
		err, projected := store.SessionEventFilesHydrateInTransaction(ctx, tx, event)
		return projected, err
	}
	projected, err := hydrate(root)
	if err != nil {
		return tadwToolingWindow{}, err
	}
	pinned, _, err := renderTADWEvent(projected)
	if err != nil {
		return tadwToolingWindow{}, err
	}
	err, cursor := store.SessionTADWToolingCursor(ctx, tx, request, agent, thinking, database.TADWToolingReadOptions{FromEventID: from, Reverse: true})
	if err != nil {
		return tadwToolingWindow{}, err
	}
	defer cursor.Close()
	seen := map[string]bool{request.Id: true}
	for _, fragment := range history.History {
		seen[fragment.ID] = true
	}
	next := func() (tadwRenderedEvent, bool, error) {
		for {
			if err := ctx.Err(); err != nil {
				return tadwRenderedEvent{}, false, err
			}
			if !cursor.Next() {
				return tadwRenderedEvent{}, false, errors.Join(cursor.Err(), ctx.Err())
			}
			event := cursor.Event()
			if event.Ref.Id == request.Id {
				continue
			}
			if seen[event.Ref.Id] {
				return tadwRenderedEvent{}, false, fmt.Errorf("duplicate TADW context event %q", event.Ref.Id)
			}
			projected, err := hydrate(event)
			if err != nil {
				return tadwRenderedEvent{}, false, err
			}
			fragment, _, err := renderTADWEvent(projected)
			seen[event.Ref.Id] = true
			return fragment, err == nil, err
		}
	}
	tooling, err := selectTADWToolingRollover(pinned, next, budget)
	if err = errors.Join(err, cursor.Close(), ctx.Err()); err != nil {
		return tadwToolingWindow{}, fmt.Errorf("roll TADW tooling: %w", err)
	}
	if len(tooling) < 2 {
		return tadwToolingWindow{}, fmt.Errorf("TADW tooling rollover found no retained tool event")
	}
	return tadwToolingWindow{FromEventID: tooling[1].ID, Transcript: tadwTranscript{History: slices.Clone(history.History), Tooling: tooling}}, nil
}

// Validate the frozen buffer/accounting, but do not invoke window selection.
// Byte limits are checked before adding, including with INT64_MAX configuration.
func tadwFixedHistoryBudget(history tadwInitialHistory, config tadwWindowConfig) (int64, error) {
	if err := config.validate(); err != nil {
		return 0, err
	}
	if len(history.Window.EventIDs) != len(history.History) {
		return 0, fmt.Errorf("invalid TADW fixed history: event IDs do not match fragments")
	}
	seen := make(map[string]bool, len(history.History))
	var used int64
	for index, fragment := range history.History {
		if fragment.ID == "" || fragment.Bytes() == 0 || fragment.ID != history.Window.EventIDs[index] || seen[fragment.ID] {
			return 0, fmt.Errorf("invalid TADW fixed history fragment %q", fragment.ID)
		}
		if fragment.Bytes() > config.HistoryMaxBytes-used {
			return 0, fmt.Errorf("TADW fixed history exceeds history_max_bytes")
		}
		used += fragment.Bytes()
		seen[fragment.ID] = true
	}
	base := ""
	if len(history.History) != 0 {
		base = history.History[0].ID
	}
	budget := config.ContextMaxBytes - used
	if history.Window.BaseEventID != base || history.Window.HistoryBytes != used || history.Window.ToolingBudgetBytes != budget {
		return 0, fmt.Errorf("invalid TADW fixed history: inconsistent base or byte accounting")
	}
	return budget, nil
}

type tadwToolingSource interface {
	Next() bool
	Event() model.SessionEvent
	Err() error
}

// collectTADWTooling consumes prompt-order events. Unlike a history cutoff, exact
// fit is not permission to stop: another renderable event means overflow. No
// splitting, skipping a crossing event, or successful partial tooling is allowed.
func collectTADWTooling(ctx context.Context, source tadwToolingSource, hydrate func(model.SessionEvent) (model.SessionEvent, error), history []tadwRenderedEvent, budget int64) (tadwTranscript, error) {
	if budget < 0 {
		return tadwTranscript{}, fmt.Errorf("TADW tooling budget must be nonnegative")
	}
	seen := make(map[string]bool, len(history))
	for _, event := range history {
		seen[event.ID] = true
	}
	var tooling []tadwRenderedEvent
	var used int64
	for {
		if err := ctx.Err(); err != nil {
			return tadwTranscript{}, err
		}
		if !source.Next() {
			break
		}
		event := source.Event()
		switch event.Kind {
		case model.SessionEventKindAgentRequest, model.SessionEventKindToolRequest,
			model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure:
		default:
			continue
		}
		if seen[event.Ref.Id] {
			return tadwTranscript{}, fmt.Errorf("duplicate TADW context event %q", event.Ref.Id)
		}
		projected, err := hydrate(event)
		if err != nil {
			return tadwTranscript{}, err
		}
		fragment, _, err := renderTADWEvent(projected)
		if err != nil {
			return tadwTranscript{}, err
		}
		if fragment.Bytes() > budget-used {
			return tadwTranscript{}, &tadwToolingBudgetError{BudgetBytes: budget, UsedBytes: used, EventBytes: fragment.Bytes(), EventID: fragment.ID}
		}
		tooling = append(tooling, fragment)
		used += fragment.Bytes()
		seen[fragment.ID] = true
	}
	if err := source.Err(); err != nil {
		return tadwTranscript{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwTranscript{}, err
	}
	// Copy slice headers/elements for ownership; immutable XML strings are reused.
	return tadwTranscript{History: slices.Clone(history), Tooling: tooling}, nil
}
