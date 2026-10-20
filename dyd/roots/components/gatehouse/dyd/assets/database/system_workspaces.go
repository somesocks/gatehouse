package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrWorkspaceAliasInvalid           = errors.New("workspace alias is invalid")
	ErrWorkspaceAliasExists            = errors.New("workspace alias already exists")
	ErrWorkspaceNameInvalid            = errors.New("workspace name is invalid")
	ErrWorkspaceGrantPrincipalNotFound = errors.New("workspace grant principal not found")
	ErrWorkspaceGrantAlreadyExists     = errors.New("workspace grant already exists")
	ErrWorkspaceGrantRoleInvalid       = errors.New("workspace grant role is invalid")
)

type SystemWorkspace struct {
	ID      string  `json:"id"`
	Alias   *string `json:"alias,omitempty"`
	Name    *string `json:"name,omitempty"`
	Enabled bool    `json:"enabled"`
}

type SystemWorkspaceSearch struct {
	Name   string
	Cursor string
	Limit  int
}

type SystemWorkspacePrincipal struct {
	ID      string                          `json:"id"`
	Alias   *string                         `json:"alias,omitempty"`
	Name    *string                         `json:"name,omitempty"`
	Enabled bool                            `json:"enabled"`
	Roles   []authz.Role                    `json:"roles"`
	Grants  []SystemWorkspacePrincipalGrant `json:"grants"`
}

type SystemWorkspacePrincipalOption struct {
	ID      string  `json:"id"`
	Alias   *string `json:"alias,omitempty"`
	Name    *string `json:"name,omitempty"`
	Enabled bool    `json:"enabled"`
}

type SystemWorkspacePrincipalSearch struct {
	WorkspaceID  string
	Name         string
	Cursor       string
	Limit        int
	AdmittedOnly bool
}

type SystemWorkspacePrincipalGrant struct {
	ID        string     `json:"id"`
	Workspace string     `json:"workspace"`
	Principal string     `json:"principal"`
	Role      authz.Role `json:"role"`
	Enabled   bool       `json:"enabled"`
	Revision  int        `json:"revision"`
}

func (store *Store) SystemWorkspacePrincipalsSearch(ctx context.Context, search SystemWorkspacePrincipalSearch) (error, []SystemWorkspacePrincipal, string) {
	if !typed_id.Valid(typed_id.Workspace, search.WorkspaceID) {
		return nil, nil, ""
	}
	if search.Limit < 1 || search.Limit > 100 {
		return fmt.Errorf("search system workspace principals: limit must be between 1 and 100"), nil, ""
	}
	if search.Cursor != "" && !typed_id.Valid(typed_id.Principal, search.Cursor) {
		return fmt.Errorf("search system workspace principals: cursor is invalid"), nil, ""
	}
	query := strings.TrimSpace(search.Name)
	if len(query) > 256 {
		return fmt.Errorf("search system workspace principals: query is too long"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		WITH page AS (
			SELECT principals.id
			FROM gatehouse_principals AS principals
			WHERE EXISTS (
				SELECT 1 FROM gatehouse_workspace_grants AS grants
				WHERE grants.workspace = `+placeholder(1)+`
					AND grants.principal = principals.id
					AND grants.enabled = TRUE
			)
				AND (
					LOWER(COALESCE(principals.alias, '')) LIKE LOWER(`+placeholder(2)+`) ESCAPE '\'
					OR LOWER(COALESCE(principals.name, '')) LIKE LOWER(`+placeholder(3)+`) ESCAPE '\'
					OR LOWER(principals.id) LIKE LOWER(`+placeholder(4)+`) ESCAPE '\'
				)
				AND (`+placeholder(5)+` = '' OR principals.id < `+placeholder(6)+`)
			ORDER BY principals.id DESC
			LIMIT `+placeholder(7)+`
		)
		SELECT principals.id, principals.alias, principals.name, principals.enabled, grants.id, grants.role, grants.enabled, grants.revision
		FROM page
		JOIN gatehouse_principals AS principals ON principals.id = page.id
		JOIN gatehouse_workspace_grants AS grants
			ON grants.workspace = `+placeholder(8)+`
			AND grants.principal = principals.id
			AND grants.enabled = TRUE
		ORDER BY principals.id DESC, grants.role
	`, search.WorkspaceID, pattern, pattern, pattern, search.Cursor, search.Cursor, search.Limit+1, search.WorkspaceID)
	if err != nil {
		return fmt.Errorf("search system workspace principals: %w", err), nil, ""
	}
	defer rows.Close()

	principals := []SystemWorkspacePrincipal{}
	var nextCursor string
	for rows.Next() {
		var principal SystemWorkspacePrincipal
		var alias, name sql.NullString
		grant := SystemWorkspacePrincipalGrant{Workspace: search.WorkspaceID}
		if err := rows.Scan(&principal.ID, &alias, &name, &principal.Enabled, &grant.ID, &grant.Role, &grant.Enabled, &grant.Revision); err != nil {
			return fmt.Errorf("scan system workspace principal: %w", err), nil, ""
		}
		grant.Principal = principal.ID
		if len(principals) > 0 && principals[len(principals)-1].ID == principal.ID {
			principals[len(principals)-1].Roles = append(principals[len(principals)-1].Roles, grant.Role)
			principals[len(principals)-1].Grants = append(principals[len(principals)-1].Grants, grant)
			continue
		}
		if len(principals) == search.Limit {
			nextCursor = principals[len(principals)-1].ID
			break
		}
		if alias.Valid {
			principal.Alias = &alias.String
		}
		if name.Valid {
			principal.Name = &name.String
		}
		principal.Roles = []authz.Role{grant.Role}
		principal.Grants = []SystemWorkspacePrincipalGrant{grant}
		principals = append(principals, principal)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate system workspace principals: %w", err), nil, ""
	}
	return nil, principals, nextCursor
}

func (store *Store) SystemWorkspacePrincipalOptionsSearch(ctx context.Context, search SystemWorkspacePrincipalSearch) (error, []SystemWorkspacePrincipalOption, string) {
	if !typed_id.Valid(typed_id.Workspace, search.WorkspaceID) {
		return nil, nil, ""
	}
	if search.Limit < 1 || search.Limit > 100 {
		return fmt.Errorf("search system workspace principal options: limit must be between 1 and 100"), nil, ""
	}
	if search.Cursor != "" && !typed_id.Valid(typed_id.Principal, search.Cursor) {
		return fmt.Errorf("search system workspace principal options: cursor is invalid"), nil, ""
	}
	query := strings.TrimSpace(search.Name)
	if len(query) > 256 {
		return fmt.Errorf("search system workspace principal options: query is too long"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	placeholder := store.sessionParticipantPlaceholder
	filter := ""
	args := []any{pattern, pattern, pattern, search.Cursor, search.Cursor, search.Limit + 1}
	if search.AdmittedOnly {
		filter = ` AND ` + workspacePrincipalAdmissionSQL(placeholder(7), "principals.id")
		args = append(args, search.WorkspaceID)
	}
	rows, err := store.QueryContext(ctx, `
		SELECT principals.id, principals.alias, principals.name, principals.enabled
		FROM gatehouse_principals AS principals
		WHERE (
			LOWER(COALESCE(principals.alias, '')) LIKE LOWER(`+placeholder(1)+`) ESCAPE '\'
			OR LOWER(COALESCE(principals.name, '')) LIKE LOWER(`+placeholder(2)+`) ESCAPE '\'
			OR LOWER(principals.id) LIKE LOWER(`+placeholder(3)+`) ESCAPE '\'
		)
			AND (`+placeholder(4)+` = '' OR principals.id < `+placeholder(5)+`)
		`+filter+` ORDER BY principals.id DESC
		LIMIT `+placeholder(6)+`
	`, args...)
	if err != nil {
		return fmt.Errorf("search system workspace principal options: %w", err), nil, ""
	}
	defer rows.Close()

	principals := []SystemWorkspacePrincipalOption{}
	for rows.Next() {
		var principal SystemWorkspacePrincipalOption
		var alias, name sql.NullString
		if err := rows.Scan(&principal.ID, &alias, &name, &principal.Enabled); err != nil {
			return fmt.Errorf("scan system workspace principal option: %w", err), nil, ""
		}
		if alias.Valid {
			principal.Alias = &alias.String
		}
		if name.Valid {
			principal.Name = &name.String
		}
		principals = append(principals, principal)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate system workspace principal options: %w", err), nil, ""
	}
	if len(principals) <= search.Limit {
		return nil, principals, ""
	}
	nextCursor := principals[search.Limit-1].ID
	return nil, principals[:search.Limit], nextCursor
}

func (store *Store) SystemWorkspacePrincipalGrantCreate(ctx context.Context, workspaceID, principalID string, role authz.Role) (error, *SystemWorkspacePrincipalGrant) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) {
		return nil, nil
	}
	if !authz.ValidRole(string(role)) {
		return ErrWorkspaceGrantRoleInvalid, nil
	}
	if !typed_id.Valid(typed_id.Principal, principalID) {
		return nil, nil
	}
	grantID, err := typed_id.New(typed_id.WorkspaceGrant)
	if err != nil {
		return fmt.Errorf("generate workspace grant ID: %w", err), nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace grant creation: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var workspaceExists, principalExists bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_workspaces WHERE id = `+placeholder(1)+`)`, workspaceID).Scan(&workspaceExists); err != nil {
		return fmt.Errorf("check workspace grant workspace: %w", err), nil
	}
	if !workspaceExists {
		return nil, nil
	}
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_principals WHERE id = `+placeholder(1)+`)`, principalID).Scan(&principalExists); err != nil {
		return fmt.Errorf("check workspace grant principal: %w", err), nil
	}
	if !principalExists {
		return ErrWorkspaceGrantPrincipalNotFound, nil
	}

	grant := SystemWorkspacePrincipalGrant{ID: grantID, Workspace: workspaceID, Principal: principalID, Role: role, Enabled: true, Revision: 1}
	err = transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, NULL, TRUE, 1)
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE
		SET enabled = TRUE, revision = gatehouse_workspace_grants.revision + 1
		WHERE gatehouse_workspace_grants.enabled = FALSE
		RETURNING id, revision
	`, grant.ID, grant.Workspace, grant.Role, grant.Principal).Scan(&grant.ID, &grant.Revision)
	if err == sql.ErrNoRows {
		return ErrWorkspaceGrantAlreadyExists, nil
	}
	if err != nil {
		return fmt.Errorf("insert or re-enable workspace grant: %w", err), nil
	}

	event := "workspace_grant.create"
	if grant.Revision > 1 {
		event = "workspace_grant.update"
	}
	workspaceRef := model.WorkspaceRef{Id: workspaceID}
	principalRef := model.PrincipalRef{Id: principalID}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Event:                  event,
		ResourceKind:           ActivityResourceKindWorkspaceGrant,
		ResourceWorkspaceGrant: &grant.ID,
	}, []string{ActivityTopicWorkspaceGrant(workspaceRef, grant.ID), ActivityTopicPrincipalGrant(principalRef, grant.ID)}); err != nil {
		return fmt.Errorf("append workspace grant activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit workspace grant creation: %w", err), nil
	}
	return nil, &grant
}

func (store *Store) SystemWorkspacePrincipalGrantsRemove(ctx context.Context, workspaceID, principalID string) (error, bool) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.Principal, principalID) {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace grant removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var workspaceExists, principalExists bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_workspaces WHERE id = `+placeholder(1)+`)`, workspaceID).Scan(&workspaceExists); err != nil {
		return fmt.Errorf("check workspace for grant removal: %w", err), false
	}
	if !workspaceExists {
		return nil, false
	}
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_principals WHERE id = `+placeholder(1)+`)`, principalID).Scan(&principalExists); err != nil {
		return fmt.Errorf("check principal for grant removal: %w", err), false
	}
	if !principalExists {
		return nil, false
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT id FROM gatehouse_workspace_grants
		WHERE workspace = `+placeholder(1)+` AND principal = `+placeholder(2)+` AND enabled = TRUE
	`, workspaceID, principalID)
	if err != nil {
		return fmt.Errorf("get workspace grants for removal: %w", err), false
	}
	grants := []string{}
	for rows.Next() {
		var grantID string
		if err := rows.Scan(&grantID); err != nil {
			rows.Close()
			return fmt.Errorf("scan workspace grant for removal: %w", err), false
		}
		grants = append(grants, grantID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate workspace grants for removal: %w", err), false
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace grants for removal: %w", err), false
	}
	workspaceRef := model.WorkspaceRef{Id: workspaceID}
	principalRef := model.PrincipalRef{Id: principalID}
	for _, grantID := range grants {
		// A concurrent role edit may have advanced the revision since the read.
		if _, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspace_grants SET enabled = FALSE, revision = revision + 1 WHERE id = `+placeholder(1), grantID); err != nil {
			return fmt.Errorf("disable workspace grant: %w", err), false
		}
		if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
			Event:                  "workspace_grant.update",
			ResourceKind:           ActivityResourceKindWorkspaceGrant,
			ResourceWorkspaceGrant: &grantID,
		}, []string{ActivityTopicWorkspaceGrant(workspaceRef, grantID), ActivityTopicPrincipalGrant(principalRef, grantID)}); err != nil {
			return fmt.Errorf("append workspace grant removal activity: %w", err), false
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit workspace grant removal: %w", err), false
	}
	return nil, true
}

func (store *Store) SystemWorkspacesSearch(ctx context.Context, search SystemWorkspaceSearch) (error, []SystemWorkspace, string) {
	if search.Limit < 1 || search.Limit > 100 {
		return fmt.Errorf("search system workspaces: limit must be between 1 and 100"), nil, ""
	}
	if search.Cursor != "" && !typed_id.Valid(typed_id.Workspace, search.Cursor) {
		return fmt.Errorf("search system workspaces: cursor is invalid"), nil, ""
	}
	query := strings.TrimSpace(search.Name)
	if len(query) > 256 {
		return fmt.Errorf("search system workspaces: query is too long"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT id, alias, name, enabled
		FROM gatehouse_workspaces
		WHERE (
			LOWER(COALESCE(alias, '')) LIKE LOWER(`+placeholder(1)+`) ESCAPE '\'
			OR LOWER(COALESCE(name, '')) LIKE LOWER(`+placeholder(2)+`) ESCAPE '\'
			OR LOWER(id) LIKE LOWER(`+placeholder(3)+`) ESCAPE '\'
		)
			AND (`+placeholder(4)+` = '' OR id < `+placeholder(5)+`)
		ORDER BY id DESC
		LIMIT `+placeholder(6)+`
	`, pattern, pattern, pattern, search.Cursor, search.Cursor, search.Limit+1)
	if err != nil {
		return fmt.Errorf("search system workspaces: %w", err), nil, ""
	}
	defer rows.Close()

	workspaces := []SystemWorkspace{}
	for rows.Next() {
		var workspace SystemWorkspace
		var alias, name sql.NullString
		if err := rows.Scan(&workspace.ID, &alias, &name, &workspace.Enabled); err != nil {
			return fmt.Errorf("scan system workspace: %w", err), nil, ""
		}
		if alias.Valid {
			workspace.Alias = &alias.String
		}
		if name.Valid {
			workspace.Name = &name.String
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate system workspaces: %w", err), nil, ""
	}
	if len(workspaces) <= search.Limit {
		return nil, workspaces, ""
	}
	nextCursor := workspaces[search.Limit-1].ID
	return nil, workspaces[:search.Limit], nextCursor
}

func (store *Store) SystemWorkspaceGet(ctx context.Context, id string) (error, *SystemWorkspace) {
	if !typed_id.Valid(typed_id.Workspace, id) {
		return nil, nil
	}
	var workspace SystemWorkspace
	var alias, name sql.NullString
	err := store.QueryRowContext(ctx, `SELECT id, alias, name, enabled FROM gatehouse_workspaces WHERE id = `+keychainPlaceholder(store.kind)(1), id).Scan(&workspace.ID, &alias, &name, &workspace.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system workspace: %w", err), nil
	}
	if alias.Valid {
		workspace.Alias = &alias.String
	}
	if name.Valid {
		workspace.Name = &name.String
	}
	return nil, &workspace
}

func (store *Store) SystemWorkspaceUpdate(ctx context.Context, id string, name *string, enabled *bool) (error, *SystemWorkspace) {
	if !typed_id.Valid(typed_id.Workspace, id) {
		return nil, nil
	}
	if name == nil && enabled == nil {
		return fmt.Errorf("update system workspace: no fields supplied"), nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system workspace update: %w", err), nil
	}
	defer transaction.Rollback()

	workspace := SystemWorkspace{}
	var alias, storedName sql.NullString
	placeholder := keychainPlaceholder(store.kind)
	err = transaction.QueryRowContext(ctx, `SELECT id, alias, name, enabled FROM gatehouse_workspaces WHERE id = `+placeholder(1), id).Scan(&workspace.ID, &alias, &storedName, &workspace.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system workspace for update: %w", err), nil
	}
	if alias.Valid {
		workspace.Alias = &alias.String
	}
	if storedName.Valid {
		workspace.Name = &storedName.String
	}
	changed := false
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			changed = storedName.Valid
			workspace.Name = nil
		} else if !storedName.Valid || storedName.String != trimmed {
			changed = true
			workspace.Name = &trimmed
		}
	}
	if enabled != nil && workspace.Enabled != *enabled {
		changed = true
		workspace.Enabled = *enabled
	}
	if !changed {
		if err := transaction.Commit(); err != nil {
			return fmt.Errorf("commit unchanged system workspace: %w", err), nil
		}
		return nil, &workspace
	}
	if _, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspaces SET name = `+placeholder(1)+`, enabled = `+placeholder(2)+` WHERE id = `+placeholder(3), optionalStringValue(workspace.Name), workspace.Enabled, id); err != nil {
		return fmt.Errorf("update system workspace: %w", err), nil
	}
	workspaceID := workspace.ID
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Event:             "workspace.update",
		ResourceKind:      ActivityResourceKindWorkspace,
		ResourceWorkspace: &workspaceID,
	}, []string{ActivityTopicWorkspace(model.WorkspaceRef{Id: workspaceID})}); err != nil {
		return fmt.Errorf("append system workspace update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system workspace update: %w", err), nil
	}
	return nil, &workspace
}

func (store *Store) SystemWorkspaceCreate(ctx context.Context, alias string, name *string, enabled bool) (error, *SystemWorkspace) {
	if !config.ValidWorkspaceAlias(alias) {
		return ErrWorkspaceAliasInvalid, nil
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return ErrWorkspaceNameInvalid, nil
		}
		name = &trimmed
	}
	id, err := typed_id.New(typed_id.Workspace)
	if err != nil {
		return fmt.Errorf("generate workspace ID: %w", err), nil
	}

	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system workspace creation: %w", err), nil
	}
	defer transaction.Rollback()

	workspace := SystemWorkspace{ID: id, Alias: &alias, Name: name, Enabled: enabled}
	placeholder := keychainPlaceholder(store.kind)
	err = transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_workspaces (id, alias, name, enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`)
		ON CONFLICT (alias) DO NOTHING
		RETURNING id
	`, id, alias, optionalStringValue(name), enabled).Scan(&workspace.ID)
	if err == sql.ErrNoRows {
		return ErrWorkspaceAliasExists, nil
	}
	if err != nil {
		return fmt.Errorf("insert system workspace: %w", err), nil
	}

	workspaceRef := model.WorkspaceRef{Id: id}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Event:             "workspace.create",
		ResourceKind:      ActivityResourceKindWorkspace,
		ResourceWorkspace: &id,
	}, []string{ActivityTopicWorkspace(workspaceRef)}); err != nil {
		return fmt.Errorf("append system workspace creation activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system workspace creation: %w", err), nil
	}
	return nil, &workspace
}
