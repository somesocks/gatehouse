package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gatehouse/model"
)

func (store *Store) SessionsCreate(ctx context.Context, session model.Session, grantee model.PrincipalRef) (error, model.Session) {
	if strings.TrimSpace(session.Ref.Id) == "" {
		return fmt.Errorf("create session: session ID must not be blank"), model.Session{}
	}
	authorPrincipal, authorAgent, authorGateway, err := sessionAuthorValues(session)
	if err != nil {
		return err, model.Session{}
	}
	session.CreatedAt = time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")

	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session creation: %w", err), model.Session{}
	}
	defer transaction.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	row := transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, id, author_principal, author_agent, author_gateway, enabled, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`)
		RETURNING created_at
	`, session.Ref.Workspace.Id, session.Ref.Id, authorPrincipal, authorAgent, authorGateway, session.Enabled, session.CreatedAt)
	if err := row.Scan(&session.CreatedAt); err != nil {
		return fmt.Errorf("insert session: %w", err), model.Session{}
	}
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_principal_grants (workspace, session, principal, enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, TRUE)
	`, session.Ref.Workspace.Id, session.Ref.Id, grantee.Id)
	if err != nil {
		return fmt.Errorf("grant session principal: %w", err), model.Session{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session creation: %w", err), model.Session{}
	}
	return nil, session
}

func (store *Store) SessionGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, *model.Session) {
	err, sessions := store.SessionsGet(ctx, session.Workspace, principal)
	if err != nil {
		return err, nil
	}
	for _, candidate := range sessions {
		if candidate.Ref == session {
			return nil, &candidate
		}
	}
	return nil, nil
}

func sessionAuthorValues(session model.Session) (any, any, any, error) {
	authors := 0
	if session.AuthorPrincipal != nil {
		authors++
	}
	if session.AuthorAgent != nil {
		authors++
	}
	if session.AuthorGateway != nil {
		authors++
	}
	if authors != 1 {
		return nil, nil, nil, fmt.Errorf("create session: exactly one author is required")
	}
	if session.AuthorPrincipal != nil {
		return session.AuthorPrincipal.Id, nil, nil, nil
	}
	if session.AuthorAgent != nil {
		if session.AuthorAgent.Workspace != session.Ref.Workspace {
			return nil, nil, nil, fmt.Errorf("create session: workspace agent author belongs to another workspace")
		}
		return nil, session.AuthorAgent.Model.Id, nil, nil
	}
	return nil, nil, session.AuthorGateway.Id, nil
}

func sessionAuthorsFromValues(workspace model.WorkspaceRef, principal, agent, gateway sql.NullString) (*model.PrincipalRef, *model.WorkspaceAgentRef, *model.GatewayRef, error) {
	authors := 0
	for _, author := range []sql.NullString{principal, agent, gateway} {
		if author.Valid {
			authors++
		}
	}
	if authors != 1 {
		return nil, nil, nil, fmt.Errorf("read session: expected exactly one author")
	}
	if principal.Valid {
		return &model.PrincipalRef{Id: principal.String}, nil, nil, nil
	}
	if agent.Valid {
		return nil, &model.WorkspaceAgentRef{Workspace: workspace, Model: model.AgentModelRef{Id: agent.String}}, nil, nil
	}
	return nil, nil, &model.GatewayRef{Id: gateway.String}, nil
}

func (store *Store) SessionEventsCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	if strings.TrimSpace(event.Ref.Id) == "" || strings.TrimSpace(event.Kind) == "" {
		return fmt.Errorf("create session event: event ID and kind must not be blank"), model.SessionEvent{}
	}
	principal, agent, gateway, err := sessionEventAuthorValues(event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	event.CreatedAt = time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode session event payload: %w", err), model.SessionEvent{}
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		INSERT INTO gatehouse_session_events (
			workspace, session, id, kind, author_principal, author_agent, author_gateway, payload, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`,
			`+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`
		)
		RETURNING created_at
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Ref.Id, event.Kind, principal, agent, gateway, string(payload), event.CreatedAt)
	if err := row.Scan(&event.CreatedAt); err != nil {
		return fmt.Errorf("insert session event: %w", err), model.SessionEvent{}
	}
	return nil, event
}

func (store *Store) SessionEventsGet(ctx context.Context, session model.SessionRef) (error, []model.SessionEvent) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT id, kind, author_principal, author_agent, author_gateway, payload, created_at
		FROM gatehouse_session_events
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+`
		ORDER BY created_at, id
	`, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("get session events: %w", err), nil
	}
	defer rows.Close()

	events := []model.SessionEvent{}
	for rows.Next() {
		var id, kind, payload, createdAt string
		var principal, agent, gateway sql.NullString
		if err := rows.Scan(
			&id,
			&kind,
			&principal,
			&agent,
			&gateway,
			&payload,
			&createdAt,
		); err != nil {
			return fmt.Errorf("scan session event: %w", err), nil
		}
		decoded := map[string]interface{}{}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			return fmt.Errorf("decode session event payload: %w", err), nil
		}
		authorPrincipal, authorAgent, authorGateway, err := sessionEventAuthorsFromValues(session.Workspace, principal, agent, gateway)
		if err != nil {
			return err, nil
		}
		events = append(events, model.SessionEvent{
			Ref:       model.SessionEventRef{Session: session, Id: id},
			Kind:      kind,
			AuthorPrincipal: authorPrincipal,
			AuthorAgent:     authorAgent,
			AuthorGateway:   authorGateway,
			Payload:         decoded,
			CreatedAt:       createdAt,
		})
		}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session events: %w", err), nil
	}
	return nil, events
}

func sessionEventAuthorValues(event model.SessionEvent) (any, any, any, error) {
	var principal, agent, gateway any
	authors := 0
	if event.AuthorPrincipal != nil {
		authors++
	}
	if event.AuthorAgent != nil {
		authors++
	}
	if event.AuthorGateway != nil {
		authors++
	}
	if authors != 1 {
		return nil, nil, nil, fmt.Errorf("create session event: exactly one author is required")
	}
	if event.AuthorPrincipal != nil {
		principal = event.AuthorPrincipal.Id
	}
	if event.AuthorAgent != nil {
		if event.AuthorAgent.Workspace != event.Ref.Session.Workspace {
			return nil, nil, nil, fmt.Errorf("create session event: workspace agent author belongs to another workspace")
		}
		agent = event.AuthorAgent.Model.Id
	}
	if event.AuthorGateway != nil {
		gateway = event.AuthorGateway.Id
	}
	return principal, agent, gateway, nil
}

func sessionEventAuthorsFromValues(workspace model.WorkspaceRef, principal, agent, gateway sql.NullString) (*model.PrincipalRef, *model.WorkspaceAgentRef, *model.GatewayRef, error) {
	authors := 0
	for _, author := range []sql.NullString{principal, agent, gateway} {
		if author.Valid {
			authors++
		}
	}
	if authors != 1 {
		return nil, nil, nil, fmt.Errorf("read session event: expected exactly one author")
	}
	if principal.Valid {
		return &model.PrincipalRef{Id: principal.String}, nil, nil, nil
	}
	if agent.Valid {
		return nil, &model.WorkspaceAgentRef{Workspace: workspace, Model: model.AgentModelRef{Id: agent.String}}, nil, nil
	}
	return nil, nil, &model.GatewayRef{Id: gateway.String}, nil
}

func (store *Store) SessionsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Session) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT
			sessions.workspace,
			sessions.id,
			sessions.author_principal,
			sessions.author_agent,
			sessions.author_gateway,
			sessions.enabled,
			sessions.created_at
		FROM gatehouse_sessions AS sessions
		WHERE sessions.workspace = `+placeholder(1)+`
			AND sessions.enabled = TRUE
			AND EXISTS (
				SELECT 1
				FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = sessions.workspace AND workspaces.enabled = TRUE
			)
			AND EXISTS (
				SELECT 1
				FROM gatehouse_principals AS principals
				WHERE principals.id = `+placeholder(2)+` AND principals.enabled = TRUE
			)
			AND (
				EXISTS (
					SELECT 1
				FROM gatehouse_session_principal_grants AS grants
					WHERE grants.workspace = sessions.workspace
						AND grants.session = sessions.id
						AND grants.principal = `+placeholder(3)+`
						AND grants.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1
					FROM gatehouse_session_group_grants AS grants
					JOIN gatehouse_groups AS groups
						ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members
						ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					WHERE grants.workspace = sessions.workspace
						AND grants.session = sessions.id
						AND grants.enabled = TRUE
						AND groups.enabled = TRUE
						AND members.principal_id = `+placeholder(4)+`
						AND members.enabled = TRUE
				)
			)
		ORDER BY sessions.created_at DESC, sessions.id
	`, workspace.Id, principal.Id, principal.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get sessions: %w", err), nil
	}
	defer rows.Close()

	sessions := []model.Session{}
	for rows.Next() {
		var session model.Session
		var authorPrincipal, authorAgent, authorGateway sql.NullString
		if err := rows.Scan(
			&session.Ref.Workspace.Id,
			&session.Ref.Id,
			&authorPrincipal,
			&authorAgent,
			&authorGateway,
			&session.Enabled,
			&session.CreatedAt,
		); err != nil {
			return fmt.Errorf("scan session: %w", err), nil
		}
		authorPrincipalRef, authorAgentRef, authorGatewayRef, err := sessionAuthorsFromValues(session.Ref.Workspace, authorPrincipal, authorAgent, authorGateway)
		if err != nil {
			return err, nil
		}
		session.AuthorPrincipal = authorPrincipalRef
		session.AuthorAgent = authorAgentRef
		session.AuthorGateway = authorGatewayRef
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sessions: %w", err), nil
	}
	return nil, sessions
}
