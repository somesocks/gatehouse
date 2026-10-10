package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrWorkspaceAliasInvalid = errors.New("workspace alias is invalid")
	ErrWorkspaceAliasExists  = errors.New("workspace alias already exists")
	ErrWorkspaceNameInvalid  = errors.New("workspace name is invalid")
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
