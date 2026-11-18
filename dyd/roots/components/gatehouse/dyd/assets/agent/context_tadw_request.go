package agent

import (
	"context"
	"fmt"
	"slices"

	"gatehouse/database"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// The request workflow owns these IDs and the latest returned boundary. Rendered
// events and provider bodies belong only to the preparation call that uses them.
type tadwRequestContext struct {
	request    model.SessionEventRef
	choice     agentContextSelection
	historyIDs []string
	boundary   string
}

func tadwRequestCompatibility(agent model.WorkspaceAgentRef, scope tadwHistoryCacheScope, limits tadwWindowConfig) (string, error) {
	return tadwPreparationDigest(struct {
		Version string
		Agent   model.WorkspaceAgentRef
		Scope   tadwHistoryCacheScope
		Limits  tadwWindowConfig
	}{"tadw-request-v1", agent, scope, limits})
}

func (state *tadwRequestContext) compatible(selected *database.WorkspaceAgentModel, projectChat bool) (tadwHistoryCacheScope, error) {
	if err := state.choice.matches(selected); err != nil {
		return tadwHistoryCacheScope{}, err
	}
	scope, err := tadwHistoryCacheScopeForAgent(state.request.Session, selected, projectChat)
	if err != nil {
		return scope, err
	}
	fingerprint, err := tadwRequestCompatibility(selected.Ref, scope, *state.choice.TADW)
	if err != nil {
		return scope, err
	}
	if fingerprint != state.choice.Compatibility {
		return scope, fmt.Errorf("TADW selected configuration or limits changed")
	}
	return scope, nil
}

// Selection runs once. Replayed IDs are consumed directly from the declared
// step result; payloads are read only by fresh boundary/provider preparations.
func (runtime *SessionEventReplyRuntime) checkpointRequestTADWHistory(ctx dbos.Context, request model.SessionEventRef, agent model.WorkspaceAgentRef, choice agentContextSelection) (*tadwRequestContext, error) {
	state := &tadwRequestContext{request: request, choice: choice}
	ids, err := dbos.RunAsStep(ctx, func(step context.Context) ([]string, error) {
		err, root := runtime.store.SessionEventGet(step, request)
		if err != nil {
			return nil, err
		}
		if root == nil || root.Parent == nil || root.Kind != model.SessionEventKindAgentRequest || root.Payload["agent"] != agent.Id {
			return nil, fmt.Errorf("invalid TADW owning request")
		}
		err, selected := runtime.selectedAgent(step, agent)
		if err != nil {
			return nil, err
		}
		err, project := runtime.store.SessionProjectGet(step, request.Session)
		if err != nil {
			return nil, err
		}
		scope, err := state.compatible(selected, project != nil)
		if err != nil {
			return nil, err
		}
		hints, err := loadTADWHistoryCacheHints(step, runtime.store, scope)
		if err != nil {
			return nil, err
		}
		history, err := loadTADWHistory(step, runtime.store, *root.Parent, hints, *choice.TADW)
		if err != nil {
			return nil, err
		}
		return slices.Clone(history.Window.EventIDs), nil
	}, dbos.WithStepName(tadwHistoryCheckpointName))
	if err != nil {
		return nil, err
	}
	state.historyIDs = ids
	return state, nil
}

func (runtime *SessionEventReplyRuntime) checkpointRequestTADWTooling(ctx dbos.Context, state *tadwRequestContext, thinking model.SessionEvent) error {
	boundary, err := dbos.RunAsStep(ctx, func(step context.Context) (string, error) {
		err, selected := runtime.selectedAgent(step, *thinking.AuthorAgent)
		if err != nil {
			return "", err
		}
		err, project := runtime.store.SessionProjectGet(step, thinking.Ref.Session)
		if err != nil {
			return "", err
		}
		if _, err := state.compatible(selected, project != nil); err != nil {
			return "", err
		}
		history, err := rehydrateTADWHistory(step, runtime.store, state.request.Session, state.historyIDs, *state.choice.TADW)
		if err != nil {
			return "", err
		}
		hints, err := loadTADWRequestHints(step, runtime.store, state.request, selected.Ref)
		if err != nil {
			return "", err
		}
		suffix, err := renderTADWRequestHints(selected, hints)
		if err != nil {
			return "", err
		}
		window, err := loadTADWToolingWindow(step, runtime.store, history, state.request, selected.Ref, thinking.Ref, *state.choice.TADW, state.boundary, int64(len(suffix)))
		return window.FromEventID, err
	}, dbos.WithStepName(tadwToolingCheckpointName))
	if err == nil {
		state.boundary = boundary
	}
	return err
}

func (runtime *SessionEventReplyRuntime) renderRequestTADW(ctx context.Context, state *tadwRequestContext, thinking model.SessionEvent, selected *database.WorkspaceAgentModel, projectChat bool) (agentProviderRequest, error) {
	if err := ctx.Err(); err != nil {
		return agentProviderRequest{}, err
	}
	if thinking.Parent == nil || *thinking.Parent != state.request || thinking.AuthorAgent == nil || *thinking.AuthorAgent != selected.Ref {
		return agentProviderRequest{}, fmt.Errorf("TADW thinking request does not match its owner")
	}
	scope, err := state.compatible(selected, projectChat)
	if err != nil {
		return agentProviderRequest{}, err
	}
	history, err := rehydrateTADWHistory(ctx, runtime.store, state.request.Session, state.historyIDs, *state.choice.TADW)
	if err != nil {
		return agentProviderRequest{}, err
	}
	hints, err := loadTADWRequestHints(ctx, runtime.store, state.request, selected.Ref)
	if err != nil {
		return agentProviderRequest{}, err
	}
	suffix, err := renderTADWRequestHints(selected, hints)
	if err != nil {
		return agentProviderRequest{}, err
	}
	transcript, err := loadTADWContextFrom(ctx, runtime.store, history, state.request, selected.Ref, thinking.Ref, *state.choice.TADW, state.boundary, int64(len(suffix)))
	if err != nil {
		return agentProviderRequest{}, err
	}
	err, request := agentProviderRequestPrepareTADW(selected, projectChat, tadwPreparedHistory{Agent: selected.Ref, Scope: scope, History: history}, transcript, *state.choice.TADW, hints)
	return request, err
}
