package agent

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type agentContextSelection struct {
	Algorithm     string            `json:"algorithm"`
	TADW          *tadwWindowConfig `json:"tadw,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
}

// A missing Context identifies admissions recorded before request-owned rounds.
// The embedded reference preserves their serialized result format.
type agentRoundAdmission struct {
	model.SessionEventRef
	Context *agentContextSelection `json:"context,omitempty"`
}

func selectedAgentContext(selected *database.WorkspaceAgentModel) (agentContextSelection, error) {
	value, err := config.ParseAgentModelCompaction(selected.Compaction)
	if err != nil {
		return agentContextSelection{}, err
	}
	result := agentContextSelection{Algorithm: value.Algorithm()}
	if value.TADW != nil {
		if selected.Protocol == "builtin" {
			return result, fmt.Errorf("TADW requires an external provider")
		}
		result.TADW = &tadwWindowConfig{ContextMaxBytes: value.TADW.ContextMaxBytes, HistoryMaxBytes: value.TADW.HistoryMaxBytes, HistoryMinBytes: value.TADW.HistoryMinBytes}
	}
	return result, nil
}

func (runtime *SessionEventReplyRuntime) applyPromptReplacement(selected *database.WorkspaceAgentModel) {
	selected.SystemPromptReplacement = runtime.systemPromptReplacement
	if text, ok := runtime.systemPromptReplacements[selected.Ref]; ok {
		selected.SystemPromptReplacement = &text
	}
}

func (runtime *SessionEventReplyRuntime) selectedAgent(ctx context.Context, agent model.WorkspaceAgentRef) (error, *database.WorkspaceAgentModel) {
	err, selected := runtime.store.WorkspaceAgentModelGet(ctx, agent.Workspace, agent.Id)
	if err != nil {
		return err, nil
	}
	if selected == nil {
		return fmt.Errorf("agent model or provider is unavailable"), nil
	}
	runtime.applyPromptReplacement(selected)
	return nil, selected
}

func (choice agentContextSelection) matches(selected *database.WorkspaceAgentModel) error {
	current, err := selectedAgentContext(selected)
	if err != nil {
		return err
	}
	if choice.Algorithm != current.Algorithm || (choice.TADW == nil) != (current.TADW == nil) || (choice.TADW != nil && *choice.TADW != *current.TADW) {
		return fmt.Errorf("selected context algorithm or limits changed during the request")
	}
	return nil
}
