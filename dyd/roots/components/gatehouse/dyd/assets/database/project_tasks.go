package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

func (store *Store) ProjectTasksGet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef) (error, []model.ProjectTask) {
	if err, available := store.ProjectGet(ctx, project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []model.ProjectTask{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT id, title, sensitive, status, enabled,
			creator_principal, creator_agent, creator_gateway, created_at,
			updater_principal, updater_agent, updater_gateway, updated_at
		FROM gatehouse_project_tasks
		WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND enabled = TRUE
		ORDER BY created_at DESC, id DESC
	`, project.Workspace.Id, project.Id)
	if err != nil {
		return fmt.Errorf("get project tasks: %w", err), nil
	}
	defer rows.Close()
	tasks := []model.ProjectTask{}
	for rows.Next() {
		var task model.ProjectTask
		var creatorPrincipal, creatorAgent, creatorGateway, updaterPrincipal, updaterAgent, updaterGateway sql.NullString
		task.Ref.Project = project
		if err := rows.Scan(&task.Ref.Id, &task.Title, &task.Sensitive, &task.Status, &task.Enabled,
			&creatorPrincipal, &creatorAgent, &creatorGateway, &task.CreatedAt,
			&updaterPrincipal, &updaterAgent, &updaterGateway, &task.UpdatedAt); err != nil {
			return fmt.Errorf("scan project task: %w", err), nil
		}
		creator := noteAuthorFromValues(project.Workspace, creatorPrincipal, creatorAgent, creatorGateway)
		task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway = creator.Principal, creator.Agent, creator.Gateway
		updater := noteAuthorFromValues(project.Workspace, updaterPrincipal, updaterAgent, updaterGateway)
		task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project tasks: %w", err), nil
	}
	return nil, tasks
}

func (store *Store) ProjectTaskGet(ctx context.Context, task model.ProjectTaskRef, principal model.PrincipalRef) (error, *model.ProjectTask) {
	if !typed_id.Valid(typed_id.ProjectTask, task.Id) {
		return fmt.Errorf("get project task: ID is invalid"), nil
	}
	if err, project := store.ProjectGet(ctx, task.Project, principal); err != nil {
		return err, nil
	} else if project == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	result := &model.ProjectTask{Ref: task}
	var description sql.NullString
	var creatorPrincipal, creatorAgent, creatorGateway, updaterPrincipal, updaterAgent, updaterGateway sql.NullString
	err := store.QueryRowContext(ctx, `
		SELECT title, description, sensitive, status, enabled,
			creator_principal, creator_agent, creator_gateway, created_at,
			updater_principal, updater_agent, updater_gateway, updated_at
		FROM gatehouse_project_tasks
		WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND id = `+placeholder(3)+` AND enabled = TRUE
	`, task.Project.Workspace.Id, task.Project.Id, task.Id).Scan(&result.Title, &description, &result.Sensitive, &result.Status, &result.Enabled,
		&creatorPrincipal, &creatorAgent, &creatorGateway, &result.CreatedAt,
		&updaterPrincipal, &updaterAgent, &updaterGateway, &result.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get project task: %w", err), nil
	}
	if description.Valid {
		result.Description = &description.String
	}
	creator := noteAuthorFromValues(task.Project.Workspace, creatorPrincipal, creatorAgent, creatorGateway)
	result.CreatorPrincipal, result.CreatorAgent, result.CreatorGateway = creator.Principal, creator.Agent, creator.Gateway
	updater := noteAuthorFromValues(task.Project.Workspace, updaterPrincipal, updaterAgent, updaterGateway)
	result.UpdaterPrincipal, result.UpdaterAgent, result.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
	return nil, result
}

func (store *Store) ProjectTaskCreate(ctx context.Context, task model.ProjectTask, principal model.PrincipalRef) (error, model.ProjectTask) {
	if !typed_id.Valid(typed_id.ProjectTask, task.Ref.Id) || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("create project task: IDs are invalid"), model.ProjectTask{}
	}
	if err := normalizeTask(&task.Title, &task.Description, &task.Status); err != nil {
		return fmt.Errorf("create project task: %w", err), model.ProjectTask{}
	}
	if err, project := store.ProjectGet(ctx, task.Ref.Project, principal); err != nil {
		return err, model.ProjectTask{}
	} else if project == nil {
		return fmt.Errorf("create project task: project is unavailable"), model.ProjectTask{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ProjectTask, task.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project task: task ID is invalid"), model.ProjectTask{}
	}
	creator := TaskAuthor{Principal: task.CreatorPrincipal, Agent: task.CreatorAgent, Gateway: task.CreatorGateway}
	if taskAuthorEmpty(creator) {
		creator.Principal = &principal
	}
	updater := TaskAuthor{Principal: task.UpdaterPrincipal, Agent: task.UpdaterAgent, Gateway: task.UpdaterGateway}
	if taskAuthorEmpty(updater) {
		updater = creator
	}
	if err := noteAuthorValid(creator, task.Ref.Project.Workspace); err != nil {
		return fmt.Errorf("create project task: %w", err), model.ProjectTask{}
	}
	if err := noteAuthorValid(updater, task.Ref.Project.Workspace); err != nil {
		return fmt.Errorf("create project task: %w", err), model.ProjectTask{}
	}
	task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway = creator.Principal, creator.Agent, creator.Gateway
	task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
	task.Enabled = true
	task.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	task.UpdatedAt = task.CreatedAt
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project task creation: %w", err), model.ProjectTask{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	creatorPrincipal, creatorAgent, creatorGateway := noteAuthorValues(creator)
	updaterPrincipal, updaterAgent, updaterGateway := noteAuthorValues(updater)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_tasks (workspace, project, id, title, description, sensitive, status, enabled, creator_principal, creator_agent, creator_gateway, created_at, updater_principal, updater_agent, updater_gateway, updated_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, TRUE, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`, `+placeholder(12)+`, `+placeholder(13)+`, `+placeholder(14)+`, `+placeholder(15)+`)
	`, task.Ref.Project.Workspace.Id, task.Ref.Project.Id, task.Ref.Id, task.Title, task.Description, task.Sensitive, task.Status, creatorPrincipal, creatorAgent, creatorGateway, task.CreatedAt, updaterPrincipal, updaterAgent, updaterGateway, task.UpdatedAt); err != nil {
		return fmt.Errorf("insert project task: %w", err), model.ProjectTask{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_task.create", ResourceKind: ActivityResourceKindProjectTask, ResourceProjectTask: &task.Ref.Id}, []string{ActivityTopicProjectTask(task.Ref)}); err != nil {
		return fmt.Errorf("append project task creation activity: %w", err), model.ProjectTask{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project task creation: %w", err), model.ProjectTask{}
	}
	return nil, task
}

func (store *Store) ProjectTaskDetailsSetAs(ctx context.Context, task model.ProjectTaskRef, principal model.PrincipalRef, updater TaskAuthor, sensitive bool, status, title string, description *string) (error, *model.ProjectTask) {
	if err := normalizeTask(&title, &description, &status); err != nil {
		return fmt.Errorf("update project task: %w", err), nil
	}
	if taskAuthorEmpty(updater) {
		updater.Principal = &principal
	}
	if err := noteAuthorValid(updater, task.Project.Workspace); err != nil {
		return fmt.Errorf("update project task: %w", err), nil
	}
	err, current := store.ProjectTaskGet(ctx, task, principal)
	if err != nil || current == nil {
		return err, current
	}
	updatedAt := taskUpdatedAt()
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project task update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	updaterPrincipal, updaterAgent, updaterGateway := noteAuthorValues(updater)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_project_tasks SET title = `+placeholder(1)+`, description = `+placeholder(2)+`, sensitive = `+placeholder(3)+`, status = `+placeholder(4)+`, updater_principal = `+placeholder(5)+`, updater_agent = `+placeholder(6)+`, updater_gateway = `+placeholder(7)+`, updated_at = `+placeholder(8)+`
		WHERE workspace = `+placeholder(9)+` AND project = `+placeholder(10)+` AND id = `+placeholder(11)+` AND enabled = TRUE
	`, title, description, sensitive, status, updaterPrincipal, updaterAgent, updaterGateway, updatedAt, task.Project.Workspace.Id, task.Project.Id, task.Id)
	if err != nil {
		return fmt.Errorf("update project task: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update project task: %w", err), nil
	}
	if changed != 1 {
		return nil, nil
	}
	current.Title, current.Description, current.Sensitive, current.Status, current.UpdatedAt = title, description, sensitive, status, updatedAt
	current.UpdaterPrincipal, current.UpdaterAgent, current.UpdaterGateway = updater.Principal, updater.Agent, updater.Gateway
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_task.update", ResourceKind: ActivityResourceKindProjectTask, ResourceProjectTask: &task.Id}, []string{ActivityTopicProjectTask(task)}); err != nil {
		return fmt.Errorf("append project task update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project task update: %w", err), nil
	}
	return nil, current
}

func (store *Store) ProjectTaskRemove(ctx context.Context, task model.ProjectTaskRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.ProjectTask, task.Id) {
		return fmt.Errorf("remove project task: ID is invalid"), false
	}
	if err, project := store.ProjectGet(ctx, task.Project, principal); err != nil {
		return err, false
	} else if project == nil {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project task removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_project_tasks SET enabled = FALSE WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND id = `+placeholder(3)+` AND enabled = TRUE`, task.Project.Workspace.Id, task.Project.Id, task.Id)
	if err != nil {
		return fmt.Errorf("remove project task: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove project task: %w", err), false
	}
	if changed != 1 {
		return nil, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_task.remove", ResourceKind: ActivityResourceKindProjectTask, ResourceProjectTask: &task.Id}, []string{ActivityTopicProjectTask(task)}); err != nil {
		return fmt.Errorf("append project task removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project task removal: %w", err), false
	}
	return nil, true
}
