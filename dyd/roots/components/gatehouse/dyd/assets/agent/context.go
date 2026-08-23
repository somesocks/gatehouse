package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/database"
	"gatehouse/model"
)

const (
	contextMaximumBytes = 80 * 1024
	contextSafetyBytes  = 16 * 1024
)

func (runtime *SessionEventReplyRuntime) agentContext(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel) (error, []openAICompatibleMessage) {
	events, err := dbos.RunAsStep(ctx, func(step context.Context) ([]model.SessionEvent, error) {
		err, entries := runtime.store.SessionEventsTreePageGet(step, parent.Session, "", 1<<30)
		if err != nil {
			return nil, err
		}
		events := make([]model.SessionEvent, len(entries))
		for index, entry := range entries {
			events[index] = entry.Event
		}
		err, events = runtime.store.SessionEventAttachmentsHydrate(step, parent.Session, events)
		return events, err
	}, dbos.WithStepName("gatehouse.session-event-context-load"))
	if err != nil {
		return err, nil
	}
	err, messages := openAICompatibleMessages(events)
	if err != nil {
		return err, nil
	}
	if len(messages) == 0 {
		return fmt.Errorf("reply to session event %q: message has no text", parent.Id), nil
	}
	messages = selectContextMessages(messages, contextByteBudget(selected))
	messages = renderTranscriptMessages(messages)
	return nil, messages
}

func contextByteBudget(selected *database.WorkspaceAgentModel) int {
	staticBytes := len(openAISystemPromptFor(selected))
	if selected.Protocol == "openai-responses" {
		encoded, _ := json.Marshal(openAIResponsesLispTool())
		staticBytes += len(encoded)
		encoded, _ = json.Marshal(openAIResponsesFinalReplyText())
		staticBytes += len(encoded)
	} else {
		encoded, _ := json.Marshal(openAICompatibleLispTool())
		staticBytes += len(encoded)
		encoded, _ = json.Marshal(openAIFinalReplyResponseFormat())
		staticBytes += len(encoded)
	}
	budget := selected.MaxInputTokens - selected.MaxOutputTokens - contextSafetyBytes - staticBytes
	if budget > contextMaximumBytes {
		return contextMaximumBytes
	}
	if budget < 0 {
		return 0
	}
	return budget
}
