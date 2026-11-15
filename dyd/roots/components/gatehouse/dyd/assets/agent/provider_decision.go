package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type providerToolRequest struct {
	CallID string
	Code   string
	Reason string
}

type providerDecisionCandidate struct {
	Failure    error
	RetryUntil *time.Time
	Tools      []providerToolRequest
	Final      *agentFinalReply
	Metrics    *model.SessionEventMetrics
}

// agentProviderDecisionCommit stores the provider's complete decision and the
// DBOS result together. The provider call and candidate validation have already
// completed, so the database transaction covers only Gatehouse writes.
func (runtime *SessionEventReplyRuntime) agentProviderDecisionCommit(
	ctx dbos.Context,
	parent model.SessionEventRef,
	thinking model.SessionEvent,
	selected *database.WorkspaceAgentModel,
	round, callCount int,
	turn agentProviderTurn,
	choice agentContextSelection,
) (error, model.SessionEventRef) {
	candidate := runtime.agentProviderDecisionCandidate(ctx, parent, selected, round, turn, choice)
	diagnostic := ""
	if candidate.Failure != nil {
		diagnostic = candidate.Failure.Error()
	}
	failureBody, publicationErr := runtime.agentResultStore(ctx, thinking, diagnostic, resultInlineLimit(selected))

	decision, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, transaction dbos.Tx) (model.SessionEventRef, error) {
		if publicationErr != nil {
			return model.SessionEventRef{}, publicationErr
		}
		return persistProviderDecision(parent, thinking, selected, round, callCount, candidate, failureBody, model.SessionEventRef{}, func(event model.SessionEvent) (model.SessionEventRef, error) {
			err, stored := runtime.store.SessionEventCreateInTransaction(step, transaction, event)
			return stored.Ref, err
		})
	}, dbos.WithStepName("gatehouse.agent-provider-decision"))
	return err, decision
}

// The caller supplies atomic persistence for all events and may reserve the
// thinking outcome reference as the provider attempt's idempotency identity.
func persistProviderDecision(parent model.SessionEventRef, thinking model.SessionEvent, selected *database.WorkspaceAgentModel, round, callCount int, candidate providerDecisionCandidate, failureBody agentResultBody, reserved model.SessionEventRef, storeEvent func(model.SessionEvent) (model.SessionEventRef, error)) (model.SessionEventRef, error) {
	result := model.SessionEventRef{}
	persist := func(event model.SessionEvent) (model.SessionEventRef, error) {
		if reserved.Id != "" && event.Parent != nil && *event.Parent == thinking.Ref {
			event.Ref = reserved
		} else {
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				return model.SessionEventRef{}, fmt.Errorf("generate provider decision event ID: %w", err)
			}
			event.Ref.Id = id
		}
		return storeEvent(event)
	}

	if candidate.RetryUntil != nil {
		until := candidate.RetryUntil.UTC().Format(time.RFC3339Nano)
		payload, err := database.SessionEventPayloadFrom(model.ThinkingUpdatePayload{Reason: "rate_limit", Until: &until})
		if err != nil {
			return result, fmt.Errorf("encode provider retry event: %w", err)
		}
		result, err = persist(model.SessionEvent{
			Ref: model.SessionEventRef{Session: thinking.Ref.Session}, Parent: &thinking.Ref,
			Kind: model.SessionEventKindThinkingUpdate, AuthorAgent: thinking.AuthorAgent,
			Payload: payload, Metrics: candidate.Metrics,
		})
		return result, err
	}

	if candidate.Failure != nil {
		failure := model.ThinkingFailurePayload{Code: failureEventCode(candidate.Failure), Message: failureBody.Output, OutputFile: failureBody.OutputFile}
		if failureBody.OutputFile != nil {
			failure.Message = failureBody.Message
		}
		payload, err := database.SessionEventPayloadFrom(failure)
		if err != nil {
			return result, fmt.Errorf("encode provider failure event: %w", err)
		}
		result, err = persist(model.SessionEvent{
			Ref: model.SessionEventRef{Session: thinking.Ref.Session}, Parent: &thinking.Ref,
			Kind: model.SessionEventKindThinkingFailure, AuthorAgent: thinking.AuthorAgent,
			Payload: payload, Metrics: candidate.Metrics,
		})
		return result, err
	}

	outcome := model.ThinkingSuccessPayload{}
	if candidate.Final != nil {
		payload, err := database.SessionEventPayloadFrom(model.AgentSuccessPayload{
			Text: candidate.Final.Text, Attachments: optionalStringSlice(candidate.Final.Attachments),
		})
		if err != nil {
			return result, fmt.Errorf("encode provider final reply: %w", err)
		}
		final, err := persist(model.SessionEvent{
			Ref: model.SessionEventRef{Session: parent.Session}, Parent: &parent,
			Kind: model.SessionEventKindAgentSuccess, AuthorAgent: &selected.Ref, Payload: payload,
		})
		if err != nil {
			return result, err
		}
		outcome.AgentSuccess = &final
	} else {
		tools := make([]model.SessionEventRef, 0, len(candidate.Tools))
		for index, call := range candidate.Tools {
			batch := round
			position := callCount + index
			payload, err := database.SessionEventPayloadFrom(model.ToolRequestPayload{
				Name: "lisp", CallId: call.CallID, Code: call.Code, Reason: call.Reason,
				Batch: &batch, Position: &position,
			})
			if err != nil {
				return result, fmt.Errorf("encode tool request %q: %w", call.CallID, err)
			}
			reference, err := persist(model.SessionEvent{
				Ref: model.SessionEventRef{Session: parent.Session}, Parent: &parent,
				Kind: model.SessionEventKindToolRequest, AuthorAgent: &selected.Ref, Payload: payload,
			})
			if err != nil {
				return result, err
			}
			tools = append(tools, reference)
		}
		if len(tools) == 0 {
			return result, fmt.Errorf("provider decision has neither a final reply nor tool calls")
		}
		outcome.ToolRequests = &tools
	}
	payload, err := database.SessionEventPayloadFrom(outcome)
	if err != nil {
		return result, fmt.Errorf("encode provider thinking success: %w", err)
	}
	return persist(model.SessionEvent{
		Ref: model.SessionEventRef{Session: thinking.Ref.Session}, Parent: &thinking.Ref,
		Kind: model.SessionEventKindThinkingSuccess, AuthorAgent: thinking.AuthorAgent,
		Payload: payload, Metrics: candidate.Metrics,
	})
}

func (runtime *SessionEventReplyRuntime) agentProviderDecisionCandidate(
	ctx context.Context,
	parent model.SessionEventRef,
	selected *database.WorkspaceAgentModel,
	round int,
	turn agentProviderTurn,
	choice agentContextSelection,
) providerDecisionCandidate {
	candidate := providerDecisionCandidate{Failure: turn.Failure, Metrics: turn.Metrics}
	if candidate.Failure != nil {
		var retryable *openAIRetryableError
		if errors.As(candidate.Failure, &retryable) {
			until := time.Now().UTC().Add(retryable.RetryAfter())
			candidate.RetryUntil = &until
			candidate.Failure = nil
		}
		return candidate
	}
	if turn.Final != nil {
		final := *turn.Final
		if len(final.Attachments) > 0 {
			err, references := runtime.store.SessionFileReferencesFilter(ctx, parent.Session, final.Attachments)
			if err != nil {
				candidate.Failure = err
				return candidate
			}
			final.Attachments = make([]string, len(references))
			for index, reference := range references {
				final.Attachments[index] = reference.ID
			}
		}
		candidate.Final = &final
		return candidate
	}
	if round >= selected.MaxTurns {
		candidate.Failure = fmt.Errorf("agent provider completion exceeded turn limit")
		return candidate
	}
	if choice.Algorithm == "mcmtr" {
		profile, err := selectedMCMTRProfile(selected)
		if err != nil {
			candidate.Failure = err
			return candidate
		}
		if !mcmtrNativeCallsFit(turn.Calls, profile.BufferBytes) {
			candidate.Failure = fmt.Errorf("OpenAI-compatible completion requested a tool batch that exceeds the MCMTR tool high-tier buffer")
			return candidate
		}
	}
	if len(turn.Calls) == 0 {
		candidate.Failure = fmt.Errorf("OpenAI-compatible completion returned neither a final reply nor tool calls")
		return candidate
	}
	seen := make(map[string]bool, len(turn.Calls))
	candidate.Tools = make([]providerToolRequest, 0, len(turn.Calls))
	for _, call := range turn.Calls {
		code, reason, err := openAICompatibleLispArguments(call)
		if err != nil {
			candidate.Failure = err
			candidate.Tools = nil
			return candidate
		}
		if seen[call.ID] {
			candidate.Failure = fmt.Errorf("OpenAI-compatible completion requested a duplicate tool call ID")
			candidate.Tools = nil
			return candidate
		}
		seen[call.ID] = true
		candidate.Tools = append(candidate.Tools, providerToolRequest{CallID: call.ID, Code: code, Reason: reason})
	}
	return candidate
}
