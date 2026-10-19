package httpservice

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gatehouse/apischema"
	"gatehouse/database"
	"gatehouse/model"
)

type transcriptEventTreeResponse struct {
	Event    any                            `json:"event"`
	Children []*transcriptEventTreeResponse `json:"children"`
}

func transcriptEventWire(event model.SessionEvent, agents map[model.WorkspaceAgentRef]database.SessionEventAgent) (error, any) {
	if event.Kind == model.SessionEventKindToolFailure {
		_, output := event.Payload["output"]
		_, file := event.Payload["output_file"]
		if output == file {
			return fmt.Errorf("tool failure requires exactly one of output and output_file"), nil
		}
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal transcript event %q: %w", event.Ref.Id, err), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var input map[string]any
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("decode transcript event %q: %w", event.Ref.Id, err), nil
	}
	if event.AuthorAgent != nil {
		author, exists := agents[*event.AuthorAgent]
		if !exists {
			return fmt.Errorf("transcript author bot %q is unavailable", event.AuthorAgent.Id), nil
		}
		input["author_agent"] = transcriptAgentWire(author)
	}
	if event.Kind == model.SessionEventKindAgentRequest {
		if id, ok := event.Payload["agent"].(string); ok {
			if target, exists := agents[model.WorkspaceAgentRef{Workspace: event.Ref.Session.Workspace, Id: id}]; exists {
				input["target_agent"] = transcriptAgentWire(target)
			}
		}
	}
	if event.Kind == model.SessionEventKindMessageText {
		payload, ok := input["payload"].(map[string]any)
		if !ok {
			return fmt.Errorf("transcript message payload must be an object"), nil
		}
		if _, canonical := payload["targets"]; !canonical {
			if ids, ok := event.Payload["agents"]; ok {
				encoded, err := json.Marshal(ids)
				if err != nil {
					return err, nil
				}
				var legacy []string
				if err := json.Unmarshal(encoded, &legacy); err != nil {
					return err, nil
				}
				targets := []any{}
				for _, id := range legacy {
					target := map[string]any{"ref": map[string]any{"kind": "agent", "id": id}}
					if agent, exists := agents[model.WorkspaceAgentRef{Workspace: event.Ref.Session.Workspace, Id: id}]; exists {
						target["alias"] = agent.Alias
						if agent.Label != nil {
							target["name"] = *agent.Label
						}
					}
					targets = append(targets, target)
				}
				payload["targets"] = targets
			}
		}
		delete(payload, "agents")
	}
	err, typed := apischema.DecodeTranscriptEvent(input)
	if err != nil {
		return fmt.Errorf("type transcript event %q (%s): %w", event.Ref.Id, event.Kind, err), nil
	}
	err, wire := apischema.EncodeTranscriptEvent(typed)
	if err != nil {
		return fmt.Errorf("encode transcript event %q (%s): %w", event.Ref.Id, event.Kind, err), nil
	}
	return nil, wire
}

func transcriptAgentWire(agent database.SessionEventAgent) map[string]any {
	value := map[string]any{"ref": map[string]any{"id": agent.Ref.Id, "workspace": map[string]any{"id": agent.Ref.Workspace.Id}}, "enabled": agent.Enabled, "alias": agent.Alias}
	if agent.Label != nil {
		value["label"] = *agent.Label
	}
	return value
}

func transcriptEventTrees(trees []*sessionEventTreeResponse, agents map[model.WorkspaceAgentRef]database.SessionEventAgent) (error, []*transcriptEventTreeResponse) {
	encoded := make([]*transcriptEventTreeResponse, 0, len(trees))
	for _, tree := range trees {
		err, event := transcriptEventWire(tree.Event, agents)
		if err != nil {
			return err, nil
		}
		err, children := transcriptEventTrees(tree.Children, agents)
		if err != nil {
			return err, nil
		}
		encoded = append(encoded, &transcriptEventTreeResponse{Event: event, Children: children})
	}
	return nil, encoded
}
