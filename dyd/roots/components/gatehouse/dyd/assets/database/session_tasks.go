package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

func (store *Store) SessionTasksGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, []model.SessionTask) {
	if err, available := store.SessionGet(ctx, session, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []model.SessionTask{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT id, title, sensitive, status, enabled,
			creator_principal, creator_agent, creator_gateway, created_at,
			updater_principal, updater_agent, updater_gateway, updated_at
		FROM gatehouse_session_tasks
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND enabled = TRUE
		ORDER BY created_at DESC, id DESC
	`, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("get session tasks: %w", err), nil
	}
	defer rows.Close()
	tasks := []model.SessionTask{}
	for rows.Next() {
		var task model.SessionTask
		var creatorPrincipal, creatorAgent, creatorGateway, updaterPrincipal, updaterAgent, updaterGateway sql.NullString
		task.Ref.Session = session
		if err := rows.Scan(&task.Ref.Id, &task.Title, &task.Sensitive, &task.Status, &task.Enabled,
			&creatorPrincipal, &creatorAgent, &creatorGateway, &task.CreatedAt,
			&updaterPrincipal, &updaterAgent, &updaterGateway, &task.UpdatedAt); err != nil {
			return fmt.Errorf("scan session task: %w", err), nil
		}
		creator := noteAuthorFromValues(session.Workspace, creatorPrincipal, creatorAgent, creatorGateway)
		task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway = creator.Principal, creator.Agent, creator.Gateway
		updater := noteAuthorFromValues(session.Workspace, updaterPrincipal, updaterAgent, updaterGateway)
		task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session tasks: %w", err), nil
	}
	return nil, tasks
}

func (store *Store) SessionTaskGet(ctx context.Context, task model.SessionTaskRef, principal model.PrincipalRef) (error, *model.SessionTask) {
	if !typed_id.Valid(typed_id.SessionTask, task.Id) {
		return fmt.Errorf("get session task: ID is invalid"), nil
	}
	if err, session := store.SessionGet(ctx, task.Session, principal); err != nil {
		return err, nil
	} else if session == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	result := &model.SessionTask{Ref: task}
	var description sql.NullString
	var creatorPrincipal, creatorAgent, creatorGateway, updaterPrincipal, updaterAgent, updaterGateway sql.NullString
	err := store.QueryRowContext(ctx, `
		SELECT title, description, sensitive, status, enabled,
			creator_principal, creator_agent, creator_gateway, created_at,
			updater_principal, updater_agent, updater_gateway, updated_at
		FROM gatehouse_session_tasks
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND id = `+placeholder(3)+` AND enabled = TRUE
	`, task.Session.Workspace.Id, task.Session.Id, task.Id).Scan(&result.Title, &description, &result.Sensitive, &result.Status, &result.Enabled,
		&creatorPrincipal, &creatorAgent, &creatorGateway, &result.CreatedAt,
		&updaterPrincipal, &updaterAgent, &updaterGateway, &result.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get session task: %w", err), nil
	}
	if description.Valid {
		result.Description = &description.String
	}
	creator := noteAuthorFromValues(task.Session.Workspace, creatorPrincipal, creatorAgent, creatorGateway)
	result.CreatorPrincipal, result.CreatorAgent, result.CreatorGateway = creator.Principal, creator.Agent, creator.Gateway
	updater := noteAuthorFromValues(task.Session.Workspace, updaterPrincipal, updaterAgent, updaterGateway)
	result.UpdaterPrincipal, result.UpdaterAgent, result.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
	return nil, result
}

func (store *Store) SessionTaskCreate(ctx context.Context, task model.SessionTask, principal model.PrincipalRef) (error, model.SessionTask) {
	if !typed_id.Valid(typed_id.SessionTask, task.Ref.Id) || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("create session task: IDs are invalid"), model.SessionTask{}
	}
	if err := normalizeTask(&task.Title, &task.Description, &task.Status); err != nil {
		return fmt.Errorf("create session task: %w", err), model.SessionTask{}
	}
	if err, session := store.SessionGet(ctx, task.Ref.Session, principal); err != nil {
		return err, model.SessionTask{}
	} else if session == nil {
		return fmt.Errorf("create session task: session is unavailable"), model.SessionTask{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.SessionTask, task.Ref.Id)
	if err != nil {
		return fmt.Errorf("create session task: task ID is invalid"), model.SessionTask{}
	}
	creator := TaskAuthor{Principal: task.CreatorPrincipal, Agent: task.CreatorAgent, Gateway: task.CreatorGateway}
	if taskAuthorEmpty(creator) {
		creator.Principal = &principal
	}
	updater := TaskAuthor{Principal: task.UpdaterPrincipal, Agent: task.UpdaterAgent, Gateway: task.UpdaterGateway}
	if taskAuthorEmpty(updater) {
		updater = creator
	}
	if err := noteAuthorValid(creator, task.Ref.Session.Workspace); err != nil {
		return fmt.Errorf("create session task: %w", err), model.SessionTask{}
	}
	if err := noteAuthorValid(updater, task.Ref.Session.Workspace); err != nil {
		return fmt.Errorf("create session task: %w", err), model.SessionTask{}
	}
	task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway = creator.Principal, creator.Agent, creator.Gateway
	task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
	task.Enabled = true
	task.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	task.UpdatedAt = task.CreatedAt
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session task creation: %w", err), model.SessionTask{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	creatorPrincipal, creatorAgent, creatorGateway := noteAuthorValues(creator)
	updaterPrincipal, updaterAgent, updaterGateway := noteAuthorValues(updater)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_tasks (workspace, session, id, title, description, sensitive, status, enabled, creator_principal, creator_agent, creator_gateway, created_at, updater_principal, updater_agent, updater_gateway, updated_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, TRUE, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`, `+placeholder(12)+`, `+placeholder(13)+`, `+placeholder(14)+`, `+placeholder(15)+`)
	`, task.Ref.Session.Workspace.Id, task.Ref.Session.Id, task.Ref.Id, task.Title, task.Description, task.Sensitive, task.Status, creatorPrincipal, creatorAgent, creatorGateway, task.CreatedAt, updaterPrincipal, updaterAgent, updaterGateway, task.UpdatedAt); err != nil {
		return fmt.Errorf("insert session task: %w", err), model.SessionTask{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "session_task.create", ResourceKind: ActivityResourceKindSessionTask, ResourceSessionTask: &task.Ref.Id}, []string{ActivityTopicSessionTask(task.Ref)}); err != nil {
		return fmt.Errorf("append session task creation activity: %w", err), model.SessionTask{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session task creation: %w", err), model.SessionTask{}
	}
	return nil, task
}

func (store *Store) SessionTaskDetailsSetAs(ctx context.Context, task model.SessionTaskRef, principal model.PrincipalRef, updater TaskAuthor, sensitive bool, status, title string, description *string) (error, *model.SessionTask) {
	if err := normalizeTask(&title, &description, &status); err != nil {
		return fmt.Errorf("update session task: %w", err), nil
	}
	if taskAuthorEmpty(updater) {
		updater.Principal = &principal
	}
	if err := noteAuthorValid(updater, task.Session.Workspace); err != nil {
		return fmt.Errorf("update session task: %w", err), nil
	}
	err, current := store.SessionTaskGet(ctx, task, principal)
	if err != nil || current == nil {
		return err, current
	}
	updatedAt := taskUpdatedAt()
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session task update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	updaterPrincipal, updaterAgent, updaterGateway := noteAuthorValues(updater)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_session_tasks SET title = `+placeholder(1)+`, description = `+placeholder(2)+`, sensitive = `+placeholder(3)+`, status = `+placeholder(4)+`, updater_principal = `+placeholder(5)+`, updater_agent = `+placeholder(6)+`, updater_gateway = `+placeholder(7)+`, updated_at = `+placeholder(8)+`
		WHERE workspace = `+placeholder(9)+` AND session = `+placeholder(10)+` AND id = `+placeholder(11)+` AND enabled = TRUE
	`, title, description, sensitive, status, updaterPrincipal, updaterAgent, updaterGateway, updatedAt, task.Session.Workspace.Id, task.Session.Id, task.Id)
	if err != nil {
		return fmt.Errorf("update session task: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update session task: %w", err), nil
	}
	if changed != 1 {
		return nil, nil
	}
	current.Title, current.Description, current.Sensitive, current.Status, current.UpdatedAt = title, description, sensitive, status, updatedAt
	current.UpdaterPrincipal, current.UpdaterAgent, current.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "session_task.update", ResourceKind: ActivityResourceKindSessionTask, ResourceSessionTask: &task.Id}, []string{ActivityTopicSessionTask(task)}); err != nil {
		return fmt.Errorf("append session task update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session task update: %w", err), nil
	}
	return nil, current
}

func (store *Store) SessionTaskRemove(ctx context.Context, task model.SessionTaskRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.SessionTask, task.Id) {
		return fmt.Errorf("remove session task: ID is invalid"), false
	}
	if err, session := store.SessionGet(ctx, task.Session, principal); err != nil {
		return err, false
	} else if session == nil {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session task removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_session_tasks SET enabled = FALSE WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND id = `+placeholder(3)+` AND enabled = TRUE`, task.Session.Workspace.Id, task.Session.Id, task.Id)
	if err != nil {
		return fmt.Errorf("remove session task: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove session task: %w", err), false
	}
	if changed != 1 {
		return nil, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "session_task.remove", ResourceKind: ActivityResourceKindSessionTask, ResourceSessionTask: &task.Id}, []string{ActivityTopicSessionTask(task)}); err != nil {
		return fmt.Errorf("append session task removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session task removal: %w", err), false
	}
	return nil, true
}
