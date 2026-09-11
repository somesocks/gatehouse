package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/authz"
	"gatehouse/model"
	"gatehouse/typed_id"
)

const (
	ActivityResourceKindKeychain                 = "keychain"
	ActivityResourceKindAgentProvider            = "agent_provider"
	ActivityResourceKindAgentModel               = "agent_model"
	ActivityResourceKindGroup                    = "group"
	ActivityResourceKindGroupMember              = "group_member"
	ActivityResourceKindIdentity                 = "identity"
	ActivityResourceKindPrincipal                = "principal"
	ActivityResourceKindProject                  = "project"
	ActivityResourceKindProjectFile              = "project_file"
	ActivityResourceKindProjectGrant             = "project_grant"
	ActivityResourceKindProjectNote              = "project_note"
	ActivityResourceKindProjectTask              = "project_task"
	ActivityResourceKindProjectSecret            = "project_secret"
	ActivityResourceKindProjectRecordSchema      = "project_record_schema"
	ActivityResourceKindProjectRecordAttribute   = "project_record_attribute"
	ActivityResourceKindProjectRecord            = "project_record"
	ActivityResourceKindSession                  = "session"
	ActivityResourceKindSessionEvent             = "session_event"
	ActivityResourceKindSessionFile              = "session_file"
	ActivityResourceKindSessionGrant             = "session_grant"
	ActivityResourceKindSessionNote              = "session_note"
	ActivityResourceKindSessionTask              = "session_task"
	ActivityResourceKindSessionSecret            = "session_secret"
	ActivityResourceKindStorageProvider          = "storage_provider"
	ActivityResourceKindSystemGrant              = "system_grant"
	ActivityResourceKindWorkspace                = "workspace"
	ActivityResourceKindWorkspaceAgent           = "workspace_agent"
	ActivityResourceKindWorkspaceGrant           = "workspace_grant"
	ActivityResourceKindWorkspaceStorageProvider = "workspace_storage_provider"
)

var activityEventNames = []string{
	"agent_model.create", "agent_model.update", "agent_provider.create", "agent_provider.update",
	"group.create", "group.update", "group_member.create", "group_member.update",
	"identity.create", "identity.update", "keychain.create",
	"principal.create", "principal.update",
	"project.create", "project.update", "project_file.create", "project_file.remove", "project_file.update",
	"project_grant.create", "project_grant.update",
	"project_note.create", "project_note.remove", "project_note.update",
	"project_task.create", "project_task.remove", "project_task.update",
	"project_secret.create", "project_secret.remove", "project_secret.update",
	"project_record_schema.create", "project_record_schema.remove", "project_record_schema.update",
	"project_record_attribute.create", "project_record_attribute.remove", "project_record_attribute.update",
	"project_record.create", "project_record.remove", "project_record.update",
	"session.create", "session.project.link", "session.project.move", "session.project.unlink", "session.update", "session_event.create",
	"session_file.create", "session_file.remove", "session_file.update",
	"session_grant.create", "session_grant.update",
	"session_note.create", "session_note.remove", "session_note.update",
	"session_task.create", "session_task.remove", "session_task.update",
	"session_secret.create", "session_secret.remove", "session_secret.update",
	"storage_provider.create", "storage_provider.update",
	"system_grant.create", "system_grant.update",
	"workspace.create", "workspace.update", "workspace_agent.create", "workspace_agent.update",
	"workspace_grant.create", "workspace_grant.update",
	"workspace_storage_provider.create", "workspace_storage_provider.update",
}

type ActivityTopicCheckpoint struct {
	Name   *string
	Topic  string
	Events []string
	ID     string
}

func ActivityTopicProject(project model.ProjectRef) string {
	return project.Workspace.Id + "/" + project.Id
}

func ActivityTopicProjectFile(file model.ProjectFileRef) string {
	return ActivityTopicProject(file.Project) + "/" + file.Id
}

func ActivityTopicProjectNote(note model.ProjectNoteRef) string {
	return ActivityTopicProject(note.Project) + "/" + note.Id
}

func ActivityTopicProjectTask(task model.ProjectTaskRef) string {
	return ActivityTopicProject(task.Project) + "/" + task.Id
}

func ActivityTopicProjectSecret(secret model.ProjectSecretRef) string {
	return ActivityTopicProject(secret.Project) + "/" + secret.Id
}

func ActivityTopicProjectRecordSchema(schema model.ProjectRecordSchemaRef) string {
	return ActivityTopicProject(schema.Project) + "/" + schema.Id
}

func ActivityTopicProjectRecordAttribute(attribute model.ProjectRecordAttributeRef) string {
	return ActivityTopicProjectRecordSchema(attribute.Schema) + "/" + attribute.Id
}

func ActivityTopicProjectRecord(record model.ProjectRecordRef) string {
	return ActivityTopicProjectRecordSchema(record.Schema) + "/" + record.Id
}

func ActivityTopicProjectGrant(project model.ProjectRef, grantID string) string {
	return ActivityTopicProject(project) + "/" + grantID
}

func ActivityTopicSession(session model.SessionRef) string {
	return session.Workspace.Id + "/" + session.Id
}

func ActivityTopicSessionEvent(event model.SessionEventRef) string {
	return ActivityTopicSession(event.Session) + "/" + event.Id
}

func ActivityTopicSessionFile(file model.SessionFileRef) string {
	return ActivityTopicSession(file.Session) + "/" + file.Id
}

func ActivityTopicSessionNote(note model.SessionNoteRef) string {
	return ActivityTopicSession(note.Session) + "/" + note.Id
}

func ActivityTopicSessionTask(task model.SessionTaskRef) string {
	return ActivityTopicSession(task.Session) + "/" + task.Id
}

func ActivityTopicSessionSecret(secret model.SessionSecretRef) string {
	return ActivityTopicSession(secret.Session) + "/" + secret.Id
}

func ActivityTopicSessionGrant(session model.SessionRef, grantID string) string {
	return ActivityTopicSession(session) + "/" + grantID
}

func ActivityTopicGroup(workspace model.WorkspaceRef, group model.GroupRef) string {
	return workspace.Id + "/" + group.Id
}

func ActivityTopicGroupMember(workspace model.WorkspaceRef, member model.GroupMemberRef) string {
	return workspace.Id + "/" + member.Group.Id
}

func ActivityTopicWorkspaceAgent(agent model.WorkspaceAgentRef) string {
	return agent.Workspace.Id
}

func ActivityTopicWorkspace(workspace model.WorkspaceRef) string {
	return workspace.Id
}

func ActivityTopicWorkspaceGrant(workspace model.WorkspaceRef, grantID string) string {
	return workspace.Id + "/" + grantID
}

func ActivityTopicPrincipalGrant(principal model.PrincipalRef, grantID string) string {
	return principal.Id + "/" + grantID
}

func ActivityTopicSystemGrant(grant model.SystemGrantRef) string { return "sys/" + grant.Id }

func ActivityTopicGroupGrant(workspace model.WorkspaceRef, group model.GroupRef, grantID string) string {
	return ActivityTopicGroup(workspace, group) + "/" + grantID
}

func ActivityTopicWorkspaceStorageProvider(workspace model.WorkspaceRef) string {
	return workspace.Id
}

// ActivityEventAppend records a global activity event in the caller's transaction.
func (store *Store) ActivityEventAppend(ctx context.Context, transaction *sql.Tx, activity model.ActivityEvent, topics []string) (error, model.ActivityEvent) {
	if transaction == nil {
		return fmt.Errorf("append activity event: transaction is required"), model.ActivityEvent{}
	}
	err := validateActivityEvent(&activity, topics)
	if err != nil {
		return err, model.ActivityEvent{}
	}
	if activity.Ref.Id == "" {
		activity.Ref.Id, err = typed_id.New(typed_id.ActivityEvent)
		if err != nil {
			return fmt.Errorf("generate activity event ID: %w", err), model.ActivityEvent{}
		}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ActivityEvent, activity.Ref.Id)
	if err != nil {
		return fmt.Errorf("append activity event: ID is invalid"), model.ActivityEvent{}
	}
	activity.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	values := activityEventValues(activity)
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, activityEventInsertSQL(placeholder), values...); err != nil {
		return fmt.Errorf("insert activity event: %w", err), model.ActivityEvent{}
	}
	for _, topic := range topics {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`)
		`, activity.Ref.Id, topic); err != nil {
			return fmt.Errorf("insert activity event topic: %w", err), model.ActivityEvent{}
		}
	}
	return nil, activity
}

// ActivityEventAppendInTransaction records a global activity event in a DBOS transaction.
func (store *Store) ActivityEventAppendInTransaction(ctx context.Context, transaction dbos.Tx, activity model.ActivityEvent, topics []string) (error, model.ActivityEvent) {
	if transaction == nil {
		return fmt.Errorf("append activity event: transaction is required"), model.ActivityEvent{}
	}
	err := validateActivityEvent(&activity, topics)
	if err != nil {
		return err, model.ActivityEvent{}
	}
	if activity.Ref.Id == "" {
		activity.Ref.Id, err = typed_id.New(typed_id.ActivityEvent)
		if err != nil {
			return fmt.Errorf("generate activity event ID: %w", err), model.ActivityEvent{}
		}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ActivityEvent, activity.Ref.Id)
	if err != nil {
		return fmt.Errorf("append activity event: ID is invalid"), model.ActivityEvent{}
	}
	activity.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	values := activityEventValues(activity)
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.Exec(ctx, activityEventInsertSQL(placeholder), values...); err != nil {
		return fmt.Errorf("insert activity event: %w", err), model.ActivityEvent{}
	}
	for _, topic := range topics {
		if _, err := transaction.Exec(ctx, `
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`)
		`, activity.Ref.Id, topic); err != nil {
			return fmt.Errorf("insert activity event topic: %w", err), model.ActivityEvent{}
		}
	}
	return nil, activity
}

func activityEventInsertSQL(placeholder func(int) string) string {
	return `INSERT INTO gatehouse_activity_events (
		id, event, resource_kind, resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_project_record_schema, resource_project_record_attribute, resource_project_record, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at
	) VALUES (` + placeholder(1) + `, ` + placeholder(2) + `, ` + placeholder(3) + `, ` + placeholder(4) + `, ` + placeholder(5) + `, ` + placeholder(6) + `, ` + placeholder(7) + `, ` + placeholder(8) + `, ` + placeholder(9) + `, ` + placeholder(10) + `, ` + placeholder(11) + `, ` + placeholder(12) + `, ` + placeholder(13) + `, ` + placeholder(14) + `, ` + placeholder(15) + `, ` + placeholder(16) + `, ` + placeholder(17) + `, ` + placeholder(18) + `, ` + placeholder(19) + `, ` + placeholder(20) + `, ` + placeholder(21) + `, ` + placeholder(22) + `, ` + placeholder(23) + `, ` + placeholder(24) + `, ` + placeholder(25) + `, ` + placeholder(26) + `, ` + placeholder(27) + `, ` + placeholder(28) + `, ` + placeholder(29) + `, ` + placeholder(30) + `, ` + placeholder(31) + `, ` + placeholder(32) + `, ` + placeholder(33) + `, ` + placeholder(34) + `, ` + placeholder(35) + `, ` + placeholder(36) + `, ` + placeholder(37) + `)`
}

func activityEventValues(activity model.ActivityEvent) []any {
	value := func(target *string) any {
		if target == nil {
			return nil
		}
		return *target
	}
	version := func(target *int) any {
		if target == nil {
			return nil
		}
		return *target
	}
	return []any{activity.Ref.Id, activity.Event, activity.ResourceKind,
		value(activity.ResourceKeychainId), version(activity.ResourceKeychainVersion), value(activity.ResourceAgentProvider), value(activity.ResourceAgentModel), value(activity.ResourceGroup), value(activity.ResourceGroupMemberGroup), value(activity.ResourceGroupMemberPrincipal), value(activity.ResourceIdentity), value(activity.ResourcePrincipal), value(activity.ResourceProject), value(activity.ResourceProjectFile), value(activity.ResourceProjectGrant), value(activity.ResourceProjectNote), value(activity.ResourceProjectTask), value(activity.ResourceProjectSecret), value(activity.ResourceProjectRecordSchema), value(activity.ResourceProjectRecordAttribute), value(activity.ResourceProjectRecord), value(activity.ResourceSession), value(activity.ResourceSessionEvent), value(activity.ResourceSessionFile), value(activity.ResourceSessionGrant), value(activity.ResourceSessionNote), value(activity.ResourceSessionTask), value(activity.ResourceSessionSecret), value(activity.ResourceStorageProvider), value(activity.ResourceSystemGrant), value(activity.ResourceWorkspace), value(activity.ResourceWorkspaceAgentWorkspace), value(activity.ResourceWorkspaceAgentId), value(activity.ResourceWorkspaceGrant), value(activity.ResourceWorkspaceStorageProviderWorkspace), value(activity.ResourceWorkspaceStorageProviderProvider), activity.CreatedAt}
}

func validateActivityEvent(activity *model.ActivityEvent, topics []string) error {
	if strings.TrimSpace(activity.Event) == "" {
		return fmt.Errorf("append activity event: event is required")
	}
	if len(topics) == 0 {
		return fmt.Errorf("append activity event: at least one topic is required")
	}
	seenTopics := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if strings.TrimSpace(topic) == "" || strings.HasPrefix(topic, "/") || strings.HasSuffix(topic, "/") {
			return fmt.Errorf("append activity event: topic must be a nonblank hierarchy")
		}
		if _, exists := seenTopics[topic]; exists {
			return fmt.Errorf("append activity event: duplicate topic %q", topic)
		}
		seenTopics[topic] = struct{}{}
	}
	targets := []*string{activity.ResourceKeychainId, activity.ResourceAgentProvider, activity.ResourceAgentModel, activity.ResourceGroup, activity.ResourceGroupMemberGroup, activity.ResourceGroupMemberPrincipal, activity.ResourceIdentity, activity.ResourcePrincipal, activity.ResourceProject, activity.ResourceProjectFile, activity.ResourceProjectGrant, activity.ResourceProjectNote, activity.ResourceProjectTask, activity.ResourceProjectSecret, activity.ResourceProjectRecordSchema, activity.ResourceProjectRecordAttribute, activity.ResourceProjectRecord, activity.ResourceSession, activity.ResourceSessionEvent, activity.ResourceSessionFile, activity.ResourceSessionGrant, activity.ResourceSessionNote, activity.ResourceSessionTask, activity.ResourceSessionSecret, activity.ResourceStorageProvider, activity.ResourceSystemGrant, activity.ResourceWorkspace, activity.ResourceWorkspaceAgentWorkspace, activity.ResourceWorkspaceAgentId, activity.ResourceWorkspaceGrant, activity.ResourceWorkspaceStorageProviderWorkspace, activity.ResourceWorkspaceStorageProviderProvider}
	count := 0
	for _, target := range targets {
		if target != nil {
			count++
		}
	}
	if activity.ResourceKeychainVersion != nil {
		count++
	}
	require := func(expected int, values ...*string) error {
		if count != expected {
			return fmt.Errorf("append activity event: resource kind %q has invalid targets", activity.ResourceKind)
		}
		for _, value := range values {
			if value == nil || strings.TrimSpace(*value) == "" {
				return fmt.Errorf("append activity event: resource kind %q has an invalid target", activity.ResourceKind)
			}
		}
		return nil
	}
	switch activity.ResourceKind {
	case ActivityResourceKindKeychain:
		if activity.ResourceKeychainVersion == nil || *activity.ResourceKeychainVersion < 1 {
			return fmt.Errorf("append activity event: keychain target version is invalid")
		}
		return require(2, activity.ResourceKeychainId)
	case ActivityResourceKindGroupMember:
		return require(2, activity.ResourceGroupMemberGroup, activity.ResourceGroupMemberPrincipal)
	case ActivityResourceKindWorkspaceAgent:
		return require(2, activity.ResourceWorkspaceAgentWorkspace, activity.ResourceWorkspaceAgentId)
	case ActivityResourceKindWorkspaceStorageProvider:
		return require(2, activity.ResourceWorkspaceStorageProviderWorkspace, activity.ResourceWorkspaceStorageProviderProvider)
	case ActivityResourceKindAgentProvider:
		return require(1, activity.ResourceAgentProvider)
	case ActivityResourceKindAgentModel:
		return require(1, activity.ResourceAgentModel)
	case ActivityResourceKindGroup:
		return require(1, activity.ResourceGroup)
	case ActivityResourceKindIdentity:
		return require(1, activity.ResourceIdentity)
	case ActivityResourceKindPrincipal:
		return require(1, activity.ResourcePrincipal)
	case ActivityResourceKindProject:
		return require(1, activity.ResourceProject)
	case ActivityResourceKindProjectFile:
		return require(1, activity.ResourceProjectFile)
	case ActivityResourceKindProjectGrant:
		return require(1, activity.ResourceProjectGrant)
	case ActivityResourceKindProjectNote:
		return require(1, activity.ResourceProjectNote)
	case ActivityResourceKindProjectTask:
		return require(1, activity.ResourceProjectTask)
	case ActivityResourceKindProjectSecret:
		return require(1, activity.ResourceProjectSecret)
	case ActivityResourceKindProjectRecordSchema:
		return require(1, activity.ResourceProjectRecordSchema)
	case ActivityResourceKindProjectRecordAttribute:
		return require(1, activity.ResourceProjectRecordAttribute)
	case ActivityResourceKindProjectRecord:
		return require(1, activity.ResourceProjectRecord)
	case ActivityResourceKindSession:
		return require(1, activity.ResourceSession)
	case ActivityResourceKindSessionEvent:
		return require(1, activity.ResourceSessionEvent)
	case ActivityResourceKindSessionFile:
		return require(1, activity.ResourceSessionFile)
	case ActivityResourceKindSessionGrant:
		return require(1, activity.ResourceSessionGrant)
	case ActivityResourceKindSessionNote:
		return require(1, activity.ResourceSessionNote)
	case ActivityResourceKindSessionTask:
		return require(1, activity.ResourceSessionTask)
	case ActivityResourceKindSessionSecret:
		return require(1, activity.ResourceSessionSecret)
	case ActivityResourceKindStorageProvider:
		return require(1, activity.ResourceStorageProvider)
	case ActivityResourceKindSystemGrant:
		return require(1, activity.ResourceSystemGrant)
	case ActivityResourceKindWorkspace:
		return require(1, activity.ResourceWorkspace)
	case ActivityResourceKindWorkspaceGrant:
		return require(1, activity.ResourceWorkspaceGrant)
	default:
		return fmt.Errorf("append activity event: unsupported resource kind %q", activity.ResourceKind)
	}
}

// ActivityEventSelectorsNormalize expands exact names and terminal .* selectors into a stable identity.
func ActivityEventSelectorsNormalize(selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return nil, fmt.Errorf("activity event selectors are required")
	}
	known := make(map[string]struct{}, len(activityEventNames))
	for _, event := range activityEventNames {
		known[event] = struct{}{}
	}
	selected := map[string]struct{}{}
	for _, selector := range selectors {
		if _, ok := known[selector]; ok {
			selected[selector] = struct{}{}
			continue
		}
		if !strings.HasSuffix(selector, ".*") || strings.Count(selector, "*") != 1 || len(selector) == 2 {
			return nil, fmt.Errorf("invalid activity event selector %q", selector)
		}
		prefix := strings.TrimSuffix(selector, "*")
		matched := false
		for _, event := range activityEventNames {
			if strings.HasPrefix(event, prefix) {
				selected[event], matched = struct{}{}, true
			}
		}
		if !matched {
			return nil, fmt.Errorf("activity event selector %q matches no known events", selector)
		}
	}
	events := make([]string, 0, len(selected))
	for event := range selected {
		events = append(events, event)
	}
	sort.Strings(events)
	return events, nil
}

// ActivityTopicCheckpointsGet advances global topic checkpoints visible to principal.
func (store *Store) ActivityTopicCheckpointsGet(ctx context.Context, principal model.PrincipalRef, checkpoints []ActivityTopicCheckpoint) (error, []ActivityTopicCheckpoint) {
	if strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("get activity topic checkpoints: principal is required"), nil
	}
	if len(checkpoints) == 0 {
		return fmt.Errorf("get activity topic checkpoints: at least one topic is required"), nil
	}
	seen := map[string]struct{}{}
	for index := range checkpoints {
		checkpoint := &checkpoints[index]
		if strings.TrimSpace(checkpoint.Topic) == "" {
			return fmt.Errorf("get activity topic checkpoints: invalid topic %q", checkpoint.Topic), nil
		}
		if checkpoint.ID != "" && !typed_id.Valid(typed_id.ActivityEvent, checkpoint.ID) {
			return fmt.Errorf("get activity topic checkpoints: cursor ID is invalid"), nil
		}
		events, err := ActivityEventSelectorsNormalize(checkpoint.Events)
		if err != nil {
			return fmt.Errorf("get activity topic checkpoints: %w", err), nil
		}
		checkpoint.Events = events
		identity := checkpoint.Topic + "\x00" + strings.Join(events, "\x00")
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("get activity topic checkpoints: duplicate selector checkpoint"), nil
		}
		seen[identity] = struct{}{}
	}
	placeholder := keychainPlaceholder(store.kind)
	advanced := make([]ActivityTopicCheckpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		next := checkpoint
		authorized, err := store.activityTopicAuthorized(ctx, principal, checkpoint.Topic)
		if err != nil {
			return err, nil
		}
		if !authorized {
			advanced = append(advanced, next)
			continue
		}
		placeholders := make([]string, len(checkpoint.Events))
		arguments := make([]any, 0, len(checkpoint.Events)+1)
		arguments = append(arguments, checkpoint.ID)
		for index, event := range checkpoint.Events {
			placeholders[index], arguments = placeholder(index+2), append(arguments, event)
		}
		query := `SELECT topics.topic, activities.id
			FROM gatehouse_activity_event_topics AS topics
			JOIN gatehouse_activity_events AS activities ON activities.id = topics.activity
			WHERE topics.activity > ` + placeholder(1) + ` AND activities.event IN (` + strings.Join(placeholders, ", ") + `)
			ORDER BY topics.activity DESC`
		rows, err := store.QueryContext(ctx, query, arguments...)
		if err != nil {
			return fmt.Errorf("get activity topic checkpoint: %w", err), nil
		}
		for rows.Next() {
			var topic, id string
			if err := rows.Scan(&topic, &id); err != nil {
				rows.Close()
				return fmt.Errorf("scan activity topic checkpoint: %w", err), nil
			}
			if !activityTopicMatches(checkpoint.Topic, topic) {
				continue
			}
			next.ID = id
			break
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate activity topic checkpoint: %w", err), nil
		}
		rows.Close()
		advanced = append(advanced, next)
	}
	return nil, advanced
}

func activityTopicMatches(prefix, topic string) bool {
	return topic == prefix || strings.HasPrefix(topic, prefix+"/")
}

func (store *Store) activityTopicAuthorized(ctx context.Context, principal model.PrincipalRef, topic string) (bool, error) {
	root, _, _ := strings.Cut(topic, "/")
	if root == principal.Id {
		return true, nil
	}
	if root == "sys" {
		err, roles := store.SystemRolesGet(ctx, principal)
		if err != nil {
			return false, err
		}
		return authz.SystemAllows(roles, authz.SystemManage), nil
	}
	if !typed_id.Valid(typed_id.Workspace, root) {
		return false, nil
	}
	workspace := model.WorkspaceRef{Id: root}
	err, available := store.WorkspaceGet(ctx, workspace, principal)
	if err != nil {
		return false, fmt.Errorf("authorize activity topic workspace: %w", err)
	}
	return available != nil, nil
}
