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
	ErrGroupInput             = errors.New("invalid group administration input")
	ErrGroupAliasExists       = errors.New("group alias is already in use in this workspace")
	ErrGroupMemberExists      = errors.New("membership already exists")
	ErrGroupNotFound          = errors.New("group not found")
	ErrGroupPrincipalNotFound = errors.New("principal not found")
	ErrGroupLastAdmin         = errors.New("A group must retain at least one enabled admin. Promote another member to admin before demoting or removing this admin.")
)

type SystemGroup struct {
	ID        string     `json:"id"`
	Workspace string     `json:"workspace"`
	Alias     *string    `json:"alias,omitempty"`
	Name      *string    `json:"name,omitempty"`
	Enabled   bool       `json:"enabled"`
	Revision  int        `json:"revision"`
	Role      authz.Role `json:"role,omitempty"`
}

type SystemGroupMember struct {
	Principal        string     `json:"principal"`
	Alias            *string    `json:"alias,omitempty"`
	Name             *string    `json:"name,omitempty"`
	PrincipalEnabled bool       `json:"principal_enabled"`
	Enabled          bool       `json:"enabled"`
	Revision         int        `json:"revision"`
	Role             authz.Role `json:"role"`
}

type SystemGroupInput struct {
	Alias   *string
	Name    *string
	Enabled *bool
}

type SystemGroupSearch struct {
	Workspace  string
	Group      string
	Name       string
	Status     string
	Cursor     string
	Limit      int
	ActiveOnly bool
}

func groupSearchValues(search SystemGroupSearch, kind string) (string, error) {
	if search.Limit < 1 || search.Limit > 100 || len(strings.TrimSpace(search.Name)) > 256 ||
		(search.Cursor != "" && !typed_id.Valid(kind, search.Cursor)) ||
		(search.Status != "" && search.Status != "enabled" && search.Status != "disabled") {
		return "", fmt.Errorf("%w: invalid search parameters", ErrGroupInput)
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(search.Name)) + "%", nil
}

func (store *Store) SystemGroupsSearch(ctx context.Context, search SystemGroupSearch) (error, []SystemGroup, string) {
	pattern, err := groupSearchValues(search, typed_id.Group)
	if err != nil {
		return err, nil, ""
	}
	p := keychainPlaceholder(store.kind)
	query := `SELECT id, workspace_id, alias, name, enabled, revision FROM gatehouse_groups
		WHERE workspace_id = ` + p(1) + ` AND (LOWER(COALESCE(alias, '')) LIKE LOWER(` + p(2) + `) ESCAPE '\' OR LOWER(COALESCE(name, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\' OR id LIKE ` + p(4) + ` ESCAPE '\')
		AND (` + p(5) + ` = '' OR id < ` + p(6) + `)`
	args := []any{search.Workspace, pattern, pattern, pattern, search.Cursor, search.Cursor}
	if search.Status != "" {
		query += ` AND enabled = ` + p(len(args)+1)
		args = append(args, search.Status == "enabled")
	}
	query += ` ORDER BY id DESC LIMIT ` + p(len(args)+1)
	args = append(args, search.Limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	groups := []SystemGroup{}
	for rows.Next() {
		group, err := scanSystemGroup(rows)
		if err != nil {
			return err, nil, ""
		}
		groups = append(groups, *group)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(groups) > search.Limit {
		return nil, groups[:search.Limit], groups[search.Limit-1].ID
	}
	return nil, groups, ""
}

func scanSystemGroup(row interface{ Scan(...any) error }) (*SystemGroup, error) {
	var group SystemGroup
	var alias, name sql.NullString
	if err := row.Scan(&group.ID, &group.Workspace, &alias, &name, &group.Enabled, &group.Revision); err != nil {
		return nil, err
	}
	if alias.Valid {
		group.Alias = &alias.String
	}
	if name.Valid {
		group.Name = &name.String
	}
	return &group, nil
}

func (store *Store) SystemGroupGet(ctx context.Context, workspace, id string) (error, *SystemGroup) {
	if !typed_id.Valid(typed_id.Workspace, workspace) || !typed_id.Valid(typed_id.Group, id) {
		return nil, nil
	}
	p := keychainPlaceholder(store.kind)
	group, err := scanSystemGroup(store.QueryRowContext(ctx, `SELECT id, workspace_id, alias, name, enabled, revision FROM gatehouse_groups WHERE workspace_id = `+p(1)+` AND id = `+p(2), workspace, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return err, group
}

func validateSystemGroupInput(input SystemGroupInput, creating bool) error {
	if creating && input.Alias == nil {
		return fmt.Errorf("%w: alias is required", ErrGroupInput)
	}
	if input.Alias != nil && (len(*input.Alias) > 256 || !administrationAlias.MatchString(*input.Alias)) {
		return fmt.Errorf("%w: alias must start with a lowercase letter and contain only lowercase letters, numbers, underscores, or hyphens", ErrGroupInput)
	}
	if input.Name != nil && (len(*input.Name) > 256 || strings.ContainsRune(*input.Name, 0)) {
		return fmt.Errorf("%w: name must be at most 256 bytes and must not contain NUL", ErrGroupInput)
	}
	return nil
}

func (store *Store) SystemGroupCreate(ctx context.Context, workspace string, input SystemGroupInput) (error, *SystemGroup) {
	return store.GroupCreate(ctx, workspace, input, nil)
}

func (store *Store) GroupCreate(ctx context.Context, workspace string, input SystemGroupInput, actor *model.PrincipalRef) (error, *SystemGroup) {
	if !typed_id.Valid(typed_id.Workspace, workspace) {
		return nil, nil
	}
	if err := validateSystemGroupInput(input, true); err != nil {
		return err, nil
	}
	id, err := typed_id.New(typed_id.Group)
	if err != nil {
		return err, nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	if err := store.groupMutationAllowed(ctx, tx, workspace, "", actor); err != nil {
		return err, nil
	}
	p := keychainPlaceholder(store.kind)
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_workspaces WHERE id = `+p(1)+`)`, workspace).Scan(&exists); err != nil {
		return err, nil
	}
	if !exists {
		return nil, nil
	}
	enabled := input.Enabled == nil || *input.Enabled
	if _, err := tx.ExecContext(ctx, `INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled, revision) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, `+p(4)+`, `+p(5)+`, 1)`, workspace, id, *input.Alias, optionalStringValue(normalizeOptionalString(input.Name)), enabled); err != nil {
		if uniquePrincipalConstraint(err) {
			return ErrGroupAliasExists, nil
		}
		return err, nil
	}
	if err := store.systemGroupActivity(ctx, tx, workspace, id, "group.create"); err != nil {
		return err, nil
	}
	if actor != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision, role) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, TRUE, 1, 'admin')`, workspace, id, actor.Id); err != nil {
			return err, nil
		}
		if err := store.systemGroupMemberActivity(ctx, tx, workspace, id, actor.Id, "group_member.create"); err != nil {
			return err, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return store.SystemGroupGet(ctx, workspace, id)
}

func (store *Store) SystemGroupUpdate(ctx context.Context, workspace, id string, revision int, input SystemGroupInput) (error, *SystemGroup) {
	return store.GroupUpdate(ctx, workspace, id, revision, input, nil)
}

func (store *Store) GroupUpdate(ctx context.Context, workspace, id string, revision int, input SystemGroupInput, actor *model.PrincipalRef) (error, *SystemGroup) {
	if !typed_id.Valid(typed_id.Workspace, workspace) || !typed_id.Valid(typed_id.Group, id) {
		return nil, nil
	}
	if err := validateSystemGroupInput(input, false); err != nil {
		return err, nil
	}
	if revision < 1 || (input.Alias == nil && input.Name == nil && input.Enabled == nil) {
		return fmt.Errorf("%w: revision and an update are required", ErrGroupInput), nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	if err := store.groupMutationAllowed(ctx, tx, workspace, id, actor); err != nil {
		return err, nil
	}
	p := keychainPlaceholder(store.kind)
	group, err := scanSystemGroup(tx.QueryRowContext(ctx, `SELECT id, workspace_id, alias, name, enabled, revision FROM gatehouse_groups WHERE workspace_id = `+p(1)+` AND id = `+p(2), workspace, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	if group.Revision != revision {
		return ErrRevisionConflict, nil
	}
	alias, name, enabled := group.Alias, group.Name, group.Enabled
	if input.Alias != nil {
		alias = input.Alias
	}
	if input.Name != nil {
		name = normalizeOptionalString(input.Name)
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if !sameGroupString(alias, group.Alias) || !sameGroupString(name, group.Name) || enabled != group.Enabled {
		result, err := tx.ExecContext(ctx, `UPDATE gatehouse_groups SET alias = `+p(1)+`, name = `+p(2)+`, enabled = `+p(3)+`, revision = revision + 1 WHERE workspace_id = `+p(4)+` AND id = `+p(5)+` AND revision = `+p(6), optionalStringValue(alias), optionalStringValue(name), enabled, workspace, id, revision)
		if uniquePrincipalConstraint(err) {
			return ErrGroupAliasExists, nil
		}
		if err != nil {
			return err, nil
		}
		if err := groupUpdatedOnce(result); err != nil {
			return err, nil
		}
		if err := store.systemGroupActivity(ctx, tx, workspace, id, "group.update"); err != nil {
			return err, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return store.SystemGroupGet(ctx, workspace, id)
}

func sameGroupString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func groupUpdatedOnce(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (store *Store) SystemGroupMembersSearch(ctx context.Context, search SystemGroupSearch) (error, []SystemGroupMember, string) {
	pattern, err := groupSearchValues(search, typed_id.Principal)
	if err != nil {
		return err, nil, ""
	}
	p := keychainPlaceholder(store.kind)
	query := `SELECT principals.id, principals.alias, principals.name, principals.enabled, members.enabled, members.revision, members.role
		FROM gatehouse_group_members AS members JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
		WHERE members.workspace_id = ` + p(1) + ` AND members.group_id = ` + p(2) + `
		AND (LOWER(COALESCE(principals.alias, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\' OR LOWER(COALESCE(principals.name, '')) LIKE LOWER(` + p(4) + `) ESCAPE '\' OR principals.id LIKE ` + p(5) + ` ESCAPE '\')
		AND (` + p(6) + ` = '' OR principals.id < ` + p(7) + `)`
	args := []any{search.Workspace, search.Group, pattern, pattern, pattern, search.Cursor, search.Cursor}
	if search.ActiveOnly {
		query += ` AND members.enabled = TRUE AND principals.enabled = TRUE`
	} else if search.Status != "" {
		query += ` AND members.enabled = ` + p(len(args)+1)
		args = append(args, search.Status == "enabled")
	}
	query += ` ORDER BY principals.id DESC LIMIT ` + p(len(args)+1)
	args = append(args, search.Limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	members := []SystemGroupMember{}
	for rows.Next() {
		member, err := scanSystemGroupMember(rows)
		if err != nil {
			return err, nil, ""
		}
		members = append(members, *member)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(members) > search.Limit {
		return nil, members[:search.Limit], members[search.Limit-1].Principal
	}
	return nil, members, ""
}

func scanSystemGroupMember(row interface{ Scan(...any) error }) (*SystemGroupMember, error) {
	var member SystemGroupMember
	var alias, name sql.NullString
	if err := row.Scan(&member.Principal, &alias, &name, &member.PrincipalEnabled, &member.Enabled, &member.Revision, &member.Role); err != nil {
		return nil, err
	}
	if alias.Valid {
		member.Alias = &alias.String
	}
	if name.Valid {
		member.Name = &name.String
	}
	return &member, nil
}

func systemGroupExists(ctx context.Context, tx *sql.Tx, p func(int) string, workspace, group string) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_groups WHERE workspace_id = `+p(1)+` AND id = `+p(2)+`)`, workspace, group).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrGroupNotFound
	}
	return nil
}

func (store *Store) SystemGroupMemberCreate(ctx context.Context, workspace, group, principal string) error {
	return store.GroupMemberCreate(ctx, workspace, group, principal, authz.Member, nil)
}

func (store *Store) GroupMemberCreate(ctx context.Context, workspace, group, principal string, role authz.Role, actor *model.PrincipalRef) error {
	if !authz.ValidGroupRole(string(role)) {
		return fmt.Errorf("%w: role must be member or admin", ErrGroupInput)
	}
	if !typed_id.Valid(typed_id.Workspace, workspace) || !typed_id.Valid(typed_id.Group, group) {
		return ErrGroupNotFound
	}
	if !typed_id.Valid(typed_id.Principal, principal) {
		return fmt.Errorf("%w: invalid principal", ErrGroupInput)
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := store.groupMutationAllowed(ctx, tx, workspace, group, actor); err != nil {
		return err
	}
	p := keychainPlaceholder(store.kind)
	if err := systemGroupExists(ctx, tx, p, workspace, group); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM gatehouse_principals WHERE id = `+p(1)+`)`, principal).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrGroupPrincipalNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT `+workspacePrincipalAdmissionSQL(p(1), p(2)), workspace, principal).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: enabled workspace membership is required", ErrGroupInput)
	}
	var memberRevision int
	err = tx.QueryRowContext(ctx, `INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision, role) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, TRUE, 1, `+p(4)+`)
		ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET enabled = TRUE, role = excluded.role, revision = gatehouse_group_members.revision + 1
		WHERE gatehouse_group_members.enabled = FALSE RETURNING revision`, workspace, group, principal, role).Scan(&memberRevision)
	if err == sql.ErrNoRows {
		return ErrGroupMemberExists
	}
	if err != nil {
		return err
	}
	event := "group_member.create"
	if memberRevision > 1 {
		event = "group_member.update"
	}
	if err := store.systemGroupMemberActivity(ctx, tx, workspace, group, principal, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) SystemGroupMemberUpdate(ctx context.Context, workspace, group, principal string, enabled bool, revision int) error {
	return store.GroupMemberUpdate(ctx, workspace, group, principal, &enabled, nil, revision, nil)
}

func (store *Store) GroupMemberUpdate(ctx context.Context, workspace, group, principal string, enabled *bool, role *authz.Role, revision int, actor *model.PrincipalRef) error {
	if (enabled == nil && role == nil) || (role != nil && !authz.ValidGroupRole(string(*role))) {
		return fmt.Errorf("%w: enabled or a member/admin role is required", ErrGroupInput)
	}
	if !typed_id.Valid(typed_id.Workspace, workspace) || !typed_id.Valid(typed_id.Group, group) || !typed_id.Valid(typed_id.Principal, principal) {
		return ErrGroupNotFound
	}
	if revision < 1 {
		return fmt.Errorf("%w: a positive revision is required", ErrGroupInput)
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := store.lockGroupMembershipUpdate(ctx, tx, workspace, group); err != nil {
		return err
	}
	if err := store.groupMutationAllowed(ctx, tx, workspace, group, actor); err != nil {
		return err
	}
	p := keychainPlaceholder(store.kind)
	var current bool
	var currentRevision int
	var currentRole authz.Role
	err = tx.QueryRowContext(ctx, `SELECT enabled, revision, role FROM gatehouse_group_members WHERE workspace_id = `+p(1)+` AND group_id = `+p(2)+` AND principal_id = `+p(3), workspace, group, principal).Scan(&current, &currentRevision, &currentRole)
	if err == sql.ErrNoRows {
		return ErrGroupNotFound
	}
	if err != nil {
		return err
	}
	if currentRevision != revision {
		return ErrRevisionConflict
	}
	nextEnabled, nextRole := current, currentRole
	if enabled != nil {
		nextEnabled = *enabled
	}
	if role != nil {
		nextRole = *role
	}
	if nextEnabled != current || nextRole != currentRole {
		if current && currentRole == authz.Admin && (!nextEnabled || nextRole != authz.Admin) {
			var losesLastAdmin bool
			if err := tx.QueryRowContext(ctx, `WITH input AS (
				SELECT `+p(1)+` AS workspace, `+p(2)+` AS "group", `+p(3)+` AS principal
			) SELECT `+workspacePrincipalAdmissionSQL("input.workspace", "input.principal")+`
				AND EXISTS (SELECT 1 FROM gatehouse_groups groups
					WHERE groups.workspace_id = input.workspace AND groups.id = input."group" AND groups.enabled = TRUE)
				AND NOT EXISTS (SELECT 1 FROM gatehouse_group_members remaining
					WHERE remaining.workspace_id = input.workspace AND remaining.group_id = input."group" AND remaining.principal_id <> input.principal
					AND remaining.enabled = TRUE AND remaining.role = 'admin'
					AND `+workspacePrincipalAdmissionSQL("remaining.workspace_id", "remaining.principal_id")+`)
				FROM input`, workspace, group, principal).Scan(&losesLastAdmin); err != nil {
				return err
			}
			if losesLastAdmin {
				return ErrGroupLastAdmin
			}
		}
		if nextEnabled && (!current || (nextRole == authz.Admin && currentRole != authz.Admin)) {
			var admitted bool
			if err := tx.QueryRowContext(ctx, `SELECT `+workspacePrincipalAdmissionSQL(p(1), p(2)), workspace, principal).Scan(&admitted); err != nil {
				return err
			}
			if !admitted {
				return fmt.Errorf("%w: enabled workspace membership is required", ErrGroupInput)
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE gatehouse_group_members SET enabled = `+p(1)+`, role = `+p(2)+`, revision = revision + 1 WHERE workspace_id = `+p(3)+` AND group_id = `+p(4)+` AND principal_id = `+p(5)+` AND revision = `+p(6), nextEnabled, nextRole, workspace, group, principal, revision)
		if err != nil {
			return err
		}
		if err := groupUpdatedOnce(result); err != nil {
			return err
		}
		if err := store.systemGroupMemberActivity(ctx, tx, workspace, group, principal, "group_member.update"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Serialize membership edits on the group before reading the actor or roster.
// PostgreSQL's table lock also coordinates with configuration reconciliation;
// the no-op row write serializes separate group edits and SQLite connections
// without changing the group's revision or emitting a metadata activity event.
func (store *Store) lockGroupMembershipUpdate(ctx context.Context, tx *sql.Tx, workspace, group string) error {
	if store.kind == config.DatabaseKindPostgres {
		if _, err := tx.ExecContext(ctx, `LOCK TABLE gatehouse_groups, gatehouse_group_members IN ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
	}
	p := keychainPlaceholder(store.kind)
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_groups SET revision = revision WHERE workspace_id = `+p(1)+` AND id = `+p(2), workspace, group)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrGroupNotFound
	}
	return nil
}

func groupPrincipalTopics(ctx context.Context, tx *sql.Tx, p func(int) string, workspace, group, suffix string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT principal_id FROM gatehouse_group_members members WHERE workspace_id = `+p(1)+` AND group_id = `+p(2)+` AND members.enabled = TRUE AND `+workspacePrincipalAdmissionSQL("members.workspace_id", "members.principal_id"), workspace, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	topics := []string{}
	for rows.Next() {
		var principal string
		if err := rows.Scan(&principal); err != nil {
			return nil, err
		}
		topics = append(topics, principal+"/"+suffix)
	}
	return topics, rows.Err()
}

func (store *Store) systemGroupActivity(ctx context.Context, tx *sql.Tx, workspace, group, event string) error {
	topics, err := groupPrincipalTopics(ctx, tx, keychainPlaceholder(store.kind), workspace, group, group)
	if err != nil {
		return err
	}
	topics = append(topics, workspace+"/"+group)
	err, _ = store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindGroup, ResourceGroup: &group}, topics)
	return err
}

func (store *Store) systemGroupMemberActivity(ctx context.Context, tx *sql.Tx, workspace, group, principal, event string) error {
	topics, err := groupPrincipalTopics(ctx, tx, keychainPlaceholder(store.kind), workspace, group, group)
	if err != nil {
		return err
	}
	topics = append(topics, workspace+"/"+group)
	affected := principal + "/" + group
	found := false
	for _, topic := range topics {
		if topic == affected {
			found = true
			break
		}
	}
	if !found {
		topics = append(topics, affected)
	}
	err, _ = store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindGroupMember, ResourceGroupMemberGroup: &group, ResourceGroupMemberPrincipal: &principal}, topics)
	return err
}
