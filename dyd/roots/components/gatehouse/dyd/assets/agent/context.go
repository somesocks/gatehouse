package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/database"
	"gatehouse/model"
)

const (
	contextMaximumBytes  = 96 * 1024
	contextBufferBytes   = 48 * 1024
	contextEventPageSize = 128
	mcmtrAlgorithmVersion = "mcmtr-v3"
)

type mcmtrProfile struct {
	Algorithm    string `json:"algorithm"`
	HistoryBytes int    `json:"history_bytes"`
	BufferBytes  int    `json:"buffer_bytes"`
}

type mcmtrContextState struct {
	UserHighFrom  string `json:"user_high_from,omitempty"`
	AgentHighFrom string `json:"agent_high_from,omitempty"`
	ToolHighFrom  string `json:"tool_high_from,omitempty"`
}

func mcmtrProfileFingerprint(selected *database.WorkspaceAgentModel) string {
	value := mcmtrAlgorithmVersion + "\x00" + selected.AgentModel.Id + "\x00" + selected.Compaction + "\x00" + openAISystemPromptFor(selected)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func (runtime *SessionEventReplyRuntime) mcmtrContextStart(ctx dbos.Context, root model.SessionEventRef, selected *database.WorkspaceAgentModel) error {
	_, err := selectedMCMTRProfile(selected)
	if err != nil {
		return err
	}
	fingerprint := mcmtrProfileFingerprint(selected)
	_, stepErr := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
		err, stored := runtime.store.AgentContextGet(step, root)
		if err != nil {
			return false, err
		}
		if stored != nil && stored.Model == selected.Ref && stored.Profile == fingerprint {
			return true, nil
		}
		state := json.RawMessage(nil)
		err, inherited := runtime.store.AgentContextLatestGet(step, root.Session, selected.Ref, fingerprint)
		if err != nil {
			return false, err
		}
		if inherited != nil {
			state = inherited.State
		} else {
			state, err = json.Marshal(mcmtrContextState{})
			if err != nil {
				return false, err
			}
		}
		return true, runtime.store.AgentContextSet(step, database.AgentContext{Root: root, Model: selected.Ref, Profile: fingerprint, State: state})
	}, dbos.WithStepName("gatehouse.session-event-context-start"))
	return stepErr
}

func (runtime *SessionEventReplyRuntime) agentContext(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel) (error, []openAICompatibleMessage) {
	profile, err := selectedMCMTRProfile(selected)
	if err != nil {
		return err, nil
	}
	state := mcmtrContextState{}
	events, err := dbos.RunAsStep(ctx, func(step context.Context) ([]model.SessionEvent, error) {
		err, stored := runtime.store.AgentContextGet(step, parent)
		if err != nil {
			return nil, err
		}
		if stored != nil && stored.Model == selected.Ref && stored.Profile == mcmtrProfileFingerprint(selected) {
			if err := json.Unmarshal(stored.State, &state); err != nil {
				return nil, fmt.Errorf("decode MCMTR state for session event %q: %w", parent.Id, err)
			}
		}
		return runtime.mcmtrContextEvents(step, parent, profile, state)
	}, dbos.WithStepName("gatehouse.session-event-context-load"))
	if err != nil {
		return err, nil
	}
	messages, next, err := compileMCMTRContext(events, parent, profile, state)
	if err != nil {
		return err, nil
	}
	_, err = dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
		encoded, err := json.Marshal(next)
		if err != nil {
			return false, err
		}
		return true, runtime.store.AgentContextSet(step, database.AgentContext{Root: parent, Model: selected.Ref, Profile: mcmtrProfileFingerprint(selected), State: encoded})
	}, dbos.WithStepName("gatehouse.session-event-context-checkpoint"))
	if err != nil {
		return err, nil
	}
	return nil, messages
}

func (runtime *SessionEventReplyRuntime) mcmtrContextEvents(ctx context.Context, parent model.SessionEventRef, profile mcmtrProfile, state mcmtrContextState) ([]model.SessionEvent, error) {
	err, active := runtime.store.SessionEventGet(ctx, parent)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, fmt.Errorf("reply to session event %q: active user message is unavailable", parent.Id)
	}
	beforeID := ""
	events := make([]model.SessionEvent, 0, contextEventPageSize)
	for {
		err, page := runtime.store.SessionEventsTailPageGet(ctx, parent.Session, beforeID, contextEventPageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return mcmtrContextEventsWithActive(events, *active), nil
		}
		events = append(events, page...)
		contextEvents := mcmtrContextEventsWithActive(events, *active)
		_, _, shared, err := compileMCMTRContextWithShared(contextEvents, parent, profile, state)
		if err != nil {
			return nil, err
		}
		oldestID := page[0].Ref.Id
		needOlder, err := mcmtrHighTiersNeedOlder(contextEvents, profile, state, oldestID)
		if err != nil {
			return nil, err
		}
		if len(page) < contextEventPageSize || (shared == 0 && !needOlder) {
			return contextEvents, nil
		}
		beforeID = oldestID
	}
}

func mcmtrContextEventsWithActive(events []model.SessionEvent, active model.SessionEvent) []model.SessionEvent {
	result := append([]model.SessionEvent(nil), events...)
	for _, event := range result {
		if event.Ref == active.Ref {
			sort.Slice(result, func(left, right int) bool { return result[left].Ref.Id < result[right].Ref.Id })
			return result
		}
	}
	result = append(result, active)
	sort.Slice(result, func(left, right int) bool { return result[left].Ref.Id < result[right].Ref.Id })
	return result
}

func mcmtrHighTiersNeedOlder(events []model.SessionEvent, profile mcmtrProfile, state mcmtrContextState, oldestID string) (bool, error) {
	used := 0
	missingCheckpoint := false
	events = mcmtrAnnotateRoots(events)
	for _, event := range events {
		record, include, err := mcmtrRecordFor(event)
		if err != nil {
			return false, err
		}
		if !include {
			continue
		}
		record.stream = mcmtrChannel(event)
		checkpoint := mcmtrHighCheckpoint(state, record.stream)
		if checkpoint == "" || record.event.Ref.Id >= checkpoint {
			used += mcmtrRecordCost(record)
		}
	}
	for _, stream := range mcmtrStreams {
		checkpoint := mcmtrHighCheckpoint(state, stream)
		if checkpoint != "" {
			if oldestID > checkpoint {
				return true, nil
			}
			continue
		}
		missingCheckpoint = true
	}
	if missingCheckpoint && used <= profile.BufferBytes {
		return true, nil
	}
	return false, nil
}

func mcmtrHighCheckpoint(state mcmtrContextState, stream string) string {
	switch stream {
	case "user":
		return state.UserHighFrom
	case "agent":
		return state.AgentHighFrom
	case "tool":
		return state.ToolHighFrom
	default:
		return ""
	}
}

func mcmtrSetHighCheckpoint(state *mcmtrContextState, stream, checkpoint string) {
	switch stream {
	case "user":
		state.UserHighFrom = checkpoint
	case "agent":
		state.AgentHighFrom = checkpoint
	case "tool":
		state.ToolHighFrom = checkpoint
	}
}

func selectedMCMTRProfile(selected *database.WorkspaceAgentModel) (mcmtrProfile, error) {
	profile := mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}
	if err := json.Unmarshal([]byte(selected.Compaction), &profile); err != nil {
		return mcmtrProfile{}, fmt.Errorf("decode MCMTR profile for model %q: %w", selected.AgentModel.Id, err)
	}
	if profile.Algorithm != "mcmtr" || profile.HistoryBytes <= 0 || profile.BufferBytes <= 0 || profile.BufferBytes > profile.HistoryBytes/2 {
		return mcmtrProfile{}, fmt.Errorf("invalid MCMTR profile for model %q", selected.AgentModel.Id)
	}
	return profile, nil
}
