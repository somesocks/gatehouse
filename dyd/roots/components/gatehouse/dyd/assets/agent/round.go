package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type SessionAgentRoundInput struct {
	AgentRequest model.SessionEventRef
	Previous     *model.SessionEventRef
}

func sessionAgentRoundWorkflowID(input SessionAgentRoundInput) string {
	if input.Previous == nil {
		return "session-agent-round:" + input.AgentRequest.Id + ":first"
	}
	return "session-agent-round:" + input.AgentRequest.Id + ":after:" + input.Previous.Id
}

// The parent only coordinates reference-valued children. Completed rounds do
// not re-enter provider preparation or HTTP when this orchestration replays.
func (runtime *SessionEventReplyRuntime) runAgentRounds(ctx dbos.Context, request model.SessionEventRef) (model.SessionEventRef, error) {
	input := SessionAgentRoundInput{AgentRequest: request}
	for {
		if err := runtime.replyCancellationCheck(ctx, request); err != nil {
			return model.SessionEventRef{}, err
		}
		handle, err := dbos.RunWorkflow(ctx, runtime.agentRound, input,
			dbos.WithRunInstance(runtime), dbos.WithWorkflowID(sessionAgentRoundWorkflowID(input)))
		if err != nil {
			return model.SessionEventRef{}, fmt.Errorf("start agent round: %w", err)
		}
		reference, err := handle.GetResult()
		if err != nil {
			return model.SessionEventRef{}, fmt.Errorf("await agent round: %w", err)
		}
		err, outcome, decision := runtime.agentRoundOutcomeGet(ctx, request, reference)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if outcome.Kind == model.SessionEventKindThinkingFailure {
			return model.SessionEventRef{}, fmt.Errorf("agent round failed; see thinking outcome %q", reference.Id)
		}
		if decision.AgentSuccess != nil {
			return *decision.AgentSuccess, nil
		}
		if _, err := runtime.runLispCalls(ctx, *decision.ToolRequests); err != nil {
			return model.SessionEventRef{}, err
		}
		input.Previous = &reference
	}
}

func (runtime *SessionEventReplyRuntime) agentRound(ctx dbos.Context, input SessionAgentRoundInput) (model.SessionEventRef, error) {
	if err := runtime.replyCancellationCheck(ctx, input.AgentRequest); err != nil {
		return model.SessionEventRef{}, err
	}
	thinkingRef, err := runtime.agentRoundPrepare(ctx, input)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	err, thinking := runtime.store.SessionEventGet(ctx, thinkingRef)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if thinking == nil || thinking.AuthorAgent == nil {
		return model.SessionEventRef{}, fmt.Errorf("round thinking request %q is unavailable", thinkingRef.Id)
	}
	if thinking.Kind != model.SessionEventKindThinkingRequest || thinking.Parent == nil || *thinking.Parent != input.AgentRequest {
		return model.SessionEventRef{}, fmt.Errorf("round thinking request belongs to another agent request")
	}
	err, root := runtime.store.SessionEventGet(ctx, input.AgentRequest)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if root == nil || root.Kind != model.SessionEventKindAgentRequest || root.AuthorPrincipal == nil || root.Payload["agent"] != thinking.AuthorAgent.Id {
		return model.SessionEventRef{}, fmt.Errorf("round agent request is unavailable")
	}
	var payload model.ThinkingRequestPayload
	if err := decodeRoundPayload(thinking, &payload); err != nil {
		return model.SessionEventRef{}, err
	}
	round, offset, err := agentRoundHistory(SessionAgentRoundInput{AgentRequest: root.Ref, Previous: payload.Previous}, *thinking.AuthorAgent,
		func(ref model.SessionEventRef) (error, *model.SessionEvent) {
			return runtime.store.SessionEventGet(ctx, ref)
		})
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if payload.Turn != round {
		return model.SessionEventRef{}, fmt.Errorf("round thinking turn does not match committed history")
	}
	selected := &database.WorkspaceAgentModel{Ref: *thinking.AuthorAgent}
	for {
		err, turn, claim := runtime.agentProviderTurn(ctx, input.AgentRequest, *thinking, selected, root.AuthorPrincipal.Ref)
		if err != nil {
			return model.SessionEventRef{}, runtime.thinkingFinishWithMetrics(ctx, *thinking, model.SessionEventKindThinkingFailure, err, turn.Metrics)
		}
		if err := runtime.replyCancellationCheck(ctx, input.AgentRequest); err != nil {
			return model.SessionEventRef{}, err
		}
		err, outcomeRef := runtime.agentProviderDecisionCommit(ctx, input.AgentRequest, *thinking, selected, round, offset, turn)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		err, outcome := runtime.store.SessionEventGet(ctx, outcomeRef)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if outcome == nil || outcome.Parent == nil || *outcome.Parent != thinkingRef {
			return model.SessionEventRef{}, fmt.Errorf("agent round returned an invalid thinking outcome")
		}
		var providerErr error
		if outcome.Kind == model.SessionEventKindThinkingUpdate {
			providerErr = &openAIRetryableError{}
		}
		inputTokens, outputTokens := agentUsageTokens(providerErr, outcome.Metrics)
		if err := runtime.agentRateLimitSettle(ctx, claim, inputTokens, outputTokens); err != nil {
			return model.SessionEventRef{}, err
		}
		if outcome.Kind != model.SessionEventKindThinkingUpdate {
			if err, _ := runtime.agentRoundOutcomeValidate(ctx, input.AgentRequest, outcome); err != nil {
				return model.SessionEventRef{}, err
			}
			return outcomeRef, nil
		}
		until, ok := outcome.Payload["until"].(string)
		if !ok {
			return model.SessionEventRef{}, fmt.Errorf("provider retry has no deadline")
		}
		retryAt, err := time.Parse(time.RFC3339Nano, until)
		if err != nil {
			return model.SessionEventRef{}, fmt.Errorf("provider retry has an invalid deadline: %w", err)
		}
		if _, err := dbos.Sleep(ctx, max(time.Until(retryAt), 0)); err != nil {
			return model.SessionEventRef{}, runtime.thinkingFinish(ctx, *thinking, model.SessionEventKindThinkingFailure, err)
		}
	}
}

// Preparation commits only the lifecycle marker and its checkpoint reference.
// Request construction happens at each attempt, using bounded canonical history.
func (runtime *SessionEventReplyRuntime) agentRoundPrepare(ctx dbos.Context, input SessionAgentRoundInput) (model.SessionEventRef, error) {
	return dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, tx dbos.Tx) (model.SessionEventRef, error) {
		get := func(ref model.SessionEventRef) (error, *model.SessionEvent) {
			return runtime.store.SessionEventGetInTransaction(step, tx, ref)
		}
		err, root := get(input.AgentRequest)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if root == nil || root.Kind != model.SessionEventKindAgentRequest || root.Parent == nil || root.AuthorPrincipal == nil {
			return model.SessionEventRef{}, fmt.Errorf("agent round request is invalid")
		}
		err, message := get(*root.Parent)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if message == nil || message.Kind != model.SessionEventKindMessageText || message.AuthorPrincipal == nil || message.Ref.Session != root.Ref.Session || message.AuthorPrincipal.Ref != root.AuthorPrincipal.Ref {
			return model.SessionEventRef{}, fmt.Errorf("agent round message is invalid")
		}
		agentID, _ := root.Payload["agent"].(string)
		err, selected := runtime.store.WorkspaceAgentModelGetInTransaction(step, tx, root.Ref.Session.Workspace, agentID)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if selected == nil {
			return model.SessionEventRef{}, fmt.Errorf("agent round model is unavailable")
		}
		if err, active := runtime.store.SessionAgentParticipantGetInTransaction(step, tx, root.Ref.Session, selected.Ref); err != nil {
			return model.SessionEventRef{}, err
		} else if !active {
			return model.SessionEventRef{}, fmt.Errorf("bot is not a participant in this chat")
		}
		if err, authorized := runtime.store.SessionRequestAuthorizedGetInTransaction(step, tx, root.Ref); err != nil {
			return model.SessionEventRef{}, err
		} else if !authorized {
			return model.SessionEventRef{}, fmt.Errorf("requesting principal is no longer authorized")
		}
		round, _, err := agentRoundHistory(input, selected.Ref, get)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		payload, err := database.SessionEventPayloadFrom(model.ThinkingRequestPayload{Turn: round, Previous: input.Previous})
		if err != nil {
			return model.SessionEventRef{}, err
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		event := model.SessionEvent{Ref: model.SessionEventRef{Session: root.Ref.Session, Id: id}, Parent: &root.Ref,
			Kind: model.SessionEventKindThinkingRequest, AuthorAgent: &selected.Ref, Payload: payload}
		if err, _ := runtime.store.SessionEventCreateInTransaction(step, tx, event); err != nil {
			return model.SessionEventRef{}, err
		}
		return event.Ref, nil
	}, dbos.WithStepName("gatehouse.agent-round-prepare"))
}

func (runtime *SessionEventReplyRuntime) agentRoundContext(ctx context.Context, thinking model.SessionEvent, selected *database.WorkspaceAgentModel) (error, bool, []openAICompatibleMessage) {
	if thinking.Parent == nil {
		return fmt.Errorf("round thinking request has no parent"), false, nil
	}
	root := *thinking.Parent
	err, project := runtime.store.SessionProjectGet(ctx, root.Session)
	if err != nil {
		return err, false, nil
	}
	projectChat := project != nil
	profile, err := selectedMCMTRProfile(selected)
	if err != nil {
		return err, projectChat, nil
	}
	state := mcmtrContextState{}
	fingerprint := mcmtrProfileFingerprint(selected, projectChat)
	compatible := func(stored *database.AgentContext) bool {
		return stored != nil && stored.Model == selected.Ref && stored.Profile == fingerprint && mcmtrContextStateBefore(stored.State, thinking.Ref.Id)
	}
	// Cache watermarks per thinking request. A later round cannot overwrite the
	// cache used to rebuild this round, and prompt/profile changes invalidate it.
	err, stored := runtime.store.AgentContextGet(ctx, thinking.Ref)
	if err != nil {
		return err, projectChat, nil
	}
	if !compatible(stored) {
		stored = nil
		var payload model.ThinkingRequestPayload
		if err := decodeRoundPayload(&thinking, &payload); err != nil {
			return err, projectChat, nil
		}
		if payload.Previous != nil {
			err, previous := runtime.store.SessionEventGet(ctx, *payload.Previous)
			if err != nil {
				return err, projectChat, nil
			}
			if previous == nil || previous.Kind != model.SessionEventKindThinkingSuccess || previous.Parent == nil || previous.AuthorAgent == nil || *previous.AuthorAgent != selected.Ref || previous.Ref.Session != root.Session {
				return fmt.Errorf("round context has an invalid predecessor"), projectChat, nil
			}
			err, stored = runtime.store.AgentContextGet(ctx, *previous.Parent)
			if err != nil {
				return err, projectChat, nil
			}
		}
	}
	if !compatible(stored) {
		err, stored = runtime.store.AgentContextPreviousGet(ctx, root.Session, selected.Ref, root.Id)
		if err != nil {
			return err, projectChat, nil
		}
		if !compatible(stored) || !mcmtrContextStateBefore(stored.State, root.Id) {
			stored = nil
		}
	}
	if stored != nil {
		if err := json.Unmarshal(stored.State, &state); err != nil {
			return fmt.Errorf("decode round context: %w", err), projectChat, nil
		}
	}
	err, events := runtime.store.SessionAgentRoundEventsGet(ctx, root, selected.Ref, thinking.Ref)
	if err != nil {
		return err, projectChat, nil
	}
	err, events = runtime.store.SessionEventOutputFilesHydrate(ctx, events)
	if err != nil {
		return err, projectChat, nil
	}
	for index := range events {
		if events[index].Kind == model.SessionEventKindMessageText {
			delete(events[index].Payload, "agents")
		}
	}
	err, active := mcmtrContextActive(events, root)
	if err != nil {
		return err, projectChat, nil
	}
	messages, next, err := compileMCMTRContext(events, active, profile, state)
	if err != nil {
		return err, projectChat, nil
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err, projectChat, nil
	}
	if err := runtime.store.AgentContextSet(ctx, database.AgentContext{
		Root: thinking.Ref, Model: selected.Ref, Profile: fingerprint, State: encoded,
	}); err != nil {
		return err, projectChat, nil
	}
	return nil, projectChat, messages
}

func agentRoundHistory(input SessionAgentRoundInput, agent model.WorkspaceAgentRef, get func(model.SessionEventRef) (error, *model.SessionEvent)) (int, int, error) {
	round, offset := 0, 0
	previous := input.Previous
	seen := map[string]bool{}
	for previous != nil {
		if previous.Session != input.AgentRequest.Session || seen[previous.Id] {
			return 0, 0, fmt.Errorf("invalid round predecessor chain")
		}
		seen[previous.Id] = true
		err, outcome := get(*previous)
		if err != nil {
			return 0, 0, err
		}
		if outcome == nil || outcome.Kind != model.SessionEventKindThinkingSuccess || outcome.Parent == nil || outcome.AuthorAgent == nil || *outcome.AuthorAgent != agent {
			return 0, 0, fmt.Errorf("round predecessor is not a successful outcome for this agent")
		}
		decision, err := agentRoundDecision(outcome)
		if err != nil || decision.ToolRequests == nil || decision.AgentSuccess != nil {
			return 0, 0, fmt.Errorf("round predecessor is not a tool decision")
		}
		err, thinking := get(*outcome.Parent)
		if err != nil {
			return 0, 0, err
		}
		if thinking == nil || thinking.Kind != model.SessionEventKindThinkingRequest || thinking.Parent == nil || *thinking.Parent != input.AgentRequest {
			return 0, 0, fmt.Errorf("round predecessor belongs to another agent request")
		}
		for _, ref := range *decision.ToolRequests {
			if ref.Session != input.AgentRequest.Session {
				return 0, 0, fmt.Errorf("round tool belongs to another session")
			}
			err, tool := get(ref)
			if err != nil {
				return 0, 0, err
			}
			if tool == nil || tool.Kind != model.SessionEventKindToolRequest || tool.Parent == nil || *tool.Parent != input.AgentRequest || tool.AuthorAgent == nil || *tool.AuthorAgent != agent {
				return 0, 0, fmt.Errorf("round predecessor has an invalid tool request")
			}
		}
		round++
		offset += len(*decision.ToolRequests)
		var payload model.ThinkingRequestPayload
		if err := decodeRoundPayload(thinking, &payload); err != nil {
			return 0, 0, err
		}
		previous = payload.Previous
	}
	return round, offset, nil
}

func decodeRoundPayload(event *model.SessionEvent, target any) error {
	encoded, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func agentRoundDecision(outcome *model.SessionEvent) (model.ThinkingSuccessPayload, error) {
	var decision model.ThinkingSuccessPayload
	if err := decodeRoundPayload(outcome, &decision); err != nil {
		return decision, err
	}
	hasTools := decision.ToolRequests != nil && len(*decision.ToolRequests) > 0
	hasFinal := decision.AgentSuccess != nil
	if hasTools == hasFinal || decision.ToolRequests != nil && !hasTools {
		return decision, fmt.Errorf("thinking success must identify exactly one committed decision")
	}
	seen := map[string]bool{}
	if hasTools {
		for _, ref := range *decision.ToolRequests {
			if !typed_id.Valid(typed_id.SessionEvent, ref.Id) || ref.Session != outcome.Ref.Session || seen[ref.Id] {
				return decision, fmt.Errorf("thinking success has an invalid tool reference")
			}
			seen[ref.Id] = true
		}
	} else if !typed_id.Valid(typed_id.SessionEvent, decision.AgentSuccess.Id) || decision.AgentSuccess.Session != outcome.Ref.Session {
		return decision, fmt.Errorf("thinking success has an invalid final reference")
	}
	return decision, nil
}

func (runtime *SessionEventReplyRuntime) agentRoundOutcomeGet(ctx context.Context, root, ref model.SessionEventRef) (error, *model.SessionEvent, model.ThinkingSuccessPayload) {
	if root.Session != ref.Session {
		return fmt.Errorf("round outcome belongs to another session"), nil, model.ThinkingSuccessPayload{}
	}
	err, outcome := runtime.store.SessionEventGet(ctx, ref)
	if err != nil {
		return err, nil, model.ThinkingSuccessPayload{}
	}
	err, decision := runtime.agentRoundOutcomeValidate(ctx, root, outcome)
	return err, outcome, decision
}

func (runtime *SessionEventReplyRuntime) agentRoundOutcomeValidate(ctx context.Context, root model.SessionEventRef, outcome *model.SessionEvent) (error, model.ThinkingSuccessPayload) {
	empty := model.ThinkingSuccessPayload{}
	if outcome == nil || outcome.Parent == nil || outcome.Ref.Session != root.Session || outcome.AuthorAgent == nil {
		return fmt.Errorf("round outcome is invalid"), empty
	}
	err, thinking := runtime.store.SessionEventGet(ctx, *outcome.Parent)
	if err != nil {
		return err, empty
	}
	if thinking == nil || thinking.Kind != model.SessionEventKindThinkingRequest || thinking.Parent == nil || *thinking.Parent != root || thinking.AuthorAgent == nil || *thinking.AuthorAgent != *outcome.AuthorAgent {
		return fmt.Errorf("round outcome belongs to another request"), empty
	}
	if outcome.Kind == model.SessionEventKindThinkingFailure {
		if code, _ := outcome.Payload["code"].(string); code == "" {
			return fmt.Errorf("round failure has no code"), empty
		}
		return nil, empty
	}
	if outcome.Kind != model.SessionEventKindThinkingSuccess {
		return fmt.Errorf("round returned a non-terminal outcome"), empty
	}
	decision, err := agentRoundDecision(outcome)
	if err != nil {
		return err, empty
	}
	refs := []model.SessionEventRef{}
	wantKind := model.SessionEventKindAgentSuccess
	if decision.ToolRequests != nil {
		refs = *decision.ToolRequests
		wantKind = model.SessionEventKindToolRequest
	} else {
		refs = append(refs, *decision.AgentSuccess)
	}
	for _, ref := range refs {
		err, event := runtime.store.SessionEventGet(ctx, ref)
		if err != nil {
			return err, empty
		}
		if event == nil || event.Kind != wantKind || event.Parent == nil || *event.Parent != root || event.AuthorAgent == nil || *event.AuthorAgent != *outcome.AuthorAgent {
			return fmt.Errorf("round decision reference is invalid"), empty
		}
	}
	return nil, decision
}
