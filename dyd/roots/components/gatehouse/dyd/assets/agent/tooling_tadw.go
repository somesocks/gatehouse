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
// history, checkpoints context, or silently trims tooling. The path is inactive.
func loadTADWContext(ctx context.Context, store *database.Store, history tadwInitialHistory, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef, config tadwWindowConfig) (tadwTranscript, error) {
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
	err, cursor := store.SessionTADWToolingCursor(ctx, tx, request, agent, thinking)
	if err != nil {
		return tadwTranscript{}, err
	}
	defer cursor.Close()
	hydrate := func(event model.SessionEvent) (model.SessionEvent, error) {
		err, projected := store.SessionEventFilesHydrateInTransaction(ctx, tx, event)
		return projected, err
	}
	transcript, err := collectTADWTooling(ctx, cursor, hydrate, history.History, budget)
	if err = errors.Join(err, cursor.Close()); err != nil {
		return tadwTranscript{}, fmt.Errorf("load TADW tooling: %w", err)
	}
	return transcript, nil
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
