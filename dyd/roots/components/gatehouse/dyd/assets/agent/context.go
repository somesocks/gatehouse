package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"gatehouse/database"
	"gatehouse/model"
)

const (
	contextMaximumBytes   = 96 * 1024
	contextBufferBytes    = 48 * 1024
	contextEventPageSize  = 128
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

func mcmtrProfileFingerprint(selected *database.WorkspaceAgentModel, projectChat bool) string {
	value := mcmtrAlgorithmVersion + "\x00" + selected.AgentModel.Id + "\x00" + selected.Compaction + "\x00" + openAISystemPromptFor(selected, projectChat)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func (runtime *SessionEventReplyRuntime) mcmtrContextStart(ctx context.Context, root model.SessionEventRef, selected *database.WorkspaceAgentModel, projectChat bool) error {
	_, err := selectedMCMTRProfile(selected)
	if err != nil {
		return err
	}
	fingerprint := mcmtrProfileFingerprint(selected, projectChat)
	err, stored := runtime.store.AgentContextGet(ctx, root)
	if err != nil {
		return err
	}
	if stored != nil && stored.Model == selected.Ref {
		return nil
	}
	state := json.RawMessage(nil)
	err, inherited := runtime.store.AgentContextPreviousGet(ctx, root.Session, selected.Ref, root.Id)
	if err != nil {
		return err
	}
	if inherited != nil && mcmtrContextStateBefore(inherited.State, root.Id) {
		state = inherited.State
	} else {
		state, err = json.Marshal(mcmtrContextState{})
		if err != nil {
			return err
		}
	}
	return runtime.store.AgentContextSet(ctx, database.AgentContext{Root: root, Model: selected.Ref, Profile: fingerprint, State: state})
}

func (runtime *SessionEventReplyRuntime) agentContext(ctx context.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, projectChat bool) (error, []openAICompatibleMessage) {
	profile, err := selectedMCMTRProfile(selected)
	if err != nil {
		return err, nil
	}
	state := mcmtrContextState{}
	err, stored := runtime.store.AgentContextGet(ctx, parent)
	if err != nil {
		return err, nil
	}
	if stored != nil && stored.Model == selected.Ref {
		if err := json.Unmarshal(stored.State, &state); err != nil {
			return fmt.Errorf("decode MCMTR state for session event %q: %w", parent.Id, err), nil
		}
	}
	err, events := runtime.mcmtrContextEvents(ctx, parent, selected.Ref)
	if err != nil {
		return err, nil
	}
	err, active := mcmtrContextActive(events, parent)
	if err != nil {
		return err, nil
	}
	messages, next, err := compileMCMTRContext(events, active, profile, state)
	if err != nil {
		return err, nil
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err, nil
	}
	err = runtime.store.AgentContextSet(ctx, database.AgentContext{Root: parent, Model: selected.Ref, Profile: mcmtrProfileFingerprint(selected, projectChat), State: encoded})
	if err != nil {
		return err, nil
	}
	return nil, messages
}

func mcmtrContextStateBefore(state json.RawMessage, before string) bool {
	checkpoint := mcmtrContextState{}
	if json.Unmarshal(state, &checkpoint) != nil {
		return false
	}
	for _, id := range []string{checkpoint.UserHighFrom, checkpoint.AgentHighFrom, checkpoint.ToolHighFrom} {
		if id != "" && id >= before {
			return false
		}
	}
	return true
}

func (runtime *SessionEventReplyRuntime) mcmtrContextEvents(ctx context.Context, parent model.SessionEventRef, agent model.WorkspaceAgentRef) (error, []model.SessionEvent) {
	err, events := runtime.store.SessionAgentRequestEventsGet(ctx, parent, agent)
	if err != nil {
		return err, nil
	}
	if len(events) == 0 {
		return fmt.Errorf("reply to session event %q: active agent request is unavailable", parent.Id), nil
	}
	for index := range events {
		if events[index].Kind != model.SessionEventKindMessageText || events[index].AuthorPrincipal == nil {
			continue
		}
		payload := make(map[string]interface{}, len(events[index].Payload))
		for key, value := range events[index].Payload {
			if key != "agents" {
				payload[key] = value
			}
		}
		events[index].Payload = payload
	}
	return nil, events
}

func mcmtrContextActive(events []model.SessionEvent, request model.SessionEventRef) (error, model.SessionEventRef) {
	for _, event := range events {
		if event.Ref != request {
			continue
		}
		if event.Kind != model.SessionEventKindAgentRequest || event.Parent == nil {
			return fmt.Errorf("reply to session event %q: active agent request is invalid", request.Id), model.SessionEventRef{}
		}
		return nil, *event.Parent
	}
	return fmt.Errorf("reply to session event %q: active agent request is unavailable", request.Id), model.SessionEventRef{}
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
