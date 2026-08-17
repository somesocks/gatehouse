package database

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"strings"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/model"
	"gatehouse/typed_id"
)

const (
	ActivityResourceKindProject      = "project"
	ActivityResourceKindSession      = "session"
	ActivityResourceKindSessionEvent = "session_event"
)

type ActivityTopicCheckpoint struct {
	Topic string
	ID    string
}

func ActivityTopicProject(project model.ProjectRef) string {
	return "project/" + project.Id
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
	var project, session, sessionEvent any
	if activity.Project != nil {
		project = activity.Project.Id
	}
	if activity.Session != nil {
		session = activity.Session.Id
	}
	if activity.SessionEvent != nil {
		sessionEvent = activity.SessionEvent.Id
	}
	row := transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, project, session, session_event, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`
		)
		RETURNING created_at
	`, activity.Ref.Workspace.Id, activity.Ref.Id, activity.Event, activity.ResourceKind, project, session, sessionEvent, activity.CreatedAt)
	if err := row.Scan(&activity.CreatedAt); err != nil {
		return fmt.Errorf("insert activity event: %w", err), model.ActivityEvent{}
	}
	for _, topic := range topics {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`)
		`, activity.Ref.Workspace.Id, activity.Ref.Id, topic, activity.CreatedAt); err != nil {
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
	var project, session, sessionEvent any
	if activity.Project != nil {
		project = activity.Project.Id
	}
	if activity.Session != nil {
		session = activity.Session.Id
	}
	if activity.SessionEvent != nil {
		sessionEvent = activity.SessionEvent.Id
	}
	row := transaction.QueryRow(ctx, `
		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, project, session, session_event, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`
		)
		RETURNING created_at
	`, activity.Ref.Workspace.Id, activity.Ref.Id, activity.Event, activity.ResourceKind, project, session, sessionEvent, activity.CreatedAt)
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
	case ActivityResourceKindProject:
		if activity.Project == nil || activity.Session != nil || activity.SessionEvent != nil {
			return fmt.Errorf("append activity event: project subject requires only a project")
		}
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
	if activity.Project != nil && (activity.Project.Workspace != activity.Ref.Workspace || strings.TrimSpace(activity.Project.Id) == "") {
		return fmt.Errorf("append activity event: project subject belongs to another workspace or has no ID")
	}
	if activity.Session != nil && (activity.Session.Workspace != activity.Ref.Workspace || strings.TrimSpace(activity.Session.Id) == "") {
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
		rows, err := store.QueryContext(ctx, `
			SELECT topics.topic, activities.id, activities.resource_kind, activities.project, activities.session
			FROM gatehouse_activity_event_topics AS topics
			JOIN gatehouse_activity_events AS activities
				ON activities.workspace = topics.workspace AND activities.id = topics.activity
			WHERE topics.workspace = `+placeholder(1)+`
				AND topics.activity > `+placeholder(2)+`
			ORDER BY topics.activity DESC
		`, workspace.Id, checkpoint.ID)
		if err != nil {
			return fmt.Errorf("get activity topic checkpoint: %w", err), nil
		}
		type candidate struct {
			topic, id, resourceKind string
			project, session  sql.NullString
		}
		candidates := []candidate{}
		for rows.Next() {
			var topic, id, resourceKind string
			var project, session sql.NullString
			if err := rows.Scan(&topic, &id, &resourceKind, &project, &session); err != nil {
				rows.Close()
				return fmt.Errorf("scan activity topic checkpoint: %w", err), nil
			}
			if activityTopicMatches(checkpoint.Topic, topic) {
				candidates = append(candidates, candidate{id: id, resourceKind: resourceKind, project: project, session: session})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate activity topic checkpoint: %w", err), nil
		}
		rows.Close()
		next := checkpoint
		for _, candidate := range candidates {
			authorized, err := store.activityTopicAuthorized(ctx, workspace, principal, candidate.resourceKind, candidate.project, candidate.session)
			if err != nil {
				return err, nil
			}
			if authorized {
				next.ID = candidate.id
				break
			}
		}
		advanced = append(advanced, next)
	}
	return nil, advanced
}

func activityTopicMatches(pattern, topic string) bool {
	matched, err := path.Match(pattern, topic)
	return err == nil && matched
}

func (store *Store) activityTopicAuthorized(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, resourceKind string, project, session sql.NullString) (bool, error) {
	switch resourceKind {
	case ActivityResourceKindProject:
		if !project.Valid {
			return false, nil
		}
		err, available := store.ProjectGet(ctx, model.ProjectRef{Workspace: workspace, Id: project.String}, principal)
		return available != nil, err
	case ActivityResourceKindSession, ActivityResourceKindSessionEvent:
		if !session.Valid {
			return false, nil
		}
		err, available := store.SessionGet(ctx, model.SessionRef{Workspace: workspace, Id: session.String}, principal)
		return available != nil, err
	default:
		return false, nil
	}
}
