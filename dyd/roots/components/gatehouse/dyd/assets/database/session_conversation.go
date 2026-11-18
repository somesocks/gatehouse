package database

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/jackc/pgx/v5"
)

//go:embed session_conversation_sqlite.sql
var conversationSQLiteSQL string

//go:embed session_conversation_postgres.sql
var conversationPostgresSQL string

var ErrConversationQuery = errors.New("invalid conversation branch query")
var ErrConversationParent = errors.New("conversation branch parent is unavailable")

type ConversationBranchQuery struct {
	Parent string
	Cursor string
	Limit  int
}

type ConversationEntry struct {
	Event          model.SessionEvent
	Previews       []model.SessionEvent
	HasMoreReplies bool
}

type ConversationAgentRequest struct {
	Request               model.SessionEventRef
	State                 string
	Outcome               *model.SessionEvent
	OutcomePreviews       []model.SessionEvent
	OutcomeHasMoreReplies bool
	Interactions          []model.SessionEventRef
}

type ConversationInteraction struct {
	Request  model.SessionEvent
	Response *model.SessionEvent
}

type ConversationBranch struct {
	Entries       []ConversationEntry
	AgentRequests []ConversationAgentRequest
	Interactions  []ConversationInteraction
	NextCursor    string
}

type conversationCursor struct {
	Version   int    `json:"v"`
	Workspace string `json:"workspace"`
	Session   string `json:"session"`
	Parent    string `json:"parent"`
	At        string `json:"at"`
	ID        string `json:"id"`
}

type conversationRow struct {
	relation, owner string
	event           model.SessionEvent
}

// SessionConversationBranchGet reads one same-parent conversational page and
// one preview level. Agent execution is projected into per-request state and
// interactions, not recursive tool/thinking transcripts. The selection uses one
// SQL statement/snapshot; cancellation and prompt discovery can still scan a
// selected request's children/tools. It does not authorize the caller or hydrate
// file metadata. Raw event payloads must be projected before public exposure.
func (store *Store) SessionConversationBranchGet(ctx context.Context, session model.SessionRef, input ConversationBranchQuery) (error, *ConversationBranch) {
	if !typed_id.Valid(typed_id.Workspace, session.Workspace.Id) || !typed_id.Valid(typed_id.Session, session.Id) || input.Limit < 1 || input.Limit > 100 || input.Parent != "" && !typed_id.Valid(typed_id.SessionEvent, input.Parent) {
		return ErrConversationQuery, nil
	}
	cursor := conversationCursor{Version: 1, Workspace: session.Workspace.Id, Session: session.Id, Parent: input.Parent, At: "0001-01-01T00:00:00Z"}
	if input.Cursor != "" {
		if len(input.Cursor) > 2048 {
			return ErrConversationQuery, nil
		}
		encoded, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil {
			return ErrConversationQuery, nil
		}
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.DisallowUnknownFields()
		var parsed conversationCursor
		if err := decoder.Decode(&parsed); err != nil {
			return ErrConversationQuery, nil
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return ErrConversationQuery, nil
		}
		if parsed.Version != 1 || parsed.Workspace != cursor.Workspace || parsed.Session != cursor.Session || parsed.Parent != cursor.Parent || !typed_id.Valid(typed_id.SessionEvent, parsed.ID) {
			return ErrConversationQuery, nil
		}
		if _, err := time.Parse(time.RFC3339Nano, parsed.At); err != nil {
			return ErrConversationQuery, nil
		}
		cursor = parsed
	}
	var parent any
	if input.Parent != "" {
		parent = input.Parent
	}
	query := conversationSQLiteSQL
	args := []any{sql.Named("workspace", session.Workspace.Id), sql.Named("session", session.Id), sql.Named("parent", parent), sql.Named("after_time", cursor.At), sql.Named("after_id", cursor.ID), sql.Named("page_size", input.Limit)}
	if store.kind == config.DatabaseKindPostgres {
		values := pgx.StrictNamedArgs{}
		for _, arg := range args {
			named := arg.(sql.NamedArg)
			values[named.Name] = named.Value
		}
		var err error
		query, args, err = values.RewriteQuery(ctx, nil, conversationPostgresSQL, nil)
		if err != nil {
			return fmt.Errorf("bind conversation branch: %w", err), nil
		}
	}
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("read conversation branch: %w", err), nil
	}
	defer rows.Close()
	selected := []conversationRow{}
	more := false
	for rows.Next() {
		var relation string
		var owner, parent, kind, at sql.NullString
		var id string
		var slot int
		var principal, alias, name, agent, gateway, payload, metrics sql.NullString
		var enabled sql.NullBool
		if err := rows.Scan(&relation, &owner, &slot, &id, &parent, &kind, &at, &principal, &alias, &name, &enabled, &agent, &gateway, &payload, &metrics); err != nil {
			return fmt.Errorf("scan conversation branch: %w", err), nil
		}
		switch relation {
		case "scope":
			if id != "valid" {
				return ErrConversationParent, nil
			}
			continue
		case "branch-more":
			more = true
			continue
		case "preview-more", "outcome-preview-more":
			selected = append(selected, conversationRow{relation: relation, owner: owner.String})
			continue
		}
		authorPrincipal, authorAgent, authorGateway, err := sessionEventAuthorsFromValues(session.Workspace, principal, alias, name, enabled, agent, gateway)
		if err != nil {
			return err, nil
		}
		event := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Kind: kind.String, CreatedAt: at.String, AuthorPrincipal: authorPrincipal, AuthorAgent: authorAgent, AuthorGateway: authorGateway}
		if parent.Valid {
			event.Parent = &model.SessionEventRef{Session: session, Id: parent.String}
		}
		if !payload.Valid {
			return fmt.Errorf("conversation event %q is unavailable", id), nil
		}
		if err := json.Unmarshal([]byte(payload.String), &event.Payload); err != nil {
			return fmt.Errorf("decode conversation event %q: %w", id, err), nil
		}
		var metricsErr error
		event.Metrics, metricsErr = sessionEventMetricsFromValue(metrics)
		if metricsErr != nil {
			return metricsErr, nil
		}
		selected = append(selected, conversationRow{relation: relation, owner: owner.String, event: event})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate conversation branch: %w", err), nil
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close conversation branch: %w", err), nil
	}
	result, err := assembleConversationBranch(selected)
	if err != nil {
		return err, nil
	}
	if more && len(result.Entries) > 0 {
		last := result.Entries[len(result.Entries)-1].Event
		if _, err := time.Parse(time.RFC3339Nano, last.CreatedAt); err != nil {
			return fmt.Errorf("parse conversation cursor event %q timestamp: %w", last.Ref.Id, err), nil
		}
		cursor.At, cursor.ID = last.CreatedAt, last.Ref.Id
		encoded, err := json.Marshal(cursor)
		if err != nil {
			return err, nil
		}
		result.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return nil, result
}

func assembleConversationBranch(rows []conversationRow) (*ConversationBranch, error) {
	result := &ConversationBranch{Entries: []ConversationEntry{}, AgentRequests: []ConversationAgentRequest{}, Interactions: []ConversationInteraction{}}
	entries := map[string]int{}
	visible := []model.SessionEvent{}
	for _, row := range rows {
		if row.relation == "branch" {
			entries[row.event.Ref.Id] = len(result.Entries)
			result.Entries = append(result.Entries, ConversationEntry{Event: row.event, Previews: []model.SessionEvent{}})
		}
	}
	for _, row := range rows {
		if row.relation != "preview" && row.relation != "preview-more" {
			continue
		}
		index, ok := entries[row.owner]
		if !ok {
			return nil, fmt.Errorf("conversation preview owner %q is unavailable", row.owner)
		}
		entry := &result.Entries[index]
		if row.relation == "preview-more" {
			entry.HasMoreReplies = true
		} else {
			entry.Previews = append(entry.Previews, row.event)
		}
	}
	for _, entry := range result.Entries {
		visible = append(visible, entry.Event)
		visible = append(visible, entry.Previews...)
	}
	agents := map[string]int{}
	for _, event := range visible {
		if event.Kind != model.SessionEventKindAgentRequest {
			continue
		}
		if _, exists := agents[event.Ref.Id]; exists {
			continue
		}
		agents[event.Ref.Id] = len(result.AgentRequests)
		result.AgentRequests = append(result.AgentRequests, ConversationAgentRequest{Request: event.Ref, State: "working", Interactions: []model.SessionEventRef{}})
	}
	interactions := map[string]int{}
	for _, row := range rows {
		if row.relation != "prompt" {
			continue
		}
		interactions[row.event.Ref.Id] = len(result.Interactions)
		result.Interactions = append(result.Interactions, ConversationInteraction{Request: row.event})
		if row.owner != "" {
			index, ok := agents[row.owner]
			if !ok {
				return nil, fmt.Errorf("conversation interaction owner %q is unavailable", row.owner)
			}
			result.AgentRequests[index].Interactions = append(result.AgentRequests[index].Interactions, row.event.Ref)
		}
	}
	for _, row := range rows {
		switch row.relation {
		case "terminal", "cancellation", "outcome-preview", "outcome-preview-more":
			index, ok := agents[row.owner]
			if !ok {
				return nil, fmt.Errorf("conversation agent request %q is unavailable", row.owner)
			}
			agent := &result.AgentRequests[index]
			switch row.relation {
			case "cancellation":
				agent.State = "cancelling"
				continue
			case "outcome-preview":
				agent.OutcomePreviews = append(agent.OutcomePreviews, row.event)
				continue
			case "outcome-preview-more":
				agent.OutcomeHasMoreReplies = true
				continue
			}
			if agent.Outcome != nil {
				return nil, fmt.Errorf("conversation agent request %q has multiple terminal events", row.owner)
			}
			event := row.event
			agent.Outcome = &event
		case "resolution":
			index, ok := interactions[row.owner]
			if !ok {
				return nil, fmt.Errorf("conversation interaction %q is unavailable", row.owner)
			}
			if result.Interactions[index].Response != nil {
				return nil, fmt.Errorf("conversation interaction %q has multiple responses", row.owner)
			}
			event := row.event
			result.Interactions[index].Response = &event
		}
	}
	for i := range result.AgentRequests {
		agent := &result.AgentRequests[i]
		if agent.Outcome != nil {
			agent.State = "completed"
			if agent.Outcome.Kind == model.SessionEventKindAgentFailure {
				agent.State = "failed"
				if agent.Outcome.Payload["code"] == "cancelled" {
					agent.State = "cancelled"
				}
			}
		} else if agent.State != "cancelling" {
			for _, ref := range agent.Interactions {
				if result.Interactions[interactions[ref.Id]].Response == nil {
					agent.State = "waiting"
					break
				}
			}
		}
	}
	return result, nil
}
