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
	ErrSessionParticipantInput     = errors.New("invalid chat participant input")
	ErrSessionParticipantNotFound  = errors.New("chat or participant is unavailable")
	ErrSessionParticipantForbidden = errors.New("chat manager access is required")
	ErrSessionGrantExists          = errors.New("this role is already assigned to the participant")
	ErrSessionLastManager          = errors.New("the chat must retain an enabled manager")
)

type SessionParticipantGrant struct {
	ID        string     `json:"id"`
	Principal *string    `json:"principal,omitempty"`
	Group     *string    `json:"group,omitempty"`
	Agent     *string    `json:"agent,omitempty"`
	Role      authz.Role `json:"role"`
	Enabled   bool       `json:"enabled"`
	Revision  int        `json:"revision"`
}

// SessionParticipantGroupSource describes inherited participation without
// exposing the source group's grant as an editable grant of this participant.
type SessionParticipantGroupSource struct {
	Group string     `json:"group"`
	Role  authz.Role `json:"role"`
}

type SessionParticipant struct {
	ID            string                          `json:"id"`
	Alias         *string                         `json:"alias,omitempty"`
	Name          *string                         `json:"name,omitempty"`
	Enabled       bool                            `json:"enabled"`
	Grants        []SessionParticipantGrant       `json:"grants"`
	InheritedFrom []SessionParticipantGroupSource `json:"inherited_from,omitempty"`
}

type SessionParticipantSearch struct {
	Kind    string
	Name    string
	Cursor  string
	Limit   int
	Outside bool
}

func sessionParticipantSearchValues(search SessionParticipantSearch) (string, error) {
	kind := typed_id.Principal
	if search.Kind == "group" {
		kind = typed_id.Group
	} else if search.Kind == "agent" {
		kind = typed_id.WorkspaceAgent
	} else if search.Kind != "principal" {
		return "", ErrSessionParticipantInput
	}
	if search.Limit < 1 || search.Limit > 100 || len(strings.TrimSpace(search.Name)) > 256 || (search.Cursor != "" && !typed_id.Valid(kind, search.Cursor)) {
		return "", ErrSessionParticipantInput
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(search.Name)) + "%", nil
}

// These queries reuse parameters. Number SQLite parameters explicitly rather
// than using anonymous placeholders, which would require repeated arguments.
func (store *Store) sessionParticipantPlaceholder(index int) string {
	if store.kind == config.DatabaseKindSQLite {
		return fmt.Sprintf("?%d", index)
	}
	return fmt.Sprintf("$%d", index)
}

// SessionParticipantsSearch lists explicit people/groups and effective bots.
// Grants contain only direct grants; inherited bot participation is separate
// read-only provenance, including for clients that do not understand it yet.
func (store *Store) SessionParticipantsSearch(ctx context.Context, session model.SessionRef, actor model.PrincipalRef, search SessionParticipantSearch, options bool) (error, []SessionParticipant, string) {
	pattern, err := sessionParticipantSearchValues(search)
	if err != nil {
		return err, nil, ""
	}
	if err, available := store.SessionGet(ctx, session, actor); err != nil {
		return err, nil, ""
	} else if available == nil {
		return ErrSessionParticipantNotFound, nil, ""
	}
	if options {
		if err, roles := store.SessionRolesGet(ctx, session, actor); err != nil {
			return err, nil, ""
		} else if !authz.SessionAllows(roles, authz.SessionGrantManage) {
			return ErrSessionParticipantForbidden, nil, ""
		}
	}
	p := store.sessionParticipantPlaceholder
	table, column, scope := "gatehouse_principals", "principal", ""
	name := "subjects.name"
	if search.Kind == "group" {
		table, column = "gatehouse_groups", `"group"`
		scope = ` AND subjects.workspace_id = ` + p(1)
	} else if search.Kind == "agent" {
		table, column, name = "gatehouse_workspace_agents", "agent", "subjects.label"
		scope = ` AND subjects.workspace_id = ` + p(1)
	}
	prefix := `WITH `
	filter := `EXISTS (SELECT 1 FROM effective_grants WHERE effective_grants.subject_id = subjects.id)`
	if options {
		filter, err = chatInvitationEligibilitySQL(search.Kind, p(1), p(6), "subjects.id")
		if err != nil {
			return err, nil, ""
		}
	} else {
		prefix += `effective_grants AS (
			SELECT grants.id, grants.role, grants.revision, grants.` + column + ` AS subject_id, NULL AS source_group, FALSE AS inherited
			FROM gatehouse_session_grants grants WHERE grants.workspace = ` + p(1) + ` AND grants.session = ` + p(2) + ` AND grants.enabled = TRUE AND grants.` + column + ` IS NOT NULL`
		if search.Kind == "agent" {
			prefix += ` UNION ALL
				SELECT grants.id, grants.role, grants.revision, members.agent_id, grants."group", TRUE
				FROM gatehouse_session_grants grants
				JOIN gatehouse_groups groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group" AND groups.enabled = TRUE
				JOIN gatehouse_group_agents members ON members.workspace_id = groups.workspace_id AND members.group_id = groups.id AND members.enabled = TRUE
				WHERE grants.workspace = ` + p(1) + ` AND grants.session = ` + p(2) + ` AND grants.enabled = TRUE`
		}
		prefix += `), `
	}
	query := prefix + `page AS (
		SELECT subjects.id, subjects.alias, ` + name + ` AS name, subjects.enabled FROM ` + table + ` subjects
		WHERE ` + filter + scope + ` AND (
			LOWER(COALESCE(` + name + `, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\'
			OR LOWER(COALESCE(subjects.alias, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\')
		AND (` + p(4) + ` = '' OR subjects.id < ` + p(4) + `) ORDER BY subjects.id DESC LIMIT ` + p(5) + `
	) SELECT page.id, page.alias, page.name, page.enabled`
	if !options {
		query += `, grants.id, grants.role, grants.revision, grants.source_group, grants.inherited FROM page JOIN effective_grants grants
			ON grants.subject_id = page.id ORDER BY page.id DESC, grants.role, grants.id`
	} else {
		// Keep the session placeholder bound on PostgreSQL as well as SQLite.
		query += ` FROM page WHERE ` + p(2) + ` <> '' ORDER BY page.id DESC`
	}
	args := []any{session.Workspace.Id, session.Id, pattern, search.Cursor, search.Limit + 1}
	if options {
		args = append(args, actor.Id)
	}
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("search chat participants: %w", err), nil, ""
	}
	defer rows.Close()
	items := []SessionParticipant{}
	for rows.Next() {
		var item SessionParticipant
		var alias, name, sourceGroup sql.NullString
		var inherited bool
		grant := SessionParticipantGrant{Enabled: true}
		values := []any{&item.ID, &alias, &name, &item.Enabled}
		if !options {
			values = append(values, &grant.ID, &grant.Role, &grant.Revision, &sourceGroup, &inherited)
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
		item.Grants = []SessionParticipantGrant{}
		if len(items) == 0 || items[len(items)-1].ID != item.ID {
			items = append(items, item)
		}
		participant := &items[len(items)-1]
		if !options {
			if inherited {
				participant.InheritedFrom = append(participant.InheritedFrom, SessionParticipantGroupSource{Group: sourceGroup.String, Role: grant.Role})
				continue
			}
			if search.Kind == "principal" {
				grant.Principal = &participant.ID
			} else if search.Kind == "agent" {
				grant.Agent = &participant.ID
			} else {
				grant.Group = &participant.ID
			}
			participant.Grants = append(participant.Grants, grant)
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

func sessionWorkspacePrincipalEligibleSQL(workspace, principal string) string {
	return `EXISTS (SELECT 1 FROM gatehouse_workspace_grants wg WHERE wg.workspace = ` + workspace + ` AND wg.enabled = TRUE AND (
		wg.principal = ` + principal + ` OR EXISTS (SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_members gm ON gm.workspace_id = g.workspace_id AND gm.group_id = g.id
		WHERE g.workspace_id = wg.workspace AND g.id = wg."group" AND g.enabled = TRUE AND gm.enabled = TRUE AND gm.principal_id = ` + principal + `)))`
}

// Lock the session row before checking roles or modifying grants. This serializes
// competing manager removals on PostgreSQL and acquires SQLite's write lock before
// reading the manager set. Permission is checked again inside the transaction.
func (store *Store) sessionParticipantTransaction(ctx context.Context, session model.SessionRef, actor model.PrincipalRef) (*sql.Tx, error) {
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	err = store.sessionParticipantLock(ctx, tx, session)
	if err == nil {
		var allowed bool
		allowed, err = store.sessionPrincipalAllowed(ctx, tx, session, actor, true)
		if err == nil && !allowed {
			err = ErrSessionParticipantForbidden
		}
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (store *Store) SessionParticipantGrantCreate(ctx context.Context, session model.SessionRef, actor model.PrincipalRef, principal, group *string, role authz.Role) (error, *SessionParticipantGrant) {
	return store.SessionParticipantGrantCreateSubject(ctx, session, actor, principal, group, nil, role)
}

func (store *Store) SessionParticipantGrantCreateSubject(ctx context.Context, session model.SessionRef, actor model.PrincipalRef, principal, group, agent *string, role authz.Role) (error, *SessionParticipantGrant) {
	ref := model.MessageTargetRef{}
	count := 0
	for kind, id := range map[string]*string{"principal": principal, "group": group, "agent": agent} {
		if id != nil {
			count++
			ref = model.MessageTargetRef{Kind: kind, Id: *id}
		}
	}
	if !authz.ValidRole(string(role)) || count != 1 || !typed_id.Valid(messageTargetIDKind(ref.Kind), ref.Id) {
		return ErrSessionParticipantInput, nil
	}
	tx, err := store.sessionParticipantTransaction(ctx, session, actor)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := store.sessionParticipantPlaceholder
	query, err := store.messageTargetSource(ref.Kind, true, p(4))
	if err != nil {
		return err, nil
	}
	if _, err := scanMessageTarget(tx.QueryRowContext(ctx, query+` AND subjects.id = `+p(3), session.Workspace.Id, session.Id, ref.Id, actor.Id), ref.Kind); err != nil {
		if errors.Is(err, ErrMessageTarget) {
			return ErrSessionParticipantNotFound, nil
		}
		return err, nil
	}
	grant, err := store.sessionParticipantGrantInsert(ctx, tx, session, ref, role, false)
	if err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return nil, grant
}

func (store *Store) sessionParticipantGrantInsert(ctx context.Context, tx *sql.Tx, session model.SessionRef, ref model.MessageTargetRef, role authz.Role, reuse bool) (*SessionParticipantGrant, error) {
	p := store.sessionParticipantPlaceholder
	id, err := typed_id.New(typed_id.SessionGrant)
	if err != nil {
		return nil, err
	}
	grant := SessionParticipantGrant{ID: id, Role: role, Enabled: true, Revision: 1}
	column := ref.Kind
	if ref.Kind == "group" {
		column = `"group"`
		grant.Group = &ref.Id
	} else if ref.Kind == "agent" {
		grant.Agent = &ref.Id
	} else {
		grant.Principal = &ref.Id
	}
	conflict := ` WHERE gatehouse_session_grants.enabled = FALSE`
	if reuse {
		conflict = ""
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO gatehouse_session_grants (id, workspace, session, principal, "group", agent, role, enabled, revision)
		VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, `+p(4)+`, `+p(5)+`, `+p(6)+`, `+p(7)+`, TRUE, 1)
		ON CONFLICT (workspace, session, role, `+column+`) WHERE `+column+` IS NOT NULL DO UPDATE
		SET enabled = TRUE, revision = gatehouse_session_grants.revision + 1`+conflict+` RETURNING id, revision`,
		id, session.Workspace.Id, session.Id, grant.Principal, grant.Group, grant.Agent, role).Scan(&grant.ID, &grant.Revision)
	if err == sql.ErrNoRows {
		return nil, ErrSessionGrantExists
	}
	if err != nil {
		return nil, err
	}
	event := "session_grant.create"
	if grant.Revision > 1 {
		event = "session_grant.update"
	}
	if err := store.sessionParticipantActivity(ctx, tx, session, grant, event); err != nil {
		return nil, err
	}
	return &grant, nil
}

func (store *Store) SessionParticipantGrantUpdate(ctx context.Context, session model.SessionRef, actor model.PrincipalRef, id string, role *authz.Role, enabled *bool, revision int) (error, *SessionParticipantGrant) {
	if !typed_id.Valid(typed_id.SessionGrant, id) || revision < 1 || (role == nil && enabled == nil) || (role != nil && !authz.ValidRole(string(*role))) {
		return ErrSessionParticipantInput, nil
	}
	tx, err := store.sessionParticipantTransaction(ctx, session, actor)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := store.sessionParticipantPlaceholder
	var grant SessionParticipantGrant
	var principal, group, agent sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, principal, "group", agent, role, enabled, revision FROM gatehouse_session_grants WHERE workspace = `+p(1)+` AND session = `+p(2)+` AND id = `+p(3), session.Workspace.Id, session.Id, id).Scan(&grant.ID, &principal, &group, &agent, &grant.Role, &grant.Enabled, &grant.Revision)
	if err == sql.ErrNoRows {
		return ErrSessionParticipantNotFound, nil
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
	if agent.Valid {
		grant.Agent = &agent.String
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
	// Re-enabling goes through the same subject validation as creation.
	if !grant.Enabled && nextEnabled {
		kind, subject := "principal", grant.Principal
		if grant.Group != nil {
			kind, subject = "group", grant.Group
		}
		if grant.Agent != nil {
			kind, subject = "agent", grant.Agent
		}
		query, err := store.messageTargetSource(kind, true, p(4))
		if err != nil {
			return err, nil
		}
		if _, err := scanMessageTarget(tx.QueryRowContext(ctx, query+` AND subjects.id = `+p(3), session.Workspace.Id, session.Id, *subject, actor.Id), kind); err != nil {
			if errors.Is(err, ErrMessageTarget) {
				return ErrSessionParticipantNotFound, nil
			}
			return err, nil
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE gatehouse_session_grants SET role = `+p(1)+`, enabled = `+p(2)+`, revision = revision + 1 WHERE id = `+p(3), nextRole, nextEnabled, id)
	if uniquePrincipalConstraint(err) {
		return ErrSessionGrantExists, nil
	}
	if err != nil {
		return err, nil
	}
	if grant.Enabled && grant.Role == authz.Manager && (!nextEnabled || nextRole != authz.Manager) {
		var manager bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM gatehouse_session_grants grants
			WHERE grants.workspace = `+p(1)+` AND grants.session = `+p(2)+` AND grants.enabled = TRUE AND grants.role = 'manager' AND (
			EXISTS (SELECT 1 FROM gatehouse_principals pr WHERE pr.id = grants.principal AND pr.enabled = TRUE)
			OR EXISTS (SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_members gm ON gm.workspace_id = g.workspace_id AND gm.group_id = g.id
			JOIN gatehouse_principals pr ON pr.id = gm.principal_id AND pr.enabled = TRUE
			WHERE g.workspace_id = grants.workspace AND g.id = grants."group" AND g.enabled = TRUE AND gm.enabled = TRUE)))`, session.Workspace.Id, session.Id).Scan(&manager)
		if err != nil {
			return err, nil
		}
		if !manager {
			return ErrSessionLastManager, nil
		}
	}
	grant.Role, grant.Enabled, grant.Revision = nextRole, nextEnabled, grant.Revision+1
	if err := store.sessionParticipantActivity(ctx, tx, session, grant, "session_grant.update"); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return nil, &grant
}

func (store *Store) sessionParticipantActivity(ctx context.Context, tx *sql.Tx, session model.SessionRef, grant SessionParticipantGrant, event string) error {
	topics := []string{ActivityTopicSessionGrant(session, grant.ID)}
	if grant.Principal != nil {
		topics = append(topics, ActivityTopicPrincipalGrant(model.PrincipalRef{Id: *grant.Principal}, grant.ID))
	} else if grant.Group != nil {
		topics = append(topics, ActivityTopicGroupGrant(session.Workspace, model.GroupRef{Id: *grant.Group}, grant.ID))
		members, err := groupPrincipalTopics(ctx, tx, keychainPlaceholder(store.kind), session.Workspace.Id, *grant.Group, grant.ID)
		if err != nil {
			return err
		}
		topics = append(topics, members...)
	}
	err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindSessionGrant, ResourceSessionGrant: &grant.ID}, topics)
	return err
}
