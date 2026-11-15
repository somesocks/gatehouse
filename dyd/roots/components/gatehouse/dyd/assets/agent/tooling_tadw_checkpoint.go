package agent

import (
	"context"
	"fmt"

	"gatehouse/database"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const tadwToolingCheckpointName = "gatehouse.tadw-tooling-window"

// checkpointTADWTooling belongs once per thinking attempt at a stable workflow
// position. Callers carry FromEventID in ordinary memory to the next attempt.
// Every attempt records a boundary decision, even when unchanged: conditionally
// adding a step only after an uncheckpointed overflow could diverge on replay.
//
// Only the boundary string is the step result, never XML, event payloads, or the
// initial-history ID list. Fresh execution reuses its selected buffer. Replay
// renders the recorded window without rerunning rollover policy.
func checkpointTADWTooling(ctx dbos.Context, store *database.Store, history tadwInitialHistory, request model.SessionEventRef, agent model.WorkspaceAgentRef, thinking model.SessionEventRef, config tadwWindowConfig, from string) (tadwToolingWindow, error) {
	if _, err := tadwFixedHistoryBudget(history, config); err != nil {
		return tadwToolingWindow{}, err
	}
	var fresh *tadwToolingWindow
	boundary, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (string, error) {
		selected, err := loadTADWToolingWindow(stepCtx, store, history, request, agent, thinking, config, from)
		if err != nil {
			return "", err
		}
		fresh = &selected
		return selected.FromEventID, nil
	}, dbos.WithStepName(tadwToolingCheckpointName))
	if err != nil {
		return tadwToolingWindow{}, fmt.Errorf("checkpoint TADW tooling: %w", err)
	}
	if fresh != nil {
		return *fresh, nil
	}
	transcript, err := loadTADWContextFrom(ctx, store, history, request, agent, thinking, config, boundary)
	if err != nil {
		return tadwToolingWindow{}, fmt.Errorf("rehydrate TADW tooling: %w", err)
	}
	return tadwToolingWindow{FromEventID: boundary, Transcript: transcript}, nil
}
