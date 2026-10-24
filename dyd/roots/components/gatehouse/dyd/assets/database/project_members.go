package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gatehouse/authz"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrProjectMemberInput     = errors.New("invalid project access input")
	ErrProjectMemberNotFound  = errors.New("project or recipient is unavailable")
	ErrProjectMemberForbidden = errors.New("project admin access is required")
	ErrProjectGrantExists     = errors.New("this role is already assigned to the recipient")
	ErrProjectLastAdmin       = errors.New("the project must retain an enabled admin with workspace access")
)

type ProjectGrant struct {
	ID        string     `json:"id"`
	Principal *string    `json:"principal,omitempty"`
	Group     *string    `json:"group,omitempty"`
	Role      authz.Role `json:"role"`
	Enabled   bool       `json:"enabled"`
	Revision  int        `json:"revision"`
}

type ProjectMember struct {
	ID      string         `json:"id"`
	Alias   *string        `json:"alias,omitempty"`
	Name    *string        `json:"name,omitempty"`
	Enabled bool           `json:"enabled"`
	Grants  []ProjectGrant `json:"grants"`
}

type ProjectMemberSearch struct {
	Kind   string
	Name   string
	Cursor string
	Limit  int
}

func projectMemberSubject(kind string) (table, column string, err error) {
	switch kind {
	case "principal":
		return "gatehouse_principals", "principal", nil
	case "group":
		return "gatehouse_groups", `"group"`, nil
	default:
		return "", "", ErrProjectMemberInput
	}
}

func projectMemberEligibleSQL(kind, workspace, subject string) string {
	if kind == "group" {
		return `EXISTS (SELECT 1 FROM gatehouse_groups eligible_group WHERE eligible_group.workspace_id = ` + workspace + ` AND eligible_group.id = ` + subject + ` AND eligible_group.enabled = TRUE)`
	}
	return workspacePrincipalAdmissionSQL(workspace, subject)
}

// The roster lists explicit grants. Group-derived access is represented by the
// group grant rather than materialized as an independently editable person.
func (store *Store) ProjectMembersSearch(ctx context.Context, project model.ProjectRef, actor model.PrincipalRef, search ProjectMemberSearch, options bool) (error, []ProjectMember, string) {
	table, column, err := projectMemberSubject(search.Kind)
	if err != nil {
		return err, nil, ""
	}
	pattern, err := sessionParticipantSearchValues(SessionParticipantSearch{Kind: search.Kind, Name: search.Name, Cursor: search.Cursor, Limit: search.Limit})
	if err != nil {
		return ErrProjectMemberInput, nil, ""
	}
	if err, available := store.ProjectGet(ctx, project, actor); err != nil {
		return err, nil, ""
	} else if available == nil {
		return ErrProjectMemberNotFound, nil, ""
	}
	if options {
		if err, roles := store.ProjectRolesGet(ctx, project, actor); err != nil {
			return err, nil, ""
		} else if !authz.ProjectAllows(roles, authz.ProjectGrantManage) {
			return ErrProjectMemberForbidden, nil, ""
		}
	}
	p := store.sessionParticipantPlaceholder
	scope := ""
	if search.Kind == "group" {
		scope = ` AND subjects.workspace_id = ` + p(1)
	}
	filter := `EXISTS (SELECT 1 FROM gatehouse_project_grants grants WHERE grants.workspace = ` + p(1) + ` AND grants.project = ` + p(2) + ` AND grants.enabled = TRUE AND grants.` + column + ` = subjects.id)`
	if options {
		filter = projectMemberEligibleSQL(search.Kind, p(1), "subjects.id")
	}
	query := `WITH page AS (
		SELECT subjects.id, subjects.alias, subjects.name, subjects.enabled FROM ` + table + ` subjects
		WHERE ` + filter + scope + ` AND (
			LOWER(COALESCE(subjects.name, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\'
			OR LOWER(COALESCE(subjects.alias, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\')
		AND (` + p(4) + ` = '' OR subjects.id < ` + p(4) + `) ORDER BY subjects.id DESC LIMIT ` + p(5) + `
	) SELECT page.id, page.alias, page.name, page.enabled`
	if options {
		query += ` FROM page WHERE ` + p(2) + ` <> '' ORDER BY page.id DESC`
	} else {
		query += `, grants.id, grants.role, grants.revision FROM page JOIN gatehouse_project_grants grants
			ON grants.` + column + ` = page.id AND grants.workspace = ` + p(1) + ` AND grants.project = ` + p(2) + ` AND grants.enabled = TRUE
			ORDER BY page.id DESC, grants.role, grants.id`
	}
	rows, err := store.QueryContext(ctx, query, project.Workspace.Id, project.Id, pattern, search.Cursor, search.Limit+1)
	if err != nil {
		return fmt.Errorf("search project members: %w", err), nil, ""
	}
	defer rows.Close()
	items := []ProjectMember{}
	for rows.Next() {
		var item ProjectMember
		var alias, name sql.NullString
		grant := ProjectGrant{Enabled: true}
		values := []any{&item.ID, &alias, &name, &item.Enabled}
		if !options {
			values = append(values, &grant.ID, &grant.Role, &grant.Revision)
		}
		if err := rows.Scan(values...); err != nil {
			return err, nil, ""
		}
		if alias.Valid {
			item.Alias = &alias.String
		}
		if name.Valid {
			item.Name = &name.String
		}
		item.Grants = []ProjectGrant{}
		if len(items) == 0 || items[len(items)-1].ID != item.ID {
			items = append(items, item)
		}
		if !options {
			member := &items[len(items)-1]
			if search.Kind == "principal" {
				grant.Principal = &member.ID
			} else {
				grant.Group = &member.ID
			}
			member.Grants = append(member.Grants, grant)
		}
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(items) > search.Limit {
		return nil, items[:search.Limit], items[search.Limit-1].ID
	}
	return nil, items, ""
}

// Lock the project before checking authorization and the admin set. Competing
// admin removals are serialized on PostgreSQL and SQLite.
func (store *Store) projectGrantTransaction(ctx context.Context, project model.ProjectRef, actor model.PrincipalRef) (*sql.Tx, error) {
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	p := store.sessionParticipantPlaceholder
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_projects SET enabled = enabled WHERE workspace = `+p(1)+` AND id = `+p(2)+` AND enabled = TRUE`, project.Workspace.Id, project.Id)
	if err == nil {
		var count int64
		count, err = result.RowsAffected()
		if err == nil && count == 0 {
			err = ErrProjectMemberNotFound
		}
	}
	if err == nil {
		var allowed bool
		err = tx.QueryRowContext(ctx, `SELECT `+workspacePrincipalAdmissionSQL(p(1), p(3))+` AND EXISTS (
			SELECT 1 FROM gatehouse_project_grants grants WHERE grants.workspace = `+p(1)+` AND grants.project = `+p(2)+` AND grants.role = 'admin' AND grants.enabled = TRUE AND (
			grants.principal = `+p(3)+` OR EXISTS (SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_members gm ON gm.workspace_id = g.workspace_id AND gm.group_id = g.id
			WHERE g.workspace_id = grants.workspace AND g.id = grants."group" AND g.enabled = TRUE AND gm.enabled = TRUE AND gm.principal_id = `+p(3)+`)))`, project.Workspace.Id, project.Id, actor.Id).Scan(&allowed)
		if err == nil && !allowed {
			err = ErrProjectMemberForbidden
		}
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (store *Store) projectGrantSubjectEligible(ctx context.Context, tx *sql.Tx, project model.ProjectRef, principal, group *string) error {
	kind, subject := "principal", principal
	if group != nil {
		kind, subject = "group", group
	}
	p := store.sessionParticipantPlaceholder
	var eligible bool
	if err := tx.QueryRowContext(ctx, `SELECT `+projectMemberEligibleSQL(kind, p(1), p(2)), project.Workspace.Id, *subject).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return ErrProjectMemberNotFound
	}
	return nil
}

func (store *Store) ProjectGrantCreate(ctx context.Context, project model.ProjectRef, actor model.PrincipalRef, principal, group *string, role authz.Role) (error, *ProjectGrant) {
	if (principal == nil) == (group == nil) || !authz.ValidRole(string(role)) || (principal != nil && !typed_id.Valid(typed_id.Principal, *principal)) || (group != nil && !typed_id.Valid(typed_id.Group, *group)) {
		return ErrProjectMemberInput, nil
	}
	tx, err := store.projectGrantTransaction(ctx, project, actor)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	if err := store.projectGrantSubjectEligible(ctx, tx, project, principal, group); err != nil {
		return err, nil
	}
	id, err := typed_id.New(typed_id.ProjectGrant)
	if err != nil {
		return err, nil
	}
	grant := ProjectGrant{ID: id, Principal: principal, Group: group, Role: role, Enabled: true, Revision: 1}
	column := "principal"
	if group != nil {
		column = `"group"`
	}
	p := store.sessionParticipantPlaceholder
	err = tx.QueryRowContext(ctx, `INSERT INTO gatehouse_project_grants (id, workspace, project, principal, "group", role, enabled, revision)
		VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, `+p(4)+`, `+p(5)+`, `+p(6)+`, TRUE, 1)
		ON CONFLICT (workspace, project, role, `+column+`) WHERE `+column+` IS NOT NULL DO UPDATE
		SET enabled = TRUE, revision = gatehouse_project_grants.revision + 1 WHERE gatehouse_project_grants.enabled = FALSE
		RETURNING id, revision`, id, project.Workspace.Id, project.Id, principal, group, role).Scan(&grant.ID, &grant.Revision)
	if err == sql.ErrNoRows {
		return ErrProjectGrantExists, nil
	}
	if err != nil {
		return err, nil
	}
	event := "project_grant.create"
	if grant.Revision > 1 {
		event = "project_grant.update"
	}
	if err := store.projectGrantActivity(ctx, tx, project, grant, event); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return nil, &grant
}

func (store *Store) ProjectGrantUpdate(ctx context.Context, project model.ProjectRef, actor model.PrincipalRef, id string, role *authz.Role, enabled *bool, revision int) (error, *ProjectGrant) {
	if !typed_id.Valid(typed_id.ProjectGrant, id) || revision < 1 || (role == nil && enabled == nil) || (role != nil && !authz.ValidRole(string(*role))) {
		return ErrProjectMemberInput, nil
	}
	tx, err := store.projectGrantTransaction(ctx, project, actor)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := store.sessionParticipantPlaceholder
	var grant ProjectGrant
	var principal, group sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, principal, "group", role, enabled, revision FROM gatehouse_project_grants WHERE workspace = `+p(1)+` AND project = `+p(2)+` AND id = `+p(3), project.Workspace.Id, project.Id, id).Scan(&grant.ID, &principal, &group, &grant.Role, &grant.Enabled, &grant.Revision)
	if err == sql.ErrNoRows {
		return ErrProjectMemberNotFound, nil
	}
	if err != nil {
		return err, nil
	}
	if grant.Revision != revision {
		return ErrRevisionConflict, nil
	}
	if principal.Valid {
		grant.Principal = &principal.String
	}
	if group.Valid {
		grant.Group = &group.String
	}
	nextRole, nextEnabled := grant.Role, grant.Enabled
	if role != nil {
		nextRole = *role
	}
	if enabled != nil {
		nextEnabled = *enabled
	}
	if nextRole == grant.Role && nextEnabled == grant.Enabled {
		return nil, &grant
	}
	if !grant.Enabled && nextEnabled {
		if err := store.projectGrantSubjectEligible(ctx, tx, project, grant.Principal, grant.Group); err != nil {
			return err, nil
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE gatehouse_project_grants SET role = `+p(1)+`, enabled = `+p(2)+`, revision = revision + 1 WHERE id = `+p(3), nextRole, nextEnabled, id)
	if uniquePrincipalConstraint(err) {
		return ErrProjectGrantExists, nil
	}
	if err != nil {
		return err, nil
	}
	if grant.Enabled && grant.Role == authz.Admin && (!nextEnabled || nextRole != authz.Admin) {
		var admin bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM gatehouse_project_grants grants
			WHERE grants.workspace = `+p(1)+` AND grants.project = `+p(2)+` AND grants.enabled = TRUE AND grants.role = 'admin' AND (
			`+workspacePrincipalAdmissionSQL("grants.workspace", "grants.principal")+`
			OR EXISTS (SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_members gm ON gm.workspace_id = g.workspace_id AND gm.group_id = g.id
			WHERE g.workspace_id = grants.workspace AND g.id = grants."group" AND g.enabled = TRUE AND gm.enabled = TRUE
			AND `+workspacePrincipalAdmissionSQL("grants.workspace", "gm.principal_id")+`)))`, project.Workspace.Id, project.Id).Scan(&admin)
		if err != nil {
			return err, nil
		}
		if !admin {
			return ErrProjectLastAdmin, nil
		}
	}
	grant.Role, grant.Enabled, grant.Revision = nextRole, nextEnabled, grant.Revision+1
	if err := store.projectGrantActivity(ctx, tx, project, grant, "project_grant.update"); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return nil, &grant
}

func (store *Store) projectGrantActivity(ctx context.Context, tx *sql.Tx, project model.ProjectRef, grant ProjectGrant, event string) error {
	topics := []string{ActivityTopicProjectGrant(project, grant.ID)}
	if grant.Principal != nil {
		topics = append(topics, ActivityTopicPrincipalGrant(model.PrincipalRef{Id: *grant.Principal}, grant.ID))
	} else {
		topics = append(topics, ActivityTopicGroupGrant(project.Workspace, model.GroupRef{Id: *grant.Group}, grant.ID))
		members, err := groupPrincipalTopics(ctx, tx, keychainPlaceholder(store.kind), project.Workspace.Id, *grant.Group, grant.ID)
		if err != nil {
			return err
		}
		topics = append(topics, members...)
	}
	err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindProjectGrant, ResourceProjectGrant: &grant.ID}, topics)
	return err
}
