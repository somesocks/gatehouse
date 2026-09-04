package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gatehouse/authz"
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
	grantID, err := typed_id.New(typed_id.ProjectGrant)
	if err != nil {
		return fmt.Errorf("generate project creator grant ID: %w", err), model.Project{}
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_grants (id, workspace, project, role, principal, "group", enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, NULL, TRUE)
	`, grantID, project.Ref.Workspace.Id, project.Ref.Id, authz.Manager, creator.Id); err != nil {
		return fmt.Errorf("bind project creator role: %w", err), model.Project{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "project_grant.create",
		ResourceKind: ActivityResourceKindProjectGrant,
		ResourceProjectGrant: &grantID,
	}, []string{ActivityTopicProjectGrant(project.Ref, grantID), ActivityTopicPrincipalGrant(creator, grantID)}); err != nil {
		return fmt.Errorf("append project creator grant activity: %w", err), model.Project{}
	}
	for _, group := range groups {
		grantID, err := typed_id.New(typed_id.ProjectGrant)
		if err != nil {
			return fmt.Errorf("generate project group grant ID: %w", err), model.Project{}
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO gatehouse_project_grants (id, workspace, project, role, principal, "group", enabled)
			VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, NULL, `+placeholder(5)+`, TRUE)
		`, grantID, project.Ref.Workspace.Id, project.Ref.Id, authz.Manager, group.Id); err != nil {
			return fmt.Errorf("bind project group role: %w", err), model.Project{}
		}
		if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
			Ref:          model.ActivityEventRef{},
			Event:        "project_grant.create",
			ResourceKind: ActivityResourceKindProjectGrant,
			ResourceProjectGrant: &grantID,
		}, []string{ActivityTopicProjectGrant(project.Ref, grantID), ActivityTopicGroupGrant(project.Ref.Workspace, group, grantID)}); err != nil {
			return fmt.Errorf("append project group grant activity: %w", err), model.Project{}
		}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "project.create",
		ResourceKind: ActivityResourceKindProject,
		ResourceProject: &project.Ref.Id,
	}, []string{ActivityTopicProject(project.Ref)}); err != nil {
		return fmt.Errorf("append project creation activity: %w", err), model.Project{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project creation: %w", err), model.Project{}
	}
	return nil, project
}

func (store *Store) ProjectRolesGet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef) (error, []authz.Role) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH input AS (
			SELECT `+placeholder(1)+` AS workspace, `+placeholder(2)+` AS project, `+placeholder(3)+` AS principal
		)
		SELECT role FROM (
			SELECT grants.role
			FROM gatehouse_project_grants AS grants
			CROSS JOIN input
			JOIN gatehouse_projects AS projects
				ON projects.workspace = grants.workspace AND projects.id = grants.project
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = projects.workspace
			JOIN gatehouse_principals AS principals ON principals.id = grants.principal
			WHERE grants.workspace = input.workspace
				AND grants.project = input.project
				AND grants.principal = input.principal
				AND workspaces.enabled = TRUE
				AND projects.enabled = TRUE
				AND grants.enabled = TRUE
				AND principals.enabled = TRUE
			UNION
			SELECT grants.role
			FROM gatehouse_project_grants AS grants
			CROSS JOIN input
			JOIN gatehouse_projects AS projects
				ON projects.workspace = grants.workspace AND projects.id = grants.project
			JOIN gatehouse_workspaces AS workspaces ON workspaces.id = projects.workspace
			JOIN gatehouse_groups AS groups
				ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
			JOIN gatehouse_group_members AS members
				ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
			JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
			WHERE grants.workspace = input.workspace
				AND grants.project = input.project
				AND members.principal_id = input.principal
				AND workspaces.enabled = TRUE
				AND projects.enabled = TRUE
				AND grants.enabled = TRUE
				AND groups.enabled = TRUE
				AND members.enabled = TRUE
				AND principals.enabled = TRUE
		) AS roles
		ORDER BY role
	`, project.Workspace.Id, project.Id, principal.Id)
	if err != nil {
		return fmt.Errorf("get project roles: %w", err), nil
	}
	defer rows.Close()

	roles := []authz.Role{}
	for rows.Next() {
		var role authz.Role
		if err := rows.Scan(&role); err != nil {
			return fmt.Errorf("scan project role: %w", err), nil
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project roles: %w", err), nil
	}
	return nil, roles
}

func (store *Store) ProjectsGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef) (error, []model.Project) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT projects.workspace, projects.id, projects.name, projects.description, projects.enabled, projects.created_at
		FROM gatehouse_projects AS projects
		WHERE projects.workspace = `+placeholder(1)+`
			AND projects.enabled = TRUE
			AND EXISTS (
				SELECT 1 FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = projects.workspace AND workspaces.enabled = TRUE
			)
			AND (
				EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					WHERE grants.workspace = projects.workspace AND grants.project = projects.id
						AND grants.principal = `+placeholder(2)+` AND grants.enabled = TRUE
				)
				OR (
					projects.workspace = `+placeholder(3)+`
					AND EXISTS (
						SELECT 1
						FROM gatehouse_project_grants AS grants
						JOIN gatehouse_groups AS groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
						JOIN gatehouse_group_members AS members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
						WHERE grants.workspace = projects.workspace AND grants.project = projects.id AND grants.enabled = TRUE
							AND groups.enabled = TRUE AND members.principal_id = `+placeholder(4)+` AND members.enabled = TRUE
					)
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
	err, projects, _ := store.projectsSearch(ctx, project.Workspace, principal, ProjectSearch{Limit: 1}, project.Id)
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

func (store *Store) ProjectsGetByRefs(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, refs []model.ProjectRef) (error, []model.Project) {
	if len(refs) == 0 {
		return nil, []model.Project{}
	}
	placeholder := keychainPlaceholder(store.kind)
	ids := make([]string, 0, len(refs))
	placeholders := make([]string, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		if ref.Workspace != workspace {
			return fmt.Errorf("get projects by references: project belongs to another workspace"), nil
		}
		if !typed_id.Valid(typed_id.Project, ref.Id) {
			return fmt.Errorf("get projects by references: project ID is invalid"), nil
		}
		if _, exists := seen[ref.Id]; exists {
			continue
		}
		seen[ref.Id] = struct{}{}
		ids = append(ids, ref.Id)
		placeholders = append(placeholders, placeholder(len(ids)+1))
	}
	arguments := make([]any, 0, len(ids)+3)
	arguments = append(arguments, workspace.Id)
	for _, id := range ids {
		arguments = append(arguments, id)
	}
	principalPlaceholder := placeholder(len(arguments) + 1)
	arguments = append(arguments, principal.Id)
	groupPlaceholder := placeholder(len(arguments) + 1)
	arguments = append(arguments, principal.Id)
	rows, err := store.QueryContext(ctx, `
		SELECT projects.workspace, projects.id, projects.name, projects.description, projects.enabled, projects.created_at
		FROM gatehouse_projects AS projects
		WHERE projects.workspace = `+placeholder(1)+`
			AND projects.id IN (`+strings.Join(placeholders, ", ")+`)
			AND projects.enabled = TRUE
			AND EXISTS (
				SELECT 1 FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = projects.workspace AND workspaces.enabled = TRUE
			)
			AND (
				EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					WHERE grants.workspace = projects.workspace AND grants.project = projects.id
						AND grants.principal = `+principalPlaceholder+` AND grants.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					JOIN gatehouse_groups AS groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					WHERE grants.workspace = projects.workspace AND grants.project = projects.id AND grants.enabled = TRUE
						AND groups.enabled = TRUE AND members.principal_id = `+groupPlaceholder+` AND members.enabled = TRUE
				)
			)
		ORDER BY projects.id DESC
	`, arguments...)
	if err != nil {
		return fmt.Errorf("get projects by references: %w", err), nil
	}
	defer rows.Close()
	projects := []model.Project{}
	for rows.Next() {
		var project model.Project
		var description sql.NullString
		if err := rows.Scan(&project.Ref.Workspace.Id, &project.Ref.Id, &project.Name, &description, &project.Enabled, &project.CreatedAt); err != nil {
			return fmt.Errorf("scan project reference: %w", err), nil
		}
		if description.Valid {
			project.Description = &description.String
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project references: %w", err), nil
	}
	return nil, projects
}

func (store *Store) ProjectDetailsSet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef, name, description *string) (error, *model.Project) {
	if name != nil {
		value := strings.Join(strings.Fields(*name), " ")
		if value == "" {
			name = nil
		} else {
			name = &value
		}
	}
	if description != nil {
		value := strings.TrimSpace(*description)
		if value == "" {
			description = nil
		} else {
			description = &value
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
		UPDATE gatehouse_projects SET name = `+placeholder(1)+`, description = `+placeholder(2)+`
		WHERE workspace = `+placeholder(3)+` AND id = `+placeholder(4)+`
	`, name, description, project.Workspace.Id, project.Id); err != nil {
		return fmt.Errorf("update project details: %w", err), nil
	}
	stored.Name = name
	stored.Description = description
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "project.update",
		ResourceKind: ActivityResourceKindProject,
		ResourceProject: &project.Id,
	}, []string{ActivityTopicProject(project)}); err != nil {
		return fmt.Errorf("append project update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project update: %w", err), nil
	}
	return nil, stored
}

type ProjectSearch struct {
	Name   string
	Cursor string
	Limit  int
}

func (store *Store) ProjectsSearch(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, search ProjectSearch) (error, []model.Project, string) {
	return store.projectsSearch(ctx, workspace, principal, search, "")
}

func (store *Store) projectsSearch(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, search ProjectSearch, id string) (error, []model.Project, string) {
	if search.Limit < 1 || search.Limit > 100 {
		return fmt.Errorf("search projects: limit must be between 1 and 100"), nil, ""
	}
	if search.Cursor != "" && !typed_id.Valid(typed_id.Project, search.Cursor) {
		return fmt.Errorf("search projects: cursor is invalid"), nil, ""
	}
	if id != "" && !typed_id.Valid(typed_id.Project, id) {
		return fmt.Errorf("search projects: ID is invalid"), nil, ""
	}
	name := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(search.Name)) + "%"
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT projects.workspace, projects.id, projects.name, projects.description, projects.enabled, projects.created_at
		FROM gatehouse_projects AS projects
		WHERE projects.workspace = `+placeholder(1)+`
			AND projects.enabled = TRUE
			AND EXISTS (
				SELECT 1 FROM gatehouse_workspaces AS workspaces
				WHERE workspaces.id = projects.workspace AND workspaces.enabled = TRUE
			)
			AND LOWER(COALESCE(projects.name, '')) LIKE LOWER(`+placeholder(2)+`) ESCAPE '\'
			AND (`+placeholder(3)+` = '' OR projects.id < `+placeholder(4)+`)
			AND (
				EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					WHERE grants.workspace = projects.workspace AND grants.project = projects.id
						AND grants.principal = `+placeholder(5)+` AND grants.enabled = TRUE
				)
				OR EXISTS (
					SELECT 1 FROM gatehouse_project_grants AS grants
					JOIN gatehouse_groups AS groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
					JOIN gatehouse_group_members AS members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id
					WHERE grants.workspace = projects.workspace AND grants.project = projects.id AND grants.enabled = TRUE
						AND groups.enabled = TRUE AND members.principal_id = `+placeholder(6)+` AND members.enabled = TRUE
				)
			)
			AND (`+placeholder(7)+` = '' OR projects.id = `+placeholder(8)+`)
		ORDER BY projects.id DESC
		LIMIT `+placeholder(9)+`
	`, workspace.Id, name, search.Cursor, search.Cursor, principal.Id, principal.Id, id, id, search.Limit+1)
	if err != nil {
		return fmt.Errorf("search projects: %w", err), nil, ""
	}
	defer rows.Close()
	projects := []model.Project{}
	for rows.Next() {
		var project model.Project
		var description sql.NullString
		if err := rows.Scan(&project.Ref.Workspace.Id, &project.Ref.Id, &project.Name, &description, &project.Enabled, &project.CreatedAt); err != nil {
			return fmt.Errorf("scan project search result: %w", err), nil, ""
		}
		if description.Valid {
			project.Description = &description.String
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project search results: %w", err), nil, ""
	}
	if len(projects) <= search.Limit {
		return nil, projects, ""
	}
	nextCursor := projects[search.Limit-1].Ref.Id
	return nil, projects[:search.Limit], nextCursor
}
