package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/model"
	"gatehouse/typed_id"
)

const (
	ActivityResourceKindSession      = "session"
	ActivityResourceKindSessionEvent = "session_event"

	ActivityTopicSessions = "sessions"
)

type ActivityTopicCheckpoint struct {
	Topic string
	ID    string
}

func ActivityTopicSession(session model.SessionRef) string {
	return "session/" + session.Id
}

// ActivityEventAppend records a resource change in the caller's transaction.
func (store *Store) ActivityEventAppend(ctx context.Context, transaction *sql.Tx, activity model.ActivityEvent, topics []string) (error, model.ActivityEvent) {
	if transaction == nil {
		return fmt.Errorf("append activity event: transaction is required"), model.ActivityEvent{}
	}
	if err := validateActivityEvent(&activity, topics); err != nil {
		return err, model.ActivityEvent{}
	}
	if activity.Ref.Id == "" {
		id, err := typed_id.New(typed_id.ActivityEvent)
		if err != nil {
			return fmt.Errorf("generate activity event ID: %w", err), model.ActivityEvent{}
		}
		activity.Ref.Id = id
	}
	createdAt, err := typed_id.Timestamp(typed_id.ActivityEvent, activity.Ref.Id)
	if err != nil {
		return fmt.Errorf("append activity event: ID is invalid"), model.ActivityEvent{}
	}
	activity.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")

	placeholder := keychainPlaceholder(store.kind)
	var session, sessionEvent any
	if activity.Session != nil {
		session = activity.Session.Id
	}
	if activity.SessionEvent != nil {
		sessionEvent = activity.SessionEvent.Id
	}
	row := transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, session, session_event, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`
		)
		RETURNING created_at
	`, activity.Ref.Workspace.Id, activity.Ref.Id, activity.Event, activity.ResourceKind, session, sessionEvent, activity.CreatedAt)
	if err := row.Scan(&activity.CreatedAt); err != nil {
		return fmt.Errorf("insert activity event: %w", err), model.ActivityEvent{}
	}
	for _, topic := range topics {
		_, err := transaction.ExecContext(ctx, `
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`)
		`, activity.Ref.Workspace.Id, activity.Ref.Id, topic, activity.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert activity event topic: %w", err), model.ActivityEvent{}
		}
	}
	return nil, activity
}

// ActivityEventAppendInTransaction records an activity event in a DBOS transaction.
func (store *Store) ActivityEventAppendInTransaction(ctx context.Context, transaction dbos.Tx, activity model.ActivityEvent, topics []string) (error, model.ActivityEvent) {
	if transaction == nil {
		return fmt.Errorf("append activity event: transaction is required"), model.ActivityEvent{}
	}
	if err := validateActivityEvent(&activity, topics); err != nil {
		return err, model.ActivityEvent{}
	}
	if activity.Ref.Id == "" {
		id, err := typed_id.New(typed_id.ActivityEvent)
		if err != nil {
			return fmt.Errorf("generate activity event ID: %w", err), model.ActivityEvent{}
		}
		activity.Ref.Id = id
	}
	createdAt, err := typed_id.Timestamp(typed_id.ActivityEvent, activity.Ref.Id)
	if err != nil {
		return fmt.Errorf("append activity event: ID is invalid"), model.ActivityEvent{}
	}
	activity.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")

	placeholder := keychainPlaceholder(store.kind)
	var session, sessionEvent any
	if activity.Session != nil {
		session = activity.Session.Id
	}
	if activity.SessionEvent != nil {
		sessionEvent = activity.SessionEvent.Id
	}
	row := transaction.QueryRow(ctx, `
		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, session, session_event, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`
		)
		RETURNING created_at
	`, activity.Ref.Workspace.Id, activity.Ref.Id, activity.Event, activity.ResourceKind, session, sessionEvent, activity.CreatedAt)
	if err := row.Scan(&activity.CreatedAt); err != nil {
		return fmt.Errorf("insert activity event: %w", err), model.ActivityEvent{}
	}
	for _, topic := range topics {
		if _, err := transaction.Exec(ctx, `
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`)
		`, activity.Ref.Workspace.Id, activity.Ref.Id, topic, activity.CreatedAt); err != nil {
			return fmt.Errorf("insert activity event topic: %w", err), model.ActivityEvent{}
		}
	}
	return nil, activity
}

func validateActivityEvent(activity *model.ActivityEvent, topics []string) error {
	if strings.TrimSpace(activity.Ref.Workspace.Id) == "" || strings.TrimSpace(activity.Event) == "" {
		return fmt.Errorf("append activity event: workspace and event are required")
	}
	if len(topics) == 0 {
		return fmt.Errorf("append activity event: at least one topic is required")
	}
	seenTopics := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if strings.TrimSpace(topic) == "" {
			return fmt.Errorf("append activity event: topic must not be blank")
		}
		if _, exists := seenTopics[topic]; exists {
			return fmt.Errorf("append activity event: duplicate topic %q", topic)
		}
		seenTopics[topic] = struct{}{}
	}

	switch activity.ResourceKind {
	case ActivityResourceKindSession:
		if activity.Session == nil || activity.SessionEvent != nil {
			return fmt.Errorf("append activity event: session subject requires only a session")
		}
	case ActivityResourceKindSessionEvent:
		if activity.Session == nil || activity.SessionEvent == nil || activity.SessionEvent.Session != *activity.Session {
			return fmt.Errorf("append activity event: session event subject requires matching session and session event")
		}
	default:
		return fmt.Errorf("append activity event: unsupported resource kind %q", activity.ResourceKind)
	}
	if activity.Session.Workspace != activity.Ref.Workspace || strings.TrimSpace(activity.Session.Id) == "" {
		return fmt.Errorf("append activity event: session subject belongs to another workspace or has no ID")
	}
	if activity.SessionEvent != nil && strings.TrimSpace(activity.SessionEvent.Id) == "" {
		return fmt.Errorf("append activity event: session event subject has no ID")
	}
	return nil
}

func (store *Store) ActivityTopicCheckpointsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, checkpoints []ActivityTopicCheckpoint) (error, []ActivityTopicCheckpoint) {
	if strings.TrimSpace(workspace.Id) == "" || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("get activity topic checkpoints: workspace and principal are required"), nil
	}
	if len(checkpoints) == 0 {
		return fmt.Errorf("get activity topic checkpoints: at least one topic is required"), nil
	}
	seenTopics := make(map[string]struct{}, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if strings.TrimSpace(checkpoint.Topic) == "" {
			return fmt.Errorf("get activity topic checkpoints: topic must not be blank"), nil
		}
		if checkpoint.ID != "" && !typed_id.Valid(typed_id.ActivityEvent, checkpoint.ID) {
			return fmt.Errorf("get activity topic checkpoints: cursor ID is invalid"), nil
		}
		if _, exists := seenTopics[checkpoint.Topic]; exists {
			return fmt.Errorf("get activity topic checkpoints: duplicate topic %q", checkpoint.Topic), nil
		}
		seenTopics[checkpoint.Topic] = struct{}{}
	}

	placeholder := keychainPlaceholder(store.kind)
	advanced := make([]ActivityTopicCheckpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		row := store.QueryRowContext(ctx, `
			SELECT activities.id
			FROM gatehouse_activity_event_topics AS topics
			JOIN gatehouse_activity_events AS activities
				ON activities.workspace = topics.workspace AND activities.id = topics.activity
			WHERE topics.workspace = `+placeholder(1)+`
				AND topics.topic = `+placeholder(2)+`
				AND topics.activity > `+placeholder(3)+`
				AND EXISTS (
					SELECT 1 FROM gatehouse_workspaces AS workspaces
					WHERE workspaces.id = activities.workspace AND workspaces.enabled = TRUE
				)
				AND EXISTS (
					SELECT 1 FROM gatehouse_principals AS principals
					WHERE principals.id = `+placeholder(5)+` AND principals.enabled = TRUE
				)
				AND EXISTS (
					SELECT 1
					FROM gatehouse_sessions AS sessions
					WHERE sessions.workspace = activities.workspace
						AND sessions.id = activities.session
						AND sessions.enabled = TRUE
						AND (
							EXISTS (
								SELECT 1
								FROM gatehouse_session_principal_grants AS grants
								WHERE grants.workspace = sessions.workspace
									AND grants.session = sessions.id
									AND grants.principal = `+placeholder(6)+`
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
									AND members.principal_id = `+placeholder(7)+`
									AND members.enabled = TRUE
							)
						)
				)
			ORDER BY topics.activity DESC
			LIMIT 1
		`, workspace.Id, checkpoint.Topic, checkpoint.ID, principal.Id, principal.Id, principal.Id)
		next := checkpoint
		if err := row.Scan(&next.ID); err != nil {
			if err == sql.ErrNoRows {
				advanced = append(advanced, next)
				continue
			}
			return fmt.Errorf("get activity topic checkpoint: %w", err), nil
		}
		advanced = append(advanced, next)
	}
	return nil, advanced
}
