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
	contextBufferBytes   = 16 * 1024
	contextEventLimit = 128
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
	value := selected.Ref.Model.Id + "\x00" + selected.Compaction + "\x00" + openAISystemPromptFor(selected)
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
		state, err := json.Marshal(mcmtrContextState{})
		if err != nil {
			return false, err
		}
		return true, runtime.store.AgentContextSet(step, database.AgentContext{Root: root, Model: selected.Ref, Profile: fingerprint, State: state})
	}, dbos.WithStepName("gatehouse.session-event-context-start"))
	return stepErr
}

func (runtime *SessionEventReplyRuntime) mcmtrContextFinish(ctx dbos.Context, root model.SessionEventRef) error {
	_, stepErr := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
		return true, runtime.store.AgentContextDelete(step, root)
	}, dbos.WithStepName("gatehouse.session-event-context-finish"))
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
		err, events := runtime.store.SessionEventsTailGet(step, parent.Session, contextEventLimit)
		if err != nil {
			return nil, err
		}
		active := false
		for _, event := range events {
			if event.Ref == parent {
				active = true
				break
			}
		}
		if !active {
			err, event := runtime.store.SessionEventGet(step, parent)
			if err != nil || event == nil {
				if err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("reply to session event %q: active user message is unavailable", parent.Id)
			}
			events = append(events, *event)
		}
		sort.Slice(events, func(left, right int) bool { return events[left].Ref.Id < events[right].Ref.Id })
		return events, nil
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

func selectedMCMTRProfile(selected *database.WorkspaceAgentModel) (mcmtrProfile, error) {
	profile := mcmtrProfile{Algorithm: "mcmtr", HistoryBytes: contextMaximumBytes, BufferBytes: contextBufferBytes}
	if err := json.Unmarshal([]byte(selected.Compaction), &profile); err != nil {
		return mcmtrProfile{}, fmt.Errorf("decode MCMTR profile for model %q: %w", selected.Ref.Model.Id, err)
	}
	if profile.Algorithm != "mcmtr" || profile.HistoryBytes <= 0 || profile.BufferBytes <= 0 || profile.BufferBytes > profile.HistoryBytes/6 {
		return mcmtrProfile{}, fmt.Errorf("invalid MCMTR profile for model %q", selected.Ref.Model.Id)
	}
	return profile, nil
}
