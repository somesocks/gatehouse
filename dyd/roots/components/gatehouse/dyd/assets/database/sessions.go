package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/authz"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrSessionApprovalResolved = errors.New("session approval is already resolved")

func (store *Store) SessionsCreate(ctx context.Context, session model.Session, grantee model.PrincipalRef) (error, model.Session) {
	if strings.TrimSpace(session.Ref.Id) == "" {
		return fmt.Errorf("create session: session ID must not be blank"), model.Session{}
	}
	authorPrincipal, authorAgent, authorGateway, err := sessionAuthorValues(session)
	if err != nil {
		return err, model.Session{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.Session, session.Ref.Id)
	if err != nil {
		return fmt.Errorf("create session: ID is invalid"), model.Session{}
	}
	session.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	if session.Project != nil {
		if session.Project.Workspace != session.Ref.Workspace {
			return fmt.Errorf("create session: project belongs to another workspace"), model.Session{}
		}
		err, project := store.ProjectGet(ctx, *session.Project, grantee)
		if err != nil {
			return err, model.Session{}
		}
		if project == nil {
			return fmt.Errorf("create session: project is unavailable"), model.Session{}
		}
	}

	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session creation: %w", err), model.Session{}
	}
	defer transaction.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	var project any
	if session.Project != nil {
		project = session.Project.Id
	}
	row := transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_sessions (workspace, project, id, name, author_principal, author_agent, author_gateway, enabled, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`)
		RETURNING created_at
	`, session.Ref.Workspace.Id, project, session.Ref.Id, session.Name, authorPrincipal, authorAgent, authorGateway, session.Enabled, session.CreatedAt)
	if err := row.Scan(&session.CreatedAt); err != nil {
		return fmt.Errorf("insert session: %w", err), model.Session{}
	}
	grantID, err := typed_id.New(typed_id.SessionGrant)
	if err != nil {
		return fmt.Errorf("generate session grant ID: %w", err), model.Session{}
	}
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, NULL, TRUE)
	`, grantID, session.Ref.Workspace.Id, session.Ref.Id, authz.Manager, grantee.Id)
	if err != nil {
		return fmt.Errorf("bind session principal role: %w", err), model.Session{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:                  model.ActivityEventRef{},
		Event:                "session_grant.create",
		ResourceKind:         ActivityResourceKindSessionGrant,
		ResourceSessionGrant: &grantID,
	}, []string{ActivityTopicSessionGrant(session.Ref, grantID), ActivityTopicPrincipalGrant(grantee, grantID)}); err != nil {
		return fmt.Errorf("append session principal grant activity: %w", err), model.Session{}
	}
	topics := []string{ActivityTopicSession(session.Ref)}
	if session.Project != nil {
		topics = append(topics, ActivityTopicProject(*session.Project))
	}
	err, _ = store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:             model.ActivityEventRef{},
		Event:           "session.create",
		ResourceKind:    ActivityResourceKindSession,
		ResourceSession: &session.Ref.Id,
		CreatedAt:       session.CreatedAt,
	}, topics)
	if err != nil {
		return fmt.Errorf("append session creation activity: %w", err), model.Session{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session creation: %w", err), model.Session{}
	}
	return nil, session
}

func (store *Store) SessionRolesGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, []authz.Role) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH input AS (
			SELECT `+placeholder(1)+` AS workspace, `+placeholder(2)+` AS session, `+placeholder(3)+` AS principal
		)
		SELECT role FROM (
			SELECT grants.role
			FROM gatehouse_session_grants AS grants
			CROSS JOIN input
			JOIN gatehouse_sessions AS sessions
				ON sessions.workspace = grants.workspace AND sessions.id = grants.session
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = sessions.workspace
			JOIN gatehouse_principals AS principals ON principals.id = grants.principal
			WHERE grants.workspace = input.workspace
				AND grants.session = input.session
				AND grants.principal = input.principal
				AND workspaces.enabled = TRUE
				AND sessions.enabled = TRUE
				AND grants.enabled = TRUE
				AND principals.enabled = TRUE
			UNION
			SELECT grants.role
			FROM gatehouse_session_grants AS grants
			CROSS JOIN input
			JOIN gatehouse_sessions AS sessions
				ON sessions.workspace = grants.workspace AND sessions.id = grants.session
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = sessions.workspace
			JOIN gatehouse_groups AS groups
				ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
			JOIN gatehouse_group_members AS members
				ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
			JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
			WHERE grants.workspace = input.workspace
				AND grants.session = input.session
				AND members.principal_id = input.principal
				AND workspaces.enabled = TRUE
				AND sessions.enabled = TRUE
				AND grants.enabled = TRUE
				AND groups.enabled = TRUE
				AND members.enabled = TRUE
				AND principals.enabled = TRUE
		) AS roles
		ORDER BY role
	`, session.Workspace.Id, session.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get session roles: %w", err), nil
	}
	defer rows.Close()

	roles := []authz.Role{}
	for rows.Next() {
		var role authz.Role
		if err := rows.Scan(&role); err != nil {
			return fmt.Errorf("scan session role: %w", err), nil
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session roles: %w", err), nil
	}
	return nil, roles
}

func (store *Store) SessionProjectSet(ctx context.Context, session model.SessionRef, project *model.ProjectRef, principal model.PrincipalRef) (error, *model.Session) {
	err, stored := store.SessionGet(ctx, session, principal)
	if err != nil {
		return err, nil
	}
	if stored == nil {
		return nil, nil
	}
	if project != nil {
		if project.Workspace != session.Workspace {
			return fmt.Errorf("set session project: project belongs to another workspace"), nil
		}
		err, available := store.ProjectGet(ctx, *project, principal)
		if err != nil {
			return err, nil
		}
		if available == nil {
			return fmt.Errorf("set session project: project is unavailable"), nil
		}
		if err := store.sessionProjectGrantCompatible(ctx, session, *project); err != nil {
			return err, nil
		}
	}
	if stored.Project == project || stored.Project != nil && project != nil && *stored.Project == *project {
		return nil, stored
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session project update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var value any
	if project != nil {
		value = project.Id
	}
	if _, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_sessions SET project = `+placeholder(1)+`
		WHERE workspace = `+placeholder(2)+` AND id = `+placeholder(3)+`
	`, value, session.Workspace.Id, session.Id); err != nil {
		return fmt.Errorf("update session project: %w", err), nil
	}
	topics := []string{ActivityTopicSession(session)}
	if stored.Project != nil {
		topics = append(topics, ActivityTopicProject(*stored.Project))
	}
	if project != nil {
		topics = append(topics, ActivityTopicProject(*project))
	}
	event := "session.project.unlink"
	if stored.Project == nil && project != nil {
		event = "session.project.link"
	} else if stored.Project != nil && project != nil {
		event = "session.project.move"
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:             model.ActivityEventRef{},
		Event:           event,
		ResourceKind:    ActivityResourceKindSession,
		ResourceSession: &session.Id,
	}, topics); err != nil {
		return fmt.Errorf("append session project activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session project update: %w", err), nil
	}
	stored.Project = project
	return nil, stored
}

func (store *Store) sessionProjectGrantCompatible(ctx context.Context, session model.SessionRef, project model.ProjectRef) error {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT grants.principal
		FROM gatehouse_session_grants AS grants
		JOIN gatehouse_principals AS principals ON principals.id = grants.principal
		WHERE grants.workspace = `+placeholder(1)+` AND grants.session = `+placeholder(2)+`
			AND grants.principal IS NOT NULL AND grants.enabled = TRUE AND principals.enabled = TRUE

		UNION

		SELECT members.principal_id
		FROM gatehouse_session_grants AS grants
		JOIN gatehouse_groups AS groups
			ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
		JOIN gatehouse_group_members AS members
			ON members.workspace_id = grants.workspace AND members.group_id = grants."group"
		JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
		WHERE grants.workspace = `+placeholder(3)+` AND grants.session = `+placeholder(4)+`
			AND grants.enabled = TRUE AND groups.enabled = TRUE AND members.enabled = TRUE AND principals.enabled = TRUE
	`, session.Workspace.Id, session.Id, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("get session grantees: %w", err)
	}
	grantees := []model.PrincipalRef{}
	for rows.Next() {
		var principal model.PrincipalRef
		if err := rows.Scan(&principal.Id); err != nil {
			rows.Close()
			return fmt.Errorf("scan session grantee: %w", err)
		}
		grantees = append(grantees, principal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate session grantees: %w", err)
	}
	rows.Close()
	for _, grantee := range grantees {
		err, available := store.ProjectGet(ctx, project, grantee)
		if err != nil {
			return err
		}
		if available == nil {
			return fmt.Errorf("set session project: destination does not grant %q", grantee.Id)
		}
	}
	return nil
}

func (store *Store) SessionGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, *model.Session) {
	err, sessions, _ := store.sessionsSearch(ctx, session.Workspace, principal, SessionSearch{Limit: 1}, session.Id)
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

func (store *Store) SessionProjectGet(ctx context.Context, session model.SessionRef) (error, *model.ProjectRef) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT project FROM gatehouse_sessions
		WHERE workspace = `+placeholder(1)+` AND id = `+placeholder(2)+`
	`, session.Workspace.Id, session.Id)
	var project sql.NullString
	if err := row.Scan(&project); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session project: %w", err), nil
	}
	if !project.Valid {
		return nil, nil
	}
	return nil, &model.ProjectRef{Workspace: session.Workspace, Id: project.String}
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
		return nil, session.AuthorAgent.Id, nil, nil
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
		return nil, &model.WorkspaceAgentRef{Workspace: workspace, Id: agent.String}, nil, nil
	}
	if !typed_id.Valid(typed_id.Gateway, gateway.String) {
		return nil, nil, nil, fmt.Errorf("read session: gateway author ID is invalid")
	}
	return nil, nil, &model.GatewayRef{Id: gateway.String}, nil
}

func (store *Store) SessionEventsCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	err, events := store.sessionEventsCreateBatch(ctx, []model.SessionEvent{event}, false, false)
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
	createdAt, err := typed_id.Timestamp(typed_id.SessionEvent, event.Ref.Id)
	if err != nil {
		return fmt.Errorf("create session event: ID is invalid"), model.SessionEvent{}
	}
	event.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode session event payload: %w", err), model.SessionEvent{}
	}
	metrics, err := sessionEventMetricsValue(event.Metrics)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO gatehouse_session_events (
			workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, metrics, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`,
			`+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`
		)
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Ref.Id, parent, event.Kind, principal, agent, gateway, string(payload), metrics, event.CreatedAt); err != nil {
		return fmt.Errorf("insert session event: %w", err), model.SessionEvent{}
	}
	topics := []string{ActivityTopicSessionEvent(event.Ref)}
	err, _ = store.ActivityEventAppendInTransaction(ctx, transaction, model.ActivityEvent{
		Ref:                  model.ActivityEventRef{},
		Event:                "session_event.create",
		ResourceKind:         ActivityResourceKindSessionEvent,
		ResourceSessionEvent: &event.Ref.Id,
		CreatedAt:            event.CreatedAt,
	}, topics)
	if err != nil {
		return fmt.Errorf("append session event activity: %w", err), model.SessionEvent{}
	}
	return nil, event
}

func (store *Store) SessionMessagesCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	err, events := store.sessionEventsCreateBatch(ctx, []model.SessionEvent{event}, true, false)
	if err != nil {
		return err, model.SessionEvent{}
	}
	return nil, events[0]
}

func sessionMessageAttachmentIDs(event model.SessionEvent) ([]string, error) {
	if event.Kind != "message.text" {
		return nil, fmt.Errorf("create session message: event kind must be message.text")
	}
	attachmentsValue, exists := event.Payload["attachments"]
	if !exists {
		return nil, nil
	}
	values, ok := attachmentsValue.([]string)
	if !ok {
		return nil, fmt.Errorf("create session message: attachments must be string IDs")
	}
	return values, nil
}

// SessionEventAttachmentsHydrate replaces attachment IDs with current session-file references.
func (store *Store) SessionEventAttachmentsHydrate(ctx context.Context, session model.SessionRef, events []model.SessionEvent) (error, []model.SessionEvent) {
	ids := []string{}
	seen := map[string]struct{}{}
	for _, event := range events {
		attachments, linked, err := sessionEventAttachmentIDs(event)
		if err != nil {
			return err, nil
		}
		if !linked {
			continue
		}
		for _, id := range attachments {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	err, references := store.SessionFileReferencesGet(ctx, session, ids)
	if err != nil {
		return err, nil
	}
	byID := make(map[string]SessionFileSummary, len(references))
	for _, reference := range references {
		byID[reference.ID] = reference
	}
	for index, event := range events {
		attachments, linked, err := sessionEventAttachmentIDs(event)
		if err != nil {
			return err, nil
		}
		if !linked {
			continue
		}
		references := make([]interface{}, 0, len(attachments))
		for _, id := range attachments {
			reference, exists := byID[id]
			if !exists {
				return fmt.Errorf("get session attachment %q: unavailable", id), nil
			}
			value := map[string]interface{}{"id": reference.ID, "name": reference.Name, "size": reference.Size, "fingerprint": reference.Fingerprint}
			if reference.MediaType != nil {
				value["media_type"] = *reference.MediaType
			}
			references = append(references, value)
		}
		events[index].Payload["attachments"] = references
	}
	return nil, events
}

func sessionEventAttachmentIDs(event model.SessionEvent) ([]string, bool, error) {
	value, exists := event.Payload["attachments"]
	if !exists {
		return nil, false, nil
	}
	switch attachments := value.(type) {
	case []string:
		return attachments, true, nil
	case []interface{}:
		ids := make([]string, 0, len(attachments))
		for _, attachment := range attachments {
			id, ok := attachment.(string)
			if !ok {
				return nil, false, nil
			}
			ids = append(ids, id)
		}
		return ids, true, nil
	default:
		return nil, false, nil
	}
}

func (store *Store) SessionEventsCreateBatch(ctx context.Context, events []model.SessionEvent) (error, []model.SessionEvent) {
	return store.sessionEventsCreateBatch(ctx, events, false, false)
}

// SessionApprovalResponseCreate persists one terminal approval response and schedules its delivery.
func (store *Store) SessionApprovalResponseCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	err, events := store.sessionEventsCreateBatch(ctx, []model.SessionEvent{event}, false, true)
	if err != nil {
		return err, model.SessionEvent{}
	}
	return nil, events[0]
}

type sessionEventInsert struct {
	event                    model.SessionEvent
	parent, principal, agent any
	gateway                  any
	payload                  string
	metrics                  any
}

func (store *Store) sessionEventsCreateBatch(ctx context.Context, events []model.SessionEvent, createReplyTasks, createApprovalResponse bool) (error, []model.SessionEvent) {
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
		createdAt, err := typed_id.Timestamp(typed_id.SessionEvent, event.Ref.Id)
		if err != nil {
			return fmt.Errorf("create session events: event %d ID is invalid", index), nil
		}
		event.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
		if createReplyTasks {
			attachments, err := sessionMessageAttachmentIDs(event)
			if err != nil {
				return err, nil
			}
			err, _ = store.sessionFileReferencesGet(ctx, transaction, event.Ref.Session, attachments)
			if err != nil {
				return fmt.Errorf("create session message: %w", err), nil
			}
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("encode session event payload: %w", err), nil
		}
		metrics, err := sessionEventMetricsValue(event.Metrics)
		if err != nil {
			return err, nil
		}
		inserts = append(inserts, sessionEventInsert{event: event, parent: parent, principal: principal, agent: agent, gateway: gateway, payload: string(payload), metrics: metrics})
		seen[event.Ref] = struct{}{}
	}
	for _, insert := range inserts {
		if createApprovalResponse && (insert.event.Parent == nil || (insert.event.Kind != "approval.approved" && insert.event.Kind != "approval.rejected")) {
			return fmt.Errorf("create approval response: event must be an approval.approved or approval.rejected child"), nil
		}
		row := transaction.QueryRowContext(ctx, `
			INSERT INTO gatehouse_session_events (
				workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, metrics, created_at
			) VALUES (
				`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`,
				`+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`
			)
			RETURNING created_at
		`, insert.event.Ref.Session.Workspace.Id, insert.event.Ref.Session.Id, insert.event.Ref.Id, insert.parent, insert.event.Kind, insert.principal, insert.agent, insert.gateway, insert.payload, insert.metrics, insert.event.CreatedAt)
		if err := row.Scan(&insert.event.CreatedAt); err != nil {
			return fmt.Errorf("insert session event: %w", err), nil
		}
		topics := []string{ActivityTopicSessionEvent(insert.event.Ref)}
		err, _ = store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
			Ref:                  model.ActivityEventRef{},
			Event:                "session_event.create",
			ResourceKind:         ActivityResourceKindSessionEvent,
			ResourceSessionEvent: &insert.event.Ref.Id,
			CreatedAt:            insert.event.CreatedAt,
		}, topics)
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
		if createApprovalResponse {
			result, err := transaction.ExecContext(ctx, `
				INSERT INTO gatehouse_session_approval_decisions (workspace, session, approval, response, created_at, delivered)
				VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, FALSE)
				ON CONFLICT (workspace, session, approval) DO NOTHING
			`, insert.event.Ref.Session.Workspace.Id, insert.event.Ref.Session.Id, insert.event.Parent.Id, insert.event.Ref.Id, insert.event.CreatedAt)
			if err != nil {
				return fmt.Errorf("create approval response decision: %w", err), nil
			}
			created, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("create approval response decision: %w", err), nil
			}
			if created != 1 {
				return ErrSessionApprovalResolved, nil
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
	return store.SessionEventsPageGet(ctx, session, "", 0)
}

func (store *Store) SessionEventsPageGet(ctx context.Context, session model.SessionRef, afterID string, limit int) (error, []model.SessionEvent) {
	if limit < 0 {
		return fmt.Errorf("get session events: limit must not be negative"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `
		SELECT events.id, events.parent, events.kind, events.author_principal, principals.alias, principals.name, principals.enabled, events.author_agent, events.author_gateway, events.payload, events.metrics, events.created_at
		FROM gatehouse_session_events AS events
		LEFT JOIN gatehouse_principals AS principals ON principals.id = events.author_principal
		WHERE events.workspace = ` + placeholder(1) + ` AND events.session = ` + placeholder(2) + `
			AND events.id > ` + placeholder(3) + `
		ORDER BY events.id
	`
	arguments := []any{session.Workspace.Id, session.Id, afterID}
	if limit > 0 {
		query += " LIMIT " + placeholder(4)
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
		var metrics sql.NullString
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
			&metrics,
			&createdAt,
		); err != nil {
			return fmt.Errorf("scan session event: %w", err), nil
		}
		decoded := map[string]interface{}{}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			return fmt.Errorf("decode session event payload: %w", err), nil
		}
		decodedMetrics, err := sessionEventMetricsFromValue(metrics)
		if err != nil {
			return err, nil
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
			Metrics:         decodedMetrics,
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

// SessionEventsTailGet returns a bounded creation-order suffix for cold context construction.
func (store *Store) SessionEventsTailGet(ctx context.Context, session model.SessionRef, limit int) (error, []model.SessionEvent) {
	return store.SessionEventsTailPageGet(ctx, session, "", limit)
}

// SessionEventsTailPageGet returns one creation-order page immediately before beforeID.
// An empty beforeID selects the newest page.
func (store *Store) SessionEventsTailPageGet(ctx context.Context, session model.SessionRef, beforeID string, limit int) (error, []model.SessionEvent) {
	if limit <= 0 {
		return fmt.Errorf("get session event tail page: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `
		SELECT id
		FROM gatehouse_session_events
		WHERE workspace = ` + placeholder(1) + ` AND session = ` + placeholder(2) + `
	`
	arguments := []any{session.Workspace.Id, session.Id}
	if beforeID != "" {
		query += ` AND id < ` + placeholder(3)
		arguments = append(arguments, beforeID)
	}
	query += `
		ORDER BY id DESC
		LIMIT ` + placeholder(len(arguments)+1)
	arguments = append(arguments, limit)
	rows, err := store.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("get session event tail page: %w", err), nil
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan session event tail page: %w", err), nil
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session event tail page: %w", err), nil
	}
	events := make([]model.SessionEvent, 0, len(ids))
	for index := len(ids) - 1; index >= 0; index-- {
		err, event := store.SessionEventGet(ctx, model.SessionEventRef{Session: session, Id: ids[index]})
		if err != nil {
			return err, nil
		}
		if event != nil {
			events = append(events, *event)
		}
	}
	return nil, events
}

type SessionEventTreeEntry struct {
	Event model.SessionEvent
	Depth int
}

// AgentContext is a durable MCMTR checkpoint owned by one reply tree.
// Session events remain the authoritative collaborative record.
type AgentContext struct {
	Root      model.SessionEventRef
	Model     model.WorkspaceAgentRef
	Profile   string
	State     json.RawMessage
	UpdatedAt string
}

func (store *Store) AgentContextGet(ctx context.Context, root model.SessionEventRef) (error, *AgentContext) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT model, profile, state, updated_at
		FROM gatehouse_agent_contexts
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND root = `+placeholder(3)+`
	`, root.Session.Workspace.Id, root.Session.Id, root.Id)
	context := AgentContext{Root: root, Model: model.WorkspaceAgentRef{Workspace: root.Session.Workspace}}
	var state string
	if err := row.Scan(&context.Model.Id, &context.Profile, &state, &context.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get agent context: %w", err), nil
	}
	context.State = json.RawMessage(state)
	return nil, &context
}

// AgentContextLatestGet returns the most recently checkpointed compatible context in a session.
func (store *Store) AgentContextLatestGet(ctx context.Context, session model.SessionRef, agent model.WorkspaceAgentRef, profile string) (error, *AgentContext) {
	if session.Workspace != agent.Workspace || agent.Id == "" || strings.TrimSpace(profile) == "" {
		return fmt.Errorf("get latest agent context: invalid context selector"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT root, state, updated_at
		FROM gatehouse_agent_contexts
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND model = `+placeholder(3)+` AND profile = `+placeholder(4)+`
		ORDER BY updated_at DESC, root DESC
		LIMIT 1
	`, session.Workspace.Id, session.Id, agent.Id, profile)
	context := AgentContext{Root: model.SessionEventRef{Session: session}, Model: agent, Profile: profile}
	var state string
	if err := row.Scan(&context.Root.Id, &state, &context.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get latest agent context: %w", err), nil
	}
	context.State = json.RawMessage(state)
	return nil, &context
}

func (store *Store) AgentContextSet(ctx context.Context, context AgentContext) error {
	if context.Root.Session.Workspace != context.Model.Workspace || context.Root.Id == "" || context.Model.Id == "" || strings.TrimSpace(context.Profile) == "" || !json.Valid(context.State) {
		return fmt.Errorf("set agent context: invalid context")
	}
	if context.UpdatedAt == "" {
		context.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	placeholder := keychainPlaceholder(store.kind)
	_, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_agent_contexts (workspace, session, root, model, profile, state, updated_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`)
		ON CONFLICT (workspace, session, root) DO UPDATE SET
			model = excluded.model, profile = excluded.profile, state = excluded.state, updated_at = excluded.updated_at
	`, context.Root.Session.Workspace.Id, context.Root.Session.Id, context.Root.Id, context.Model.Id, context.Profile, string(context.State), context.UpdatedAt)
	if err != nil {
		return fmt.Errorf("set agent context: %w", err)
	}
	return nil
}

func (store *Store) AgentContextDelete(ctx context.Context, root model.SessionEventRef) error {
	placeholder := keychainPlaceholder(store.kind)
	_, err := store.ExecContext(ctx, `
		DELETE FROM gatehouse_agent_contexts
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND root = `+placeholder(3)+`
	`, root.Session.Workspace.Id, root.Session.Id, root.Id)
	if err != nil {
		return fmt.Errorf("delete agent context: %w", err)
	}
	return nil
}

func (store *Store) SessionEventsTreePageGet(ctx context.Context, session model.SessionRef, afterID string, limit int) (error, []SessionEventTreeEntry) {
	if limit <= 0 {
		return fmt.Errorf("get session event tree: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH RECURSIVE
		roots AS (
			SELECT workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, metrics, created_at,
				0 AS depth, id AS display_path
			FROM gatehouse_session_events
			WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+`
				AND parent IS NULL
				AND id > `+placeholder(3)+`
			ORDER BY id
			LIMIT `+placeholder(4)+`
		),
		tree AS (
			SELECT workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, metrics, created_at, depth, display_path
			FROM roots

			UNION ALL

			SELECT child.workspace, child.session, child.id, child.parent, child.kind, child.author_principal, child.author_agent, child.author_gateway, child.payload, child.metrics, child.created_at,
				tree.depth + 1, tree.display_path || '/' || child.id
			FROM gatehouse_session_events AS child
			JOIN tree ON child.workspace = tree.workspace AND child.session = tree.session AND child.parent = tree.id
		)
		SELECT tree.id, tree.parent, tree.kind, tree.author_principal, principals.alias, principals.name, principals.enabled, tree.author_agent, tree.author_gateway, tree.payload, tree.metrics, tree.created_at, tree.depth
		FROM tree
		LEFT JOIN gatehouse_principals AS principals ON principals.id = tree.author_principal
		ORDER BY display_path
	`, session.Workspace.Id, session.Id, afterID, limit)
	if err != nil {
		return fmt.Errorf("get session event tree: %w", err), nil
	}
	defer rows.Close()

	entries := []SessionEventTreeEntry{}
	for rows.Next() {
		var entry SessionEventTreeEntry
		var id, kind, payload, createdAt string
		var metrics sql.NullString
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
			&metrics,
			&createdAt,
			&entry.Depth,
		); err != nil {
			return fmt.Errorf("scan session event tree entry: %w", err), nil
		}
		decoded := map[string]interface{}{}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			return fmt.Errorf("decode session event payload: %w", err), nil
		}
		decodedMetrics, err := sessionEventMetricsFromValue(metrics)
		if err != nil {
			return err, nil
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
			Metrics:         decodedMetrics,
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

// SessionEventsTreeTailGet returns a bounded suffix of root trees in creation order.
func (store *Store) SessionEventsTreeTailGet(ctx context.Context, session model.SessionRef, limit int) (error, []SessionEventTreeEntry) {
	if limit <= 0 {
		return fmt.Errorf("get session event tree tail: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT id
		FROM gatehouse_session_events
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND parent IS NULL
		ORDER BY id DESC
		LIMIT 1 OFFSET `+placeholder(3)+`
	`, session.Workspace.Id, session.Id, limit)
	var before string
	if err := row.Scan(&before); err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("get session event tree tail: %w", err), nil
		}
		before = ""
	}
	return store.SessionEventsTreePageGet(ctx, session, before, limit)
}

func (store *Store) SessionEventGet(ctx context.Context, event model.SessionEventRef) (error, *model.SessionEvent) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT events.parent, events.kind, events.author_principal, principals.alias, principals.name, principals.enabled, events.author_agent, events.author_gateway, events.payload, events.metrics, events.created_at
		FROM gatehouse_session_events AS events
		LEFT JOIN gatehouse_principals AS principals ON principals.id = events.author_principal
		WHERE events.workspace = `+placeholder(1)+` AND events.session = `+placeholder(2)+` AND events.id = `+placeholder(3)+`
	`, event.Session.Workspace.Id, event.Session.Id, event.Id)
	var stored model.SessionEvent
	stored.Ref = event
	var payload, createdAt string
	var metrics sql.NullString
	var parent, principal, principalAlias, principalName, agent, gateway sql.NullString
	var principalEnabled sql.NullBool
	if err := row.Scan(&parent, &stored.Kind, &principal, &principalAlias, &principalName, &principalEnabled, &agent, &gateway, &payload, &metrics, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session event: %w", err), nil
	}
	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return fmt.Errorf("decode session event payload: %w", err), nil
	}
	decodedMetrics, err := sessionEventMetricsFromValue(metrics)
	if err != nil {
		return err, nil
	}
	authorPrincipal, authorAgent, authorGateway, err := sessionEventAuthorsFromValues(event.Session.Workspace, principal, principalAlias, principalName, principalEnabled, agent, gateway)
	if err != nil {
		return err, nil
	}
	stored.AuthorPrincipal = authorPrincipal
	stored.AuthorAgent = authorAgent
	stored.AuthorGateway = authorGateway
	stored.Payload = decoded
	stored.Metrics = decodedMetrics
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

type SessionApprovalDecisionTask struct {
	Approval model.SessionEventRef
	Response model.SessionEventRef
}

func (store *Store) SessionApprovalDecisionTasksGet(ctx context.Context, limit int) (error, []SessionApprovalDecisionTask) {
	if limit <= 0 {
		return fmt.Errorf("get session approval decision tasks: limit must be positive"), nil
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT workspace, session, approval, response
		FROM gatehouse_session_approval_decisions
		WHERE delivered = FALSE
		ORDER BY created_at, response
		LIMIT `+placeholder(1)+`
	`, limit)
	if err != nil {
		return fmt.Errorf("get session approval decision tasks: %w", err), nil
	}
	defer rows.Close()

	tasks := []SessionApprovalDecisionTask{}
	for rows.Next() {
		var task SessionApprovalDecisionTask
		if err := rows.Scan(&task.Approval.Session.Workspace.Id, &task.Approval.Session.Id, &task.Approval.Id, &task.Response.Id); err != nil {
			return fmt.Errorf("scan session approval decision task: %w", err), nil
		}
		task.Response.Session = task.Approval.Session
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session approval decision tasks: %w", err), nil
	}
	return nil, tasks
}

func (store *Store) SessionApprovalDecisionTaskDelivered(ctx context.Context, approval model.SessionEventRef) error {
	placeholder := keychainPlaceholder(store.kind)
	_, err := store.ExecContext(ctx, `
		UPDATE gatehouse_session_approval_decisions SET delivered = TRUE
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND approval = `+placeholder(3)+`
	`, approval.Session.Workspace.Id, approval.Session.Id, approval.Id)
	if err != nil {
		return fmt.Errorf("mark session approval decision delivered: %w", err)
	}
	return nil
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
		RETURNING id, project
	`, name, session.Workspace.Id, session.Id)
	var updated string
	var project sql.NullString
	if err := row.Scan(&updated, &project); err != nil {
		if err == sql.ErrNoRows {
			if err := transaction.Commit(); err != nil {
				return fmt.Errorf("commit unchanged session name: %w", err), false
			}
			return nil, false
		}
		return fmt.Errorf("update session name: %w", err), false
	}
	topics := []string{ActivityTopicSession(session)}
	if project.Valid {
		topics = append(topics, ActivityTopicProject(model.ProjectRef{Workspace: session.Workspace, Id: project.String}))
	}
	err, _ = store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:             model.ActivityEventRef{},
		Event:           "session.update",
		ResourceKind:    ActivityResourceKindSession,
		ResourceSession: &session.Id,
	}, topics)
	if err != nil {
		return fmt.Errorf("append session name activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session name update: %w", err), false
	}
	return nil, true
}

func (store *Store) SessionNameUpdate(ctx context.Context, session model.SessionRef, principal model.PrincipalRef, name string) (error, *model.Session) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return fmt.Errorf("update session name: name must not be blank"), nil
	}
	if len(name) > 256 {
		return fmt.Errorf("update session name: name is too long"), nil
	}
	err, stored := store.SessionGet(ctx, session, principal)
	if err != nil || stored == nil {
		return err, stored
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session name update: %w", err), nil
	}
	defer transaction.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_sessions
		SET name = `+placeholder(1)+`
		WHERE workspace = `+placeholder(2)+` AND id = `+placeholder(3)+`
	`, name, session.Workspace.Id, session.Id); err != nil {
		return fmt.Errorf("update session name: %w", err), nil
	}
	topics := []string{ActivityTopicSession(session)}
	if stored.Project != nil {
		topics = append(topics, ActivityTopicProject(*stored.Project))
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:             model.ActivityEventRef{},
		Event:           "session.update",
		ResourceKind:    ActivityResourceKindSession,
		ResourceSession: &session.Id,
	}, topics); err != nil {
		return fmt.Errorf("append session name update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session name update: %w", err), nil
	}
	stored.Name = &name
	return nil, stored
}

type WorkspaceAgentModel struct {
	Ref             model.WorkspaceAgentRef
	AgentModel      model.AgentModelRef
	ProviderID      string
	ProviderAlias   *string
	Protocol        string
	BaseURL         *string
	Keychain        *model.KeychainRef
	APIKey          *string
	Model           string
	Parameters      string
	Compaction      string
	MaxTurns        int
	MaxOutputTokens int
	SystemPrompt    *string
}

type WorkspaceAgent struct {
	ID    string
	Alias string
	Label *string
}

func (store *Store) WorkspaceAgentsGet(ctx context.Context, workspace model.WorkspaceRef) (error, []WorkspaceAgent) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT bindings.id, bindings.alias, bindings.label
		FROM gatehouse_workspace_agents AS bindings
		JOIN gatehouse_agent_models AS models ON models.id = bindings.model_id
		JOIN gatehouse_agent_providers AS providers ON providers.id = models.provider_id
		WHERE bindings.workspace_id = `+placeholder(1)+`
			AND bindings.enabled = TRUE
			AND models.enabled = TRUE
			AND providers.enabled = TRUE
		ORDER BY bindings.priority DESC, bindings.id
	`, workspace.Id)
	if err != nil {
		return fmt.Errorf("get workspace agents: %w", err), nil
	}
	defer rows.Close()
	agents := []WorkspaceAgent{}
	for rows.Next() {
		var agent WorkspaceAgent
		var label sql.NullString
		if err := rows.Scan(&agent.ID, &agent.Alias, &label); err != nil {
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
		SELECT bindings.id, models.id, providers.id, providers.alias, providers.protocol, providers.base_url, providers.keychain_id, providers.keychain_version, providers.api_key, models.model, models.parameters, models.compaction, models.max_turns, models.max_output_tokens, bindings.system_prompt
		FROM gatehouse_workspace_agents AS bindings
		JOIN gatehouse_agent_models AS models ON models.id = bindings.model_id
		JOIN gatehouse_agent_providers AS providers ON providers.id = models.provider_id
		WHERE bindings.workspace_id = `+placeholder(1)+`
			AND bindings.enabled = TRUE
			AND models.enabled = TRUE
			AND providers.enabled = TRUE
		ORDER BY CASE WHEN `+placeholder(2)+` <> '' AND bindings.id = `+placeholder(3)+` THEN 0 ELSE 1 END, bindings.priority DESC, RANDOM()
		LIMIT 1
	`, workspace.Id, preferred, preferred)
	var selected WorkspaceAgentModel
	selected.Ref.Workspace = workspace
	var providerAlias, baseURL, keychainID, apiKey, systemPrompt sql.NullString
	var keychainVersion sql.NullInt64
	if err := row.Scan(&selected.Ref.Id, &selected.AgentModel.Id, &selected.ProviderID, &providerAlias, &selected.Protocol, &baseURL, &keychainID, &keychainVersion, &apiKey, &selected.Model, &selected.Parameters, &selected.Compaction, &selected.MaxTurns, &selected.MaxOutputTokens, &systemPrompt); err != nil {
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
	if providerAlias.Valid {
		selected.ProviderAlias = &providerAlias.String
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

func sessionEventMetricsValue(metrics *model.SessionEventMetrics) (any, error) {
	if metrics == nil {
		return nil, nil
	}
	for name, value := range map[string]*int{
		"request_ms": metrics.RequestMs, "input_tokens": metrics.InputTokens, "cached_input_tokens": metrics.CachedInputTokens,
		"output_tokens": metrics.OutputTokens, "reasoning_tokens": metrics.ReasoningTokens, "total_tokens": metrics.TotalTokens,
	} {
		if value != nil && *value < 0 {
			return nil, fmt.Errorf("create session event: metrics.%s must not be negative", name)
		}
	}
	if metrics.CachedInputTokens != nil && metrics.InputTokens != nil && *metrics.CachedInputTokens > *metrics.InputTokens {
		return nil, fmt.Errorf("create session event: cached input tokens exceed input tokens")
	}
	encoded, err := json.Marshal(metrics)
	if err != nil {
		return nil, fmt.Errorf("encode session event metrics: %w", err)
	}
	return string(encoded), nil
}

func sessionEventMetricsFromValue(value sql.NullString) (*model.SessionEventMetrics, error) {
	if !value.Valid {
		return nil, nil
	}
	metrics := model.SessionEventMetrics{}
	if err := json.Unmarshal([]byte(value.String), &metrics); err != nil {
		return nil, fmt.Errorf("decode session event metrics: %w", err)
	}
	if _, err := sessionEventMetricsValue(&metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
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
		agent = event.AuthorAgent.Id
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
		return nil, &model.WorkspaceAgentRef{Workspace: workspace, Id: agent.String}, nil, nil
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
			sessions.project,
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
				FROM gatehouse_session_grants AS grants
					WHERE grants.workspace = sessions.workspace
						AND grants.session = sessions.id
						AND grants.principal = `+placeholder(3)+`
						AND grants.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1
					FROM gatehouse_session_grants AS grants
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
		var project, name, authorPrincipal, authorAgent, authorGateway sql.NullString
		if err := rows.Scan(
			&session.Ref.Workspace.Id,
			&project,
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
		if project.Valid {
			session.Project = &model.ProjectRef{Workspace: session.Ref.Workspace, Id: project.String}
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
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close sessions: %w", err), nil
	}
	visible := make([]model.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.Project != nil {
			err, available := store.ProjectGet(ctx, *session.Project, principal)
			if err != nil {
				return err, nil
			}
			if available == nil {
				continue
			}
		}
		visible = append(visible, session)
	}
	return nil, visible
}

type SessionSearch struct {
	Name    string
	Project string
	Cursor  string
	Limit   int
}

func (store *Store) SessionsSearch(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, search SessionSearch) (error, []model.Session, string) {
	return store.sessionsSearch(ctx, workspace, principal, search, "")
}

func (store *Store) sessionsSearch(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, search SessionSearch, id string) (error, []model.Session, string) {
	if search.Limit < 1 || search.Limit > 100 {
		return fmt.Errorf("search sessions: limit must be between 1 and 100"), nil, ""
	}
	if search.Cursor != "" && !typed_id.Valid(typed_id.Session, search.Cursor) {
		return fmt.Errorf("search sessions: cursor is invalid"), nil, ""
	}
	if search.Project != "" && !typed_id.Valid(typed_id.Project, search.Project) {
		return fmt.Errorf("search sessions: project is invalid"), nil, ""
	}
	if id != "" && !typed_id.Valid(typed_id.Session, id) {
		return fmt.Errorf("search sessions: ID is invalid"), nil, ""
	}
	name := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(search.Name)) + "%"
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT
			sessions.workspace,
			sessions.project,
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
			AND LOWER(COALESCE(sessions.name, '')) LIKE LOWER(`+placeholder(2)+`) ESCAPE '\'
			AND (`+placeholder(3)+` = '' OR sessions.id < `+placeholder(4)+`)
			AND EXISTS (
				SELECT 1 FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = sessions.workspace AND workspaces.enabled = TRUE
			)
			AND EXISTS (
				SELECT 1 FROM gatehouse_principals AS principals
				WHERE principals.id = `+placeholder(5)+` AND principals.enabled = TRUE
			)
			AND (
				sessions.project IS NULL
				OR EXISTS (
					SELECT 1 FROM gatehouse_projects AS projects
					WHERE projects.workspace = sessions.workspace AND projects.id = sessions.project
						AND projects.enabled = TRUE
				)
			)
			AND (
				EXISTS (
					SELECT 1 FROM gatehouse_session_grants AS grants
					WHERE grants.workspace = sessions.workspace AND grants.session = sessions.id
						AND grants.principal = `+placeholder(6)+` AND grants.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1 FROM gatehouse_session_grants AS grants
					JOIN gatehouse_groups AS groups
						ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members
						ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					WHERE grants.workspace = sessions.workspace AND grants.session = sessions.id
						AND grants.enabled = TRUE AND groups.enabled = TRUE
						AND members.principal_id = `+placeholder(7)+` AND members.enabled = TRUE
				)
			)
			AND (
				sessions.project IS NULL
				OR EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					WHERE grants.workspace = sessions.workspace AND grants.project = sessions.project
						AND grants.principal = `+placeholder(8)+` AND grants.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					JOIN gatehouse_groups AS groups
						ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members
						ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					WHERE grants.workspace = sessions.workspace AND grants.project = sessions.project
						AND grants.enabled = TRUE AND groups.enabled = TRUE
						AND members.principal_id = `+placeholder(9)+` AND members.enabled = TRUE
				)
			)
			AND (`+placeholder(10)+` = '' OR sessions.project = `+placeholder(11)+`)
			AND (`+placeholder(12)+` = '' OR sessions.id = `+placeholder(13)+`)
		ORDER BY sessions.id DESC
		LIMIT `+placeholder(14)+`
	`, workspace.Id, name, search.Cursor, search.Cursor, principal.Id, principal.Id, principal.Id, principal.Id, principal.Id, search.Project, search.Project, id, id, search.Limit+1)
	if err != nil {
		return fmt.Errorf("search sessions: %w", err), nil, ""
	}
	defer rows.Close()

	sessions := []model.Session{}
	for rows.Next() {
		var session model.Session
		var project, name, authorPrincipal, authorAgent, authorGateway sql.NullString
		if err := rows.Scan(
			&session.Ref.Workspace.Id,
			&project,
			&session.Ref.Id,
			&name,
			&authorPrincipal,
			&authorAgent,
			&authorGateway,
			&session.Enabled,
			&session.CreatedAt,
		); err != nil {
			return fmt.Errorf("scan session search result: %w", err), nil, ""
		}
		if name.Valid {
			session.Name = &name.String
		}
		if project.Valid {
			session.Project = &model.ProjectRef{Workspace: session.Ref.Workspace, Id: project.String}
		}
		authorPrincipalRef, authorAgentRef, authorGatewayRef, err := sessionAuthorsFromValues(session.Ref.Workspace, authorPrincipal, authorAgent, authorGateway)
		if err != nil {
			return err, nil, ""
		}
		session.AuthorPrincipal = authorPrincipalRef
		session.AuthorAgent = authorAgentRef
		session.AuthorGateway = authorGatewayRef
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session search results: %w", err), nil, ""
	}
	if len(sessions) <= search.Limit {
		return nil, sessions, ""
	}
	nextCursor := sessions[search.Limit-1].Ref.Id
	return nil, sessions[:search.Limit], nextCursor
}
