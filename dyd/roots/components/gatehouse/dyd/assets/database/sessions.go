package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/model"
	"gatehouse/typed_id"
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
		INSERT INTO gatehouse_sessions (workspace, id, name, author_principal, author_agent, author_gateway, enabled, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`)
		RETURNING created_at
	`, session.Ref.Workspace.Id, session.Ref.Id, session.Name, authorPrincipal, authorAgent, authorGateway, session.Enabled, session.CreatedAt)
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
	err, _ = store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: session.Ref.Workspace},
		Event:        "session.create",
		ResourceKind: ActivityResourceKindSession,
		Session:      &session.Ref,
		CreatedAt:    session.CreatedAt,
	}, []string{ActivityTopicSessions, ActivityTopicSession(session.Ref)})
	if err != nil {
		return fmt.Errorf("append session creation activity: %w", err), model.Session{}
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
	if !typed_id.Valid(typed_id.Gateway, session.AuthorGateway.Id) {
		return nil, nil, nil, fmt.Errorf("create session: gateway author ID is invalid")
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
	if !typed_id.Valid(typed_id.Gateway, gateway.String) {
		return nil, nil, nil, fmt.Errorf("read session: gateway author ID is invalid")
	}
	return nil, nil, &model.GatewayRef{Id: gateway.String}, nil
}

func (store *Store) SessionEventsCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	err, events := store.sessionEventsCreateBatch(ctx, []model.SessionEvent{event}, false)
	if err != nil {
		return err, model.SessionEvent{}
	}
	return nil, events[0]
}

// SessionEventCreateInTransaction persists an agent event with the DBOS checkpoint transaction.
func (store *Store) SessionEventCreateInTransaction(ctx context.Context, transaction dbos.Tx, event model.SessionEvent) (error, model.SessionEvent) {
	if transaction == nil {
		return fmt.Errorf("create session event: transaction is required"), model.SessionEvent{}
	}
	if strings.TrimSpace(event.Ref.Id) == "" || strings.TrimSpace(event.Kind) == "" {
		return fmt.Errorf("create session event: ID and kind must not be blank"), model.SessionEvent{}
	}
	principal, agent, gateway, err := sessionEventAuthorValues(event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	parent, err := sessionEventParentValue(event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	placeholder := keychainPlaceholder(store.kind)
	if event.Parent != nil {
		var parentExists bool
		if err := transaction.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM gatehouse_session_events
				WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND id = `+placeholder(3)+`
			)
		`, event.Parent.Session.Workspace.Id, event.Parent.Session.Id, event.Parent.Id).Scan(&parentExists); err != nil {
			return fmt.Errorf("check session event parent: %w", err), model.SessionEvent{}
		}
		if !parentExists {
			return fmt.Errorf("create session event: parent must already exist"), model.SessionEvent{}
		}
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode session event payload: %w", err), model.SessionEvent{}
	}
	row := transaction.QueryRow(ctx, `
		INSERT INTO gatehouse_session_events (
			workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`,
			`+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`
		)
		RETURNING created_at
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Ref.Id, parent, event.Kind, principal, agent, gateway, string(payload), event.CreatedAt)
	if err := row.Scan(&event.CreatedAt); err != nil {
		return fmt.Errorf("insert session event: %w", err), model.SessionEvent{}
	}
	session := event.Ref.Session
	err, _ = store.ActivityEventAppendInTransaction(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: session.Workspace},
		Event:        "session_event.create",
		ResourceKind: ActivityResourceKindSessionEvent,
		Session:      &session,
		SessionEvent: &event.Ref,
		CreatedAt:    event.CreatedAt,
	}, []string{ActivityTopicSession(session)})
	if err != nil {
		return fmt.Errorf("append session event activity: %w", err), model.SessionEvent{}
	}
	return nil, event
}

func (store *Store) SessionMessagesCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	err, events := store.sessionEventsCreateBatch(ctx, []model.SessionEvent{event}, true)
	if err != nil {
		return err, model.SessionEvent{}
	}
	return nil, events[0]
}

func sessionMessageFileIDs(event model.SessionEvent) ([]string, error) {
	if event.Kind != "message.text" {
		return nil, fmt.Errorf("create session message: event kind must be message.text")
	}
	filesValue, exists := event.Payload["files"]
	if !exists {
		return nil, nil
	}
	values, ok := filesValue.([]string)
	if !ok {
		return nil, fmt.Errorf("create session message: files must be string IDs")
	}
	return values, nil
}

func (store *Store) SessionEventsCreateBatch(ctx context.Context, events []model.SessionEvent) (error, []model.SessionEvent) {
	return store.sessionEventsCreateBatch(ctx, events, false)
}

type sessionEventInsert struct {
	event                    model.SessionEvent
	parent, principal, agent any
	gateway                  any
	payload                  string
}

func (store *Store) sessionEventsCreateBatch(ctx context.Context, events []model.SessionEvent, createReplyTasks bool) (error, []model.SessionEvent) {
	if len(events) == 0 {
		return nil, []model.SessionEvent{}
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session event creation: %w", err), nil
	}
	defer transaction.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	inserts := make([]sessionEventInsert, 0, len(events))
	seen := make(map[model.SessionEventRef]struct{}, len(events))
	for index, event := range events {
		if strings.TrimSpace(event.Ref.Id) == "" || strings.TrimSpace(event.Kind) == "" {
			return fmt.Errorf("create session events: event %d ID and kind must not be blank", index), nil
		}
		if _, exists := seen[event.Ref]; exists {
			return fmt.Errorf("create session events: event %d has a duplicate ID", index), nil
		}
		principal, agent, gateway, err := sessionEventAuthorValues(event)
		if err != nil {
			return err, nil
		}
		parent, err := sessionEventParentValue(event)
		if err != nil {
			return err, nil
		}
		if event.Parent != nil {
			if _, exists := seen[*event.Parent]; !exists {
				var parentExists bool
				err := transaction.QueryRowContext(ctx, `
					SELECT EXISTS (
						SELECT 1
						FROM gatehouse_session_events
						WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND id = `+placeholder(3)+`
					)
				`, event.Parent.Session.Workspace.Id, event.Parent.Session.Id, event.Parent.Id).Scan(&parentExists)
				if err != nil {
					return fmt.Errorf("check session event parent: %w", err), nil
				}
				if !parentExists {
					return fmt.Errorf("create session events: event %d parent must already exist or appear earlier in the batch", index), nil
				}
			}
		}
		if event.CreatedAt == "" {
			event.CreatedAt = time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
		}
		if createReplyTasks {
			files, err := sessionMessageFileIDs(event)
			if err != nil {
				return err, nil
			}
			err, snapshots := store.SessionFileSnapshots(ctx, transaction, event.Ref.Session, files)
			if err != nil {
				return fmt.Errorf("create session message: %w", err), nil
			}
			if len(snapshots) > 0 {
				event.Payload["files"] = snapshots
			}
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("encode session event payload: %w", err), nil
		}
		inserts = append(inserts, sessionEventInsert{event: event, parent: parent, principal: principal, agent: agent, gateway: gateway, payload: string(payload)})
		seen[event.Ref] = struct{}{}
	}
	for _, insert := range inserts {
		row := transaction.QueryRowContext(ctx, `
			INSERT INTO gatehouse_session_events (
				workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, created_at
			) VALUES (
				`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`,
				`+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`
			)
			RETURNING created_at
		`, insert.event.Ref.Session.Workspace.Id, insert.event.Ref.Session.Id, insert.event.Ref.Id, insert.parent, insert.event.Kind, insert.principal, insert.agent, insert.gateway, insert.payload, insert.event.CreatedAt)
		if err := row.Scan(&insert.event.CreatedAt); err != nil {
			return fmt.Errorf("insert session event: %w", err), nil
		}
		session := insert.event.Ref.Session
		err, _ = store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
			Ref:          model.ActivityEventRef{Workspace: session.Workspace},
			Event:        "session_event.create",
			ResourceKind: ActivityResourceKindSessionEvent,
			Session:      &session,
			SessionEvent: &insert.event.Ref,
			CreatedAt:    insert.event.CreatedAt,
		}, []string{ActivityTopicSession(session)})
		if err != nil {
			return fmt.Errorf("append session event activity: %w", err), nil
		}
		if createReplyTasks {
			_, err := transaction.ExecContext(ctx, `
				INSERT INTO gatehouse_agent_tasks__session_event_reply (workspace, session, event, created_at)
				VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`)
			`, insert.event.Ref.Session.Workspace.Id, insert.event.Ref.Session.Id, insert.event.Ref.Id, insert.event.CreatedAt)
			if err != nil {
				return fmt.Errorf("insert session event reply task: %w", err), nil
			}
			if text, _ := insert.event.Payload["text"].(string); strings.TrimSpace(text) != "" {
				_, err = transaction.ExecContext(ctx, `
					INSERT INTO gatehouse_agent_tasks__session_name (workspace, session, created_at)
					SELECT `+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`
					WHERE EXISTS (
						SELECT 1 FROM gatehouse_sessions
						WHERE workspace = `+placeholder(4)+` AND id = `+placeholder(5)+` AND name IS NULL
					)
					ON CONFLICT (workspace, session) DO NOTHING
				`, insert.event.Ref.Session.Workspace.Id, insert.event.Ref.Session.Id, insert.event.CreatedAt, insert.event.Ref.Session.Workspace.Id, insert.event.Ref.Session.Id)
				if err != nil {
					return fmt.Errorf("insert session name task: %w", err), nil
				}
			}
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session event creation: %w", err), nil
	}
	stored := make([]model.SessionEvent, len(inserts))
	for index, insert := range inserts {
		stored[index] = insert.event
	}
	return nil, stored
}

func (store *Store) SessionEventsGet(ctx context.Context, session model.SessionRef) (error, []model.SessionEvent) {
	return store.SessionEventsPageGet(ctx, session, "", "", 0)
}

func (store *Store) SessionEventsPageGet(ctx context.Context, session model.SessionRef, afterCreatedAt, afterID string, limit int) (error, []model.SessionEvent) {
	if limit < 0 {
		return fmt.Errorf("get session events: limit must not be negative"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `
		SELECT events.id, events.parent, events.kind, events.author_principal, principals.alias, principals.name, principals.enabled, events.author_agent, events.author_gateway, events.payload, events.created_at
		FROM gatehouse_session_events AS events
		LEFT JOIN gatehouse_principals AS principals ON principals.id = events.author_principal
		WHERE events.workspace = ` + placeholder(1) + ` AND events.session = ` + placeholder(2) + `
			AND (events.created_at, events.id) > (` + placeholder(3) + `, ` + placeholder(4) + `)
		ORDER BY events.created_at, events.id
	`
	arguments := []any{session.Workspace.Id, session.Id, afterCreatedAt, afterID}
	if limit > 0 {
		query += " LIMIT " + placeholder(5)
		arguments = append(arguments, limit)
	}
	rows, err := store.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("get session events: %w", err), nil
	}
	defer rows.Close()

	events := []model.SessionEvent{}
	for rows.Next() {
		var id, kind, payload, createdAt string
		var parent, principal, principalAlias, principalName, agent, gateway sql.NullString
		var principalEnabled sql.NullBool
		if err := rows.Scan(
			&id,
			&parent,
			&kind,
			&principal,
			&principalAlias,
			&principalName,
			&principalEnabled,
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
		authorPrincipal, authorAgent, authorGateway, err := sessionEventAuthorsFromValues(session.Workspace, principal, principalAlias, principalName, principalEnabled, agent, gateway)
		if err != nil {
			return err, nil
		}
		event := model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Kind:            kind,
			AuthorPrincipal: authorPrincipal,
			AuthorAgent:     authorAgent,
			AuthorGateway:   authorGateway,
			Payload:         decoded,
			CreatedAt:       createdAt,
		}
		if parent.Valid {
			event.Parent = &model.SessionEventRef{Session: session, Id: parent.String}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session events: %w", err), nil
	}
	return nil, events
}

type SessionEventTreeEntry struct {
	Event model.SessionEvent
	Depth int
}

func (store *Store) SessionEventsTreePageGet(ctx context.Context, session model.SessionRef, afterCreatedAt, afterID string, limit int) (error, []SessionEventTreeEntry) {
	if limit <= 0 {
		return fmt.Errorf("get session event tree: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH RECURSIVE
		roots AS (
			SELECT workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, created_at,
				0 AS depth, created_at || '/' || id AS display_path
			FROM gatehouse_session_events
			WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+`
				AND parent IS NULL
				AND (created_at, id) > (`+placeholder(3)+`, `+placeholder(4)+`)
			ORDER BY created_at, id
			LIMIT `+placeholder(5)+`
		),
		tree AS (
			SELECT workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, created_at, depth, display_path
			FROM roots

			UNION ALL

			SELECT child.workspace, child.session, child.id, child.parent, child.kind, child.author_principal, child.author_agent, child.author_gateway, child.payload, child.created_at,
				tree.depth + 1, tree.display_path || '/' || child.created_at || '/' || child.id
			FROM gatehouse_session_events AS child
			JOIN tree ON child.workspace = tree.workspace AND child.session = tree.session AND child.parent = tree.id
		)
		SELECT tree.id, tree.parent, tree.kind, tree.author_principal, principals.alias, principals.name, principals.enabled, tree.author_agent, tree.author_gateway, tree.payload, tree.created_at, tree.depth
		FROM tree
		LEFT JOIN gatehouse_principals AS principals ON principals.id = tree.author_principal
		ORDER BY display_path
	`, session.Workspace.Id, session.Id, afterCreatedAt, afterID, limit)
	if err != nil {
		return fmt.Errorf("get session event tree: %w", err), nil
	}
	defer rows.Close()

	entries := []SessionEventTreeEntry{}
	for rows.Next() {
		var entry SessionEventTreeEntry
		var id, kind, payload, createdAt string
		var parent, principal, principalAlias, principalName, agent, gateway sql.NullString
		var principalEnabled sql.NullBool
		if err := rows.Scan(
			&id,
			&parent,
			&kind,
			&principal,
			&principalAlias,
			&principalName,
			&principalEnabled,
			&agent,
			&gateway,
			&payload,
			&createdAt,
			&entry.Depth,
		); err != nil {
			return fmt.Errorf("scan session event tree entry: %w", err), nil
		}
		decoded := map[string]interface{}{}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			return fmt.Errorf("decode session event payload: %w", err), nil
		}
		authorPrincipal, authorAgent, authorGateway, err := sessionEventAuthorsFromValues(session.Workspace, principal, principalAlias, principalName, principalEnabled, agent, gateway)
		if err != nil {
			return err, nil
		}
		entry.Event = model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Kind:            kind,
			AuthorPrincipal: authorPrincipal,
			AuthorAgent:     authorAgent,
			AuthorGateway:   authorGateway,
			Payload:         decoded,
			CreatedAt:       createdAt,
		}
		if parent.Valid {
			entry.Event.Parent = &model.SessionEventRef{Session: session, Id: parent.String}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session event tree: %w", err), nil
	}
	return nil, entries
}

func (store *Store) SessionEventGet(ctx context.Context, event model.SessionEventRef) (error, *model.SessionEvent) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT events.parent, events.kind, events.author_principal, principals.alias, principals.name, principals.enabled, events.author_agent, events.author_gateway, events.payload, events.created_at
		FROM gatehouse_session_events AS events
		LEFT JOIN gatehouse_principals AS principals ON principals.id = events.author_principal
		WHERE events.workspace = `+placeholder(1)+` AND events.session = `+placeholder(2)+` AND events.id = `+placeholder(3)+`
	`, event.Session.Workspace.Id, event.Session.Id, event.Id)
	var stored model.SessionEvent
	stored.Ref = event
	var payload, createdAt string
	var parent, principal, principalAlias, principalName, agent, gateway sql.NullString
	var principalEnabled sql.NullBool
	if err := row.Scan(&parent, &stored.Kind, &principal, &principalAlias, &principalName, &principalEnabled, &agent, &gateway, &payload, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session event: %w", err), nil
	}
	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return fmt.Errorf("decode session event payload: %w", err), nil
	}
	authorPrincipal, authorAgent, authorGateway, err := sessionEventAuthorsFromValues(event.Session.Workspace, principal, principalAlias, principalName, principalEnabled, agent, gateway)
	if err != nil {
		return err, nil
	}
	stored.AuthorPrincipal = authorPrincipal
	stored.AuthorAgent = authorAgent
	stored.AuthorGateway = authorGateway
	stored.Payload = decoded
	stored.CreatedAt = createdAt
	if parent.Valid {
		stored.Parent = &model.SessionEventRef{Session: event.Session, Id: parent.String}
	}
	return nil, &stored
}

func (store *Store) SessionEventChildGet(ctx context.Context, parent model.SessionEventRef, kind string) (error, *model.SessionEvent) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT id
		FROM gatehouse_session_events
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND parent = `+placeholder(3)+` AND kind = `+placeholder(4)+`
		ORDER BY created_at, id
		LIMIT 1
	`, parent.Session.Workspace.Id, parent.Session.Id, parent.Id, kind)
	var childID string
	if err := row.Scan(&childID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session event child: %w", err), nil
	}
	return store.SessionEventGet(ctx, model.SessionEventRef{Session: parent.Session, Id: childID})
}

type SessionEventReplyTask struct {
	Event model.SessionEventRef
}

func (store *Store) SessionEventReplyTasksGet(ctx context.Context, limit int) (error, []SessionEventReplyTask) {
	if limit <= 0 {
		return fmt.Errorf("get session event reply tasks: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT workspace, session, event
		FROM gatehouse_agent_tasks__session_event_reply
		ORDER BY created_at, event
		LIMIT `+placeholder(1)+`
	`, limit)
	if err != nil {
		return fmt.Errorf("get session event reply tasks: %w", err), nil
	}
	defer rows.Close()

	tasks := []SessionEventReplyTask{}
	for rows.Next() {
		var task SessionEventReplyTask
		if err := rows.Scan(&task.Event.Session.Workspace.Id, &task.Event.Session.Id, &task.Event.Id); err != nil {
			return fmt.Errorf("scan session event reply task: %w", err), nil
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session event reply tasks: %w", err), nil
	}
	return nil, tasks
}

func (store *Store) SessionEventReplyTaskDelete(ctx context.Context, event model.SessionEventRef) error {
	placeholder := keychainPlaceholder(store.kind)
	_, err := store.ExecContext(ctx, `
		DELETE FROM gatehouse_agent_tasks__session_event_reply
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND event = `+placeholder(3)+`
	`, event.Session.Workspace.Id, event.Session.Id, event.Id)
	if err != nil {
		return fmt.Errorf("delete session event reply task: %w", err)
	}
	return nil
}

type SessionNameTask struct {
	Session model.SessionRef
}

func (store *Store) SessionNameTasksGet(ctx context.Context, limit int) (error, []SessionNameTask) {
	if limit <= 0 {
		return fmt.Errorf("get session name tasks: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT workspace, session
		FROM gatehouse_agent_tasks__session_name
		ORDER BY created_at, session
		LIMIT `+placeholder(1)+`
	`, limit)
	if err != nil {
		return fmt.Errorf("get session name tasks: %w", err), nil
	}
	defer rows.Close()

	tasks := []SessionNameTask{}
	for rows.Next() {
		var task SessionNameTask
		if err := rows.Scan(&task.Session.Workspace.Id, &task.Session.Id); err != nil {
			return fmt.Errorf("scan session name task: %w", err), nil
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session name tasks: %w", err), nil
	}
	return nil, tasks
}

func (store *Store) SessionNameTaskDelete(ctx context.Context, session model.SessionRef) error {
	placeholder := keychainPlaceholder(store.kind)
	_, err := store.ExecContext(ctx, `
		DELETE FROM gatehouse_agent_tasks__session_name
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+`
	`, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("delete session name task: %w", err)
	}
	return nil
}

func (store *Store) SessionNameGet(ctx context.Context, session model.SessionRef) (error, *string) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT name FROM gatehouse_sessions
		WHERE workspace = `+placeholder(1)+` AND id = `+placeholder(2)+`
	`, session.Workspace.Id, session.Id)
	var name sql.NullString
	if err := row.Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session name: %w", err), nil
	}
	if !name.Valid {
		return nil, nil
	}
	return nil, &name.String
}

func (store *Store) SessionNameSet(ctx context.Context, session model.SessionRef, name string) (error, bool) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return fmt.Errorf("set session name: name must not be blank"), false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session name update: %w", err), false
	}
	defer transaction.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	row := transaction.QueryRowContext(ctx, `
		UPDATE gatehouse_sessions
		SET name = `+placeholder(1)+`
		WHERE workspace = `+placeholder(2)+` AND id = `+placeholder(3)+` AND name IS NULL
		RETURNING id
	`, name, session.Workspace.Id, session.Id)
	var updated string
	if err := row.Scan(&updated); err != nil {
		if err == sql.ErrNoRows {
			if err := transaction.Commit(); err != nil {
				return fmt.Errorf("commit unchanged session name: %w", err), false
			}
			return nil, false
		}
		return fmt.Errorf("update session name: %w", err), false
	}
	err, _ = store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: session.Workspace},
		Event:        "session.update",
		ResourceKind: ActivityResourceKindSession,
		Session:      &session,
	}, []string{ActivityTopicSessions, ActivityTopicSession(session)})
	if err != nil {
		return fmt.Errorf("append session name activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session name update: %w", err), false
	}
	return nil, true
}

type WorkspaceAgentModel struct {
	Ref        model.WorkspaceAgentRef
	ProviderID string
	Protocol   string
	BaseURL    *string
	Keychain   *model.KeychainRef
	APIKey     *string
	Model      string
	Parameters string
	MaxTurns   int
	SystemPrompt *string
}

type WorkspaceAgent struct {
	ID    string
	Label *string
}

func (store *Store) WorkspaceAgentsGet(ctx context.Context, workspace model.WorkspaceRef) (error, []WorkspaceAgent) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT bindings.model_id, bindings.label
		FROM gatehouse_workspace_agents AS bindings
		JOIN gatehouse_agent_models AS models ON models.id = bindings.model_id
		JOIN gatehouse_agent_providers AS providers ON providers.id = models.provider_id
		WHERE bindings.workspace_id = `+placeholder(1)+`
			AND bindings.enabled = TRUE
			AND models.enabled = TRUE
			AND providers.enabled = TRUE
		ORDER BY bindings.priority DESC, bindings.model_id
	`, workspace.Id)
	if err != nil {
		return fmt.Errorf("get workspace agents: %w", err), nil
	}
	defer rows.Close()
	agents := []WorkspaceAgent{}
	for rows.Next() {
		var agent WorkspaceAgent
		var label sql.NullString
		if err := rows.Scan(&agent.ID, &label); err != nil {
			return fmt.Errorf("scan workspace agent: %w", err), nil
		}
		if label.Valid {
			agent.Label = &label.String
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace agents: %w", err), nil
	}
	return nil, agents
}

func (store *Store) WorkspaceAgentModelSelect(ctx context.Context, workspace model.WorkspaceRef, preferred string) (error, *WorkspaceAgentModel) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT models.id, providers.id, providers.protocol, providers.base_url, providers.keychain_id, providers.keychain_version, providers.api_key, models.model, models.parameters, bindings.max_turns, bindings.system_prompt
		FROM gatehouse_workspace_agents AS bindings
		JOIN gatehouse_agent_models AS models ON models.id = bindings.model_id
		JOIN gatehouse_agent_providers AS providers ON providers.id = models.provider_id
		WHERE bindings.workspace_id = `+placeholder(1)+`
			AND bindings.enabled = TRUE
			AND models.enabled = TRUE
			AND providers.enabled = TRUE
		ORDER BY CASE WHEN `+placeholder(2)+` <> '' AND bindings.model_id = `+placeholder(3)+` THEN 0 ELSE 1 END, bindings.priority DESC, RANDOM()
		LIMIT 1
	`, workspace.Id, preferred, preferred)
	var selected WorkspaceAgentModel
	selected.Ref.Workspace = workspace
	var baseURL, keychainID, apiKey, systemPrompt sql.NullString
	var keychainVersion sql.NullInt64
	if err := row.Scan(&selected.Ref.Model.Id, &selected.ProviderID, &selected.Protocol, &baseURL, &keychainID, &keychainVersion, &apiKey, &selected.Model, &selected.Parameters, &selected.MaxTurns, &systemPrompt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("select workspace agent model: %w", err), nil
	}
	if baseURL.Valid || keychainID.Valid || keychainVersion.Valid || apiKey.Valid {
		if !baseURL.Valid || !keychainID.Valid || !keychainVersion.Valid || keychainVersion.Int64 <= 0 || !apiKey.Valid {
			return fmt.Errorf("select workspace agent model: invalid provider credentials"), nil
		}
		selected.BaseURL = &baseURL.String
		selected.Keychain = &model.KeychainRef{Id: keychainID.String, Version: int(keychainVersion.Int64)}
		selected.APIKey = &apiKey.String
	}
	if systemPrompt.Valid {
		selected.SystemPrompt = &systemPrompt.String
	}
	return nil, &selected
}

func sessionEventParentValue(event model.SessionEvent) (any, error) {
	if event.Parent == nil {
		return nil, nil
	}
	if event.Parent.Session != event.Ref.Session {
		return nil, fmt.Errorf("create session event: parent belongs to another session")
	}
	if strings.TrimSpace(event.Parent.Id) == "" || event.Parent.Id == event.Ref.Id {
		return nil, fmt.Errorf("create session event: parent is invalid")
	}
	return event.Parent.Id, nil
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
		principal = event.AuthorPrincipal.Ref.Id
	}
	if event.AuthorAgent != nil {
		if event.AuthorAgent.Workspace != event.Ref.Session.Workspace {
			return nil, nil, nil, fmt.Errorf("create session event: workspace agent author belongs to another workspace")
		}
		agent = event.AuthorAgent.Model.Id
	}
	if event.AuthorGateway != nil {
		if !typed_id.Valid(typed_id.Gateway, event.AuthorGateway.Id) {
			return nil, nil, nil, fmt.Errorf("create session event: gateway author ID is invalid")
		}
		gateway = event.AuthorGateway.Id
	}
	return principal, agent, gateway, nil
}

func sessionEventAuthorsFromValues(workspace model.WorkspaceRef, principalID, principalAlias, principalName sql.NullString, principalEnabled sql.NullBool, agent, gateway sql.NullString) (*model.Principal, *model.WorkspaceAgentRef, *model.GatewayRef, error) {
	authors := 0
	for _, author := range []sql.NullString{principalID, agent, gateway} {
		if author.Valid {
			authors++
		}
	}
	if authors != 1 {
		return nil, nil, nil, fmt.Errorf("read session event: expected exactly one author")
	}
	if principalID.Valid {
		if !principalEnabled.Valid {
			return nil, nil, nil, fmt.Errorf("read session event: principal author was not found")
		}
		principal := model.Principal{Ref: model.PrincipalRef{Id: principalID.String}, Enabled: principalEnabled.Bool}
		if principalAlias.Valid {
			principal.Alias = &principalAlias.String
		}
		if principalName.Valid {
			principal.Name = &principalName.String
		}
		return &principal, nil, nil, nil
	}
	if agent.Valid {
		return nil, &model.WorkspaceAgentRef{Workspace: workspace, Model: model.AgentModelRef{Id: agent.String}}, nil, nil
	}
	if !typed_id.Valid(typed_id.Gateway, gateway.String) {
		return nil, nil, nil, fmt.Errorf("read session event: gateway author ID is invalid")
	}
	return nil, nil, &model.GatewayRef{Id: gateway.String}, nil
}

func (store *Store) SessionsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Session) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT
			sessions.workspace,
			sessions.id,
			sessions.name,
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
		var name, authorPrincipal, authorAgent, authorGateway sql.NullString
		if err := rows.Scan(
			&session.Ref.Workspace.Id,
			&session.Ref.Id,
			&name,
			&authorPrincipal,
			&authorAgent,
			&authorGateway,
			&session.Enabled,
			&session.CreatedAt,
		); err != nil {
			return fmt.Errorf("scan session: %w", err), nil
		}
		if name.Valid {
			session.Name = &name.String
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
