package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"gatehouse/model"
)

// SessionEventAgent is display metadata for an event reference, not membership
// or invitation eligibility. It intentionally excludes bot configuration.
type SessionEventAgent struct {
	Ref     model.WorkspaceAgentRef `json:"ref"`
	Alias   string                  `json:"alias"`
	Label   *string                 `json:"label,omitempty"`
	Enabled bool                    `json:"enabled"`
}

// SessionEventAgentsGet resolves only identities referenced by a loaded event
// page. Callers authorize the chat before loading its events. Disabled bindings
// remain resolvable for historical attribution; chat history is never searched.
func (store *Store) SessionEventAgentsGet(ctx context.Context, session model.SessionRef, events []model.SessionEvent) (error, map[model.WorkspaceAgentRef]SessionEventAgent) {
	ids := map[string]struct{}{}
	for _, event := range events {
		if event.Ref.Session != session {
			return fmt.Errorf("get event bot metadata: event belongs to another chat"), nil
		}
		if event.AuthorAgent != nil {
			if event.AuthorAgent.Workspace != session.Workspace {
				return fmt.Errorf("get event bot metadata: author belongs to another workspace"), nil
			}
			ids[event.AuthorAgent.Id] = struct{}{}
		}
		if event.Kind == model.SessionEventKindAgentRequest {
			if id, ok := event.Payload["agent"].(string); ok {
				ids[id] = struct{}{}
			}
		}
		if event.Kind == model.SessionEventKindMessageText {
			if _, canonical := event.Payload["targets"]; !canonical {
				if legacy, ok := eventStringSlice(event.Payload["agents"]); ok {
					for _, id := range legacy {
						ids[id] = struct{}{}
					}
				}
			}
		}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		if id != "" {
			ordered = append(ordered, id)
		}
	}
	sort.Strings(ordered)
	agents := map[model.WorkspaceAgentRef]SessionEventAgent{}
	p := keychainPlaceholder(store.kind)
	const batchSize = 200
	for start := 0; start < len(ordered); start += batchSize {
		end := min(start+batchSize, len(ordered))
		args := []any{session.Workspace.Id}
		placeholders := []string{}
		for _, id := range ordered[start:end] {
			args = append(args, id)
			placeholders = append(placeholders, p(len(args)))
		}
		rows, err := store.QueryContext(ctx, `SELECT id, alias, label, enabled FROM gatehouse_workspace_agents WHERE workspace_id = `+p(1)+` AND id IN (`+strings.Join(placeholders, ", ")+`)`, args...)
		if err != nil {
			return fmt.Errorf("get event bot metadata: %w", err), nil
		}
		for rows.Next() {
			agent := SessionEventAgent{Ref: model.WorkspaceAgentRef{Workspace: session.Workspace}}
			var label sql.NullString
			if err := rows.Scan(&agent.Ref.Id, &agent.Alias, &label, &agent.Enabled); err != nil {
				rows.Close()
				return err, nil
			}
			if label.Valid {
				agent.Label = &label.String
			}
			agents[agent.Ref] = agent
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err, nil
		}
	}
	return nil, agents
}
