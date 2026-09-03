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
	ActivityResourceKindProject                  = "project"
	ActivityResourceKindSession                  = "session"
	ActivityResourceKindSessionEvent             = "session_event"
	ActivityResourceKindGroup                    = "group"
	ActivityResourceKindGroupMember              = "group_member"
	ActivityResourceKindWorkspaceAgent           = "workspace_agent"
	ActivityResourceKindWorkspace                = "workspace"
	ActivityResourceKindWorkspaceGrant           = "workspace_grant"
	ActivityResourceKindWorkspaceStorageProvider = "workspace_storage_provider"
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

func ActivityTopicGroup(group model.GroupRef) string {
	return "group/" + group.Id
}

func ActivityTopicGroupMember(member model.GroupMemberRef) string {
	return "group_member/" + member.Group.Id + "-" + member.Principal.Id
}

func ActivityTopicWorkspaceAgent(agent model.WorkspaceAgentRef) string {
	return "workspace_agent/" + agent.Model.Id
}

func ActivityTopicWorkspace(workspace model.WorkspaceRef) string {
	return "workspace/" + workspace.Id
}

func ActivityTopicWorkspaceGrant(role, subjectID string) string {
	return "workspace_grant/" + role + "-" + subjectID
}

func ActivityTopicWorkspaceStorageProvider(providerID string) string {
	return "workspace_storage_provider/" + providerID
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
	var project, session, sessionEvent, group, principal, workspaceAgent any
	if activity.Project != nil {
		project = activity.Project.Id
	}
	if activity.Session != nil {
		session = activity.Session.Id
	}
	if activity.SessionEvent != nil {
		sessionEvent = activity.SessionEvent.Id
	}
	if activity.Group != nil {
		group = activity.Group.Id
	}
	if activity.GroupMember != nil {
		group = activity.GroupMember.Group.Id
		principal = activity.GroupMember.Principal.Id
	}
	if activity.WorkspaceAgent != nil {
		workspaceAgent = activity.WorkspaceAgent.Model.Id
	}
	row := transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`
		)
		RETURNING created_at
	`, activity.Ref.Workspace.Id, activity.Ref.Id, activity.Event, activity.ResourceKind, project, session, sessionEvent, group, principal, workspaceAgent, activity.CreatedAt)
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
	var project, session, sessionEvent, group, principal, workspaceAgent any
	if activity.Project != nil {
		project = activity.Project.Id
	}
	if activity.Session != nil {
		session = activity.Session.Id
	}
	if activity.SessionEvent != nil {
		sessionEvent = activity.SessionEvent.Id
	}
	if activity.Group != nil {
		group = activity.Group.Id
	}
	if activity.GroupMember != nil {
		group = activity.GroupMember.Group.Id
		principal = activity.GroupMember.Principal.Id
	}
	if activity.WorkspaceAgent != nil {
		workspaceAgent = activity.WorkspaceAgent.Model.Id
	}
	row := transaction.QueryRow(ctx, `
		INSERT INTO gatehouse_activity_events (
			workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
		) VALUES (
			`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`
		)
		RETURNING created_at
	`, activity.Ref.Workspace.Id, activity.Ref.Id, activity.Event, activity.ResourceKind, project, session, sessionEvent, group, principal, workspaceAgent, activity.CreatedAt)
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
		if activity.Project == nil || activity.Session != nil || activity.SessionEvent != nil || activity.Group != nil || activity.GroupMember != nil || activity.WorkspaceAgent != nil {
			return fmt.Errorf("append activity event: project subject requires only a project")
		}
	case ActivityResourceKindSession:
		if activity.Session == nil || activity.SessionEvent != nil || activity.Group != nil || activity.GroupMember != nil || activity.WorkspaceAgent != nil {
			return fmt.Errorf("append activity event: session subject requires only a session")
		}
	case ActivityResourceKindSessionEvent:
		if activity.Session == nil || activity.SessionEvent == nil || activity.SessionEvent.Session != *activity.Session || activity.Group != nil || activity.GroupMember != nil || activity.WorkspaceAgent != nil {
			return fmt.Errorf("append activity event: session event subject requires matching session and session event")
		}
	case ActivityResourceKindGroup:
		if activity.Group == nil || activity.Project != nil || activity.Session != nil || activity.SessionEvent != nil || activity.GroupMember != nil || activity.WorkspaceAgent != nil {
			return fmt.Errorf("append activity event: group subject requires only a group")
		}
	case ActivityResourceKindGroupMember:
		if activity.GroupMember == nil || activity.Project != nil || activity.Session != nil || activity.SessionEvent != nil || activity.Group != nil || activity.WorkspaceAgent != nil {
			return fmt.Errorf("append activity event: group member subject requires only a group member")
		}
	case ActivityResourceKindWorkspaceAgent:
		if activity.WorkspaceAgent == nil || activity.Project != nil || activity.Session != nil || activity.SessionEvent != nil || activity.Group != nil || activity.GroupMember != nil {
			return fmt.Errorf("append activity event: workspace agent subject requires only a workspace agent")
		}
	case ActivityResourceKindWorkspace, ActivityResourceKindWorkspaceGrant, ActivityResourceKindWorkspaceStorageProvider:
		if activity.Project != nil || activity.Session != nil || activity.SessionEvent != nil || activity.Group != nil || activity.GroupMember != nil || activity.WorkspaceAgent != nil {
			return fmt.Errorf("append activity event: workspace-scoped subject requires no additional resource")
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
	if activity.Group != nil && (activity.Group.Workspace != activity.Ref.Workspace || strings.TrimSpace(activity.Group.Id) == "") {
		return fmt.Errorf("append activity event: group subject belongs to another workspace or has no ID")
	}
	if activity.GroupMember != nil && (activity.GroupMember.Group.Workspace != activity.Ref.Workspace || strings.TrimSpace(activity.GroupMember.Group.Id) == "" || strings.TrimSpace(activity.GroupMember.Principal.Id) == "") {
		return fmt.Errorf("append activity event: group member subject belongs to another workspace or has no ID")
	}
	if activity.WorkspaceAgent != nil && (activity.WorkspaceAgent.Workspace != activity.Ref.Workspace || strings.TrimSpace(activity.WorkspaceAgent.Model.Id) == "") {
		return fmt.Errorf("append activity event: workspace agent subject belongs to another workspace or has no ID")
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
			SELECT topics.topic, activities.id, activities.resource_kind, activities.project, activities.session, activities."group", activities.principal, activities.workspace_agent
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
			topic, id, resourceKind                            string
			project, session, group, principal, workspaceAgent sql.NullString
		}
		candidates := []candidate{}
		for rows.Next() {
			var topic, id, resourceKind string
			var project, session, group, principal, workspaceAgent sql.NullString
			if err := rows.Scan(&topic, &id, &resourceKind, &project, &session, &group, &principal, &workspaceAgent); err != nil {
				rows.Close()
				return fmt.Errorf("scan activity topic checkpoint: %w", err), nil
			}
			if activityTopicMatches(checkpoint.Topic, topic) {
				candidates = append(candidates, candidate{id: id, resourceKind: resourceKind, project: project, session: session, group: group, principal: principal, workspaceAgent: workspaceAgent})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate activity topic checkpoint: %w", err), nil
		}
		rows.Close()
		next := checkpoint
		for _, candidate := range candidates {
			authorized, err := store.activityTopicAuthorized(ctx, workspace, principal, candidate.resourceKind, candidate.project, candidate.session, candidate.group, candidate.principal, candidate.workspaceAgent)
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

func (store *Store) activityTopicAuthorized(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, resourceKind string, project, session, group, member, workspaceAgent sql.NullString) (bool, error) {
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
	case ActivityResourceKindGroup:
		if !group.Valid {
			return false, nil
		}
		err, available := store.WorkspaceGet(ctx, workspace, principal)
		if err != nil || available == nil {
			return false, err
		}
		return store.activityGroupAuthorized(ctx, workspace, principal, group.String)
	case ActivityResourceKindGroupMember:
		if !group.Valid || !member.Valid {
			return false, nil
		}
		err, available := store.WorkspaceGet(ctx, workspace, principal)
		if err != nil || available == nil {
			return false, err
		}
		return store.activityGroupAuthorized(ctx, workspace, principal, group.String)
	case ActivityResourceKindWorkspaceAgent:
		if !workspaceAgent.Valid {
			return false, nil
		}
		err, available := store.WorkspaceGet(ctx, workspace, principal)
		return available != nil, err
	case ActivityResourceKindWorkspace, ActivityResourceKindWorkspaceGrant, ActivityResourceKindWorkspaceStorageProvider:
		err, available := store.WorkspaceGet(ctx, workspace, principal)
		return available != nil, err
	default:
		return false, nil
	}
}

func (store *Store) activityGroupAuthorized(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, groupID string) (bool, error) {
	placeholder := keychainPlaceholder(store.kind)
	var available bool
	err := store.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM gatehouse_groups AS groups
			JOIN gatehouse_group_members AS members
				ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
			JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id
			WHERE groups.workspace_id = `+placeholder(1)+`
				AND groups.id = `+placeholder(2)+`
				AND groups.enabled = TRUE
				AND members.principal_id = `+placeholder(3)+`
				AND members.enabled = TRUE
				AND principals.enabled = TRUE
				AND workspaces.enabled = TRUE
		)
	`, workspace.Id, groupID, principal.Id).Scan(&available)
	if err != nil {
		return false, fmt.Errorf("authorize group activity topic: %w", err)
	}
	return available, nil
}
