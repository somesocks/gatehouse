package agent

import (
	"context"
	"fmt"
	"log/slog"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type agentProviderEstimate struct {
	Ready  bool `json:"ready"`
	Tokens *int `json:"tokens,omitempty"`
}

// Estimation and execution have separate lifetimes around the durable rate-limit
// wait. Only readiness/token counts and outcome references are checkpointed;
// requests and responses remain inside their step callbacks.
func (runtime *SessionEventReplyRuntime) agentProviderAttempt(ctx dbos.Context, parent model.SessionEventRef, thinking model.SessionEvent, principal model.PrincipalRef, round, offset int, choice agentContextSelection, state *tadwRequestContext) (model.SessionEventRef, database.AgentRateLimitClaim, error) {
	estimate, err := dbos.RunAsStep(ctx, func(step context.Context) (agentProviderEstimate, error) {
		err, selected, request := runtime.agentProviderRequestForThinking(step, thinking, state)
		// Preparation failures are materialized as thinking outcomes by execution.
		if err != nil {
			return agentProviderEstimate{}, nil
		}
		if err := choice.matches(selected); err != nil {
			return agentProviderEstimate{}, nil
		}
		if request.body() == nil {
			return agentProviderEstimate{Ready: true}, nil
		}
		tokens, err := estimateAgentRequestTokens(request.body())
		return agentProviderEstimate{Ready: true, Tokens: &tokens}, err
	}, dbos.WithStepName("gatehouse.agent-provider-estimate"))
	if err != nil {
		return model.SessionEventRef{}, database.AgentRateLimitClaim{}, err
	}
	selected := &database.WorkspaceAgentModel{Ref: *thinking.AuthorAgent}
	claimErr, claim := runtime.agentRateLimitClaim(ctx, parent, thinking, selected, principal, estimate.Tokens)
	if claimErr != nil {
		return model.SessionEventRef{}, claim, runtime.thinkingFinish(ctx, thinking, model.SessionEventKindThinkingFailure, claimErr)
	}
	reserved, err := dbos.RunAsStep(ctx, func(context.Context) (model.SessionEventRef, error) {
		id, err := typed_id.New(typed_id.SessionEvent)
		return model.SessionEventRef{Session: thinking.Ref.Session, Id: id}, err
	}, dbos.WithStepName("gatehouse.agent-provider-outcome-id"))
	if err != nil {
		return model.SessionEventRef{}, claim, err
	}
	if reserved.Session != thinking.Ref.Session || !typed_id.Valid(typed_id.SessionEvent, reserved.Id) {
		return model.SessionEventRef{}, claim, fmt.Errorf("invalid reserved provider outcome reference")
	}
	outcome, err := dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEventRef, error) {
		// Application persistence can commit before DBOS records this step result.
		// The reserved reference identifies that same attempt across this crash gap.
		exists, err := runtime.providerOutcomeExists(step, reserved, thinking)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if exists {
			return reserved, nil
		}
		if err := runtime.replyCancellationCheck(step, parent); err != nil {
			return model.SessionEventRef{}, err
		}
		preparationErr, current, request := runtime.agentProviderRequestForThinking(step, thinking, state)
		if current != nil {
			selected = current
		}
		if preparationErr == nil {
			preparationErr = choice.matches(selected)
		}
		if preparationErr == nil && !estimate.Ready {
			preparationErr = fmt.Errorf("provider request was unavailable during token estimation")
		}
		if preparationErr == nil && request.body() != nil && estimate.Tokens == nil {
			preparationErr = fmt.Errorf("provider request requires a token estimate")
		}
		turn := agentProviderTurn{Failure: preparationErr}
		if preparationErr == nil {
			sendErr, result, _ := runtime.agentProviderSend(step, parent, selected, request, claim)
			turn = result
			if sendErr != nil {
				turn.Failure = sendErr
			}
		}
		var hintScope tadwHistoryCacheScope
		var hintBase model.SessionEventRef
		if state != nil && len(state.historyIDs) > 0 && turn.Failure == nil {
			var hintErr error
			hintScope, hintErr = tadwHistoryCacheScopeForSentRequest(parent.Session, selected, request)
			if hintErr == nil {
				hintBase = model.SessionEventRef{Session: parent.Session, Id: state.historyIDs[0]}
			} else {
				slog.WarnContext(step, "derive TADW history alignment hint", "error", hintErr)
			}
		}
		request = agentProviderRequest{}
		if err := runtime.replyCancellationCheck(step, parent); err != nil {
			return model.SessionEventRef{}, err
		}
		candidate := runtime.agentProviderDecisionCandidate(step, parent, selected, round, turn, choice)
		diagnostic := ""
		if candidate.Failure != nil {
			diagnostic = candidate.Failure.Error()
		}
		failureBody, err := runtime.agentResultStore(step, thinking, diagnostic, resultInlineLimit(selected))
		if err != nil {
			return model.SessionEventRef{}, err
		}
		var events []model.SessionEvent
		ref, err := persistProviderDecision(parent, thinking, selected, round, offset, candidate, failureBody, reserved, func(event model.SessionEvent) (model.SessionEventRef, error) {
			events = append(events, event)
			return event.Ref, nil
		})
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if err, _ := runtime.store.SessionEventsCreateBatch(step, events); err != nil {
			// Concurrent executions of the same interrupted step share the reservation.
			// A committed outcome wins; a failed transaction leaves no partial decision.
			if exists, lookupErr := runtime.providerOutcomeExists(step, reserved, thinking); lookupErr == nil && exists {
				return reserved, nil
			}
			return model.SessionEventRef{}, err
		}
		// Optional alignment work follows the durable application decision. A crash
		// before publication may lose a hint, but cannot repeat a committed send.
		if hintBase.Id != "" {
			if err := publishTADWHistoryCacheHint(step, runtime.store, hintScope, hintBase); err != nil {
				slog.WarnContext(step, "publish TADW history alignment hint", "error", err)
			}
		}
		return ref, nil
	}, dbos.WithStepName("gatehouse.agent-provider-execute"))
	return outcome, claim, err
}

func (runtime *SessionEventReplyRuntime) providerOutcomeExists(ctx context.Context, ref model.SessionEventRef, thinking model.SessionEvent) (bool, error) {
	err, event := runtime.store.SessionEventGet(ctx, ref)
	if err != nil {
		return false, err
	}
	if event == nil {
		return false, nil
	}
	if event.Parent == nil || *event.Parent != thinking.Ref || event.AuthorAgent == nil || thinking.AuthorAgent == nil || *event.AuthorAgent != *thinking.AuthorAgent {
		return false, fmt.Errorf("provider outcome reference belongs to another thinking request")
	}
	switch event.Kind {
	case model.SessionEventKindThinkingSuccess, model.SessionEventKindThinkingFailure, model.SessionEventKindThinkingUpdate:
		return true, nil
	default:
		return false, fmt.Errorf("provider outcome reference has invalid kind %q", event.Kind)
	}
}
