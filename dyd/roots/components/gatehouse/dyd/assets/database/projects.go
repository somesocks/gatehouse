package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

func (store *Store) ProjectsCreate(ctx context.Context, project model.Project, creator model.PrincipalRef, groups []model.GroupRef) (error, model.Project) {
	if project.Name != nil {
		name := strings.Join(strings.Fields(*project.Name), " ")
		if name == "" {
			project.Name = nil
		} else {
			project.Name = &name
		}
	}
	createdAt, err := typed_id.Timestamp(typed_id.Project, project.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project: ID is invalid"), model.Project{}
	}
	if project.Description != nil {
		description := strings.TrimSpace(*project.Description)
		if description == "" {
			project.Description = nil
		} else {
			project.Description = &description
		}
	}
	project.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")

	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project creation: %w", err), model.Project{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_projects (workspace, id, name, description, enabled, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`)
	`, project.Ref.Workspace.Id, project.Ref.Id, project.Name, project.Description, project.Enabled, project.CreatedAt); err != nil {
		return fmt.Errorf("insert project: %w", err), model.Project{}
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_principal_grants (workspace, project, principal, enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, TRUE)
	`, project.Ref.Workspace.Id, project.Ref.Id, creator.Id); err != nil {
		return fmt.Errorf("grant project creator: %w", err), model.Project{}
	}
	for _, group := range groups {
		if group.Workspace != project.Ref.Workspace {
			return fmt.Errorf("grant project group: group belongs to another workspace"), model.Project{}
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO gatehouse_project_group_grants (workspace, project, "group", enabled)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, TRUE)
		`, project.Ref.Workspace.Id, project.Ref.Id, group.Id); err != nil {
			return fmt.Errorf("grant project group: %w", err), model.Project{}
		}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: project.Ref.Workspace},
		Event:        "project.create",
		ResourceKind: ActivityResourceKindProject,
		Project:      &project.Ref,
	}, []string{ActivityTopicProject(project.Ref)}); err != nil {
		return fmt.Errorf("append project creation activity: %w", err), model.Project{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project creation: %w", err), model.Project{}
	}
	return nil, project
}

func (store *Store) ProjectsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Project) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT projects.workspace, projects.id, projects.name, projects.description, projects.enabled, projects.created_at
		FROM gatehouse_projects AS projects
		WHERE projects.workspace = `+placeholder(1)+`
			AND projects.enabled = TRUE
			AND EXISTS (
				SELECT 1 FROM gatehouse_project_principal_grants AS grants
				WHERE grants.workspace = projects.workspace AND grants.project = projects.id
					AND grants.principal = `+placeholder(2)+` AND grants.enabled = TRUE
			)
			OR (
				projects.workspace = `+placeholder(3)+` AND projects.enabled = TRUE
				AND EXISTS (
					SELECT 1
					FROM gatehouse_project_group_grants AS grants
					JOIN gatehouse_groups AS groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					WHERE grants.workspace = projects.workspace AND grants.project = projects.id AND grants.enabled = TRUE
						AND groups.enabled = TRUE AND members.principal_id = `+placeholder(4)+` AND members.enabled = TRUE
				)
			)
		ORDER BY projects.id DESC
	`, workspace.Id, principal.Id, workspace.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get projects: %w", err), nil
	}
	defer rows.Close()
	projects := []model.Project{}
	for rows.Next() {
		var project model.Project
		var description sql.NullString
		if err := rows.Scan(&project.Ref.Workspace.Id, &project.Ref.Id, &project.Name, &description, &project.Enabled, &project.CreatedAt); err != nil {
			return fmt.Errorf("scan project: %w", err), nil
		}
		if description.Valid {
			project.Description = &description.String
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate projects: %w", err), nil
	}
	return nil, projects
}

func (store *Store) ProjectGet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef) (error, *model.Project) {
	err, projects := store.ProjectsGet(ctx, project.Workspace, principal)
	if err != nil {
		return err, nil
	}
	for _, candidate := range projects {
		if candidate.Ref == project {
			return nil, &candidate
		}
	}
	return nil, nil
}

func (store *Store) ProjectNameSet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef, name *string) (error, *model.Project) {
	if name != nil {
		value := strings.Join(strings.Fields(*name), " ")
		if value == "" {
			name = nil
		} else {
			name = &value
		}
	}
	err, stored := store.ProjectGet(ctx, project, principal)
	if err != nil || stored == nil {
		return err, stored
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_projects SET name = `+placeholder(1)+`
		WHERE workspace = `+placeholder(2)+` AND id = `+placeholder(3)+`
	`, name, project.Workspace.Id, project.Id); err != nil {
		return fmt.Errorf("update project name: %w", err), nil
	}
	stored.Name = name
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: project.Workspace},
		Event:        "project.update",
		ResourceKind: ActivityResourceKindProject,
		Project:      &project,
	}, []string{ActivityTopicProject(project)}); err != nil {
		return fmt.Errorf("append project update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project update: %w", err), nil
	}
	return nil, stored
}
