package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gatehouse/authz"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrMessageTarget = errors.New("message recipient is unavailable or invalid")

func messageTargetIDKind(kind string) string {
	switch kind {
	case "agent":
		return typed_id.WorkspaceAgent
	case "principal":
		return typed_id.Principal
	case "group":
		return typed_id.Group
	default:
		return ""
	}
}

// SessionMessageTargets reads canonical targets and historical bot-only messages.
// The order is retained, and each identity appears at most once.
func SessionMessageTargets(event model.SessionEvent) ([]model.MessageTarget, error) {
	if event.Kind != model.SessionEventKindMessageText {
		return nil, fmt.Errorf("message targets: event must be message.text")
	}
	if _, exists := event.Payload["agent"]; exists {
		return nil, fmt.Errorf("%w: use targets instead of agent", ErrMessageTarget)
	}
	targets := []model.MessageTarget{}
	if value, exists := event.Payload["targets"]; exists {
		if _, legacy := event.Payload["agents"]; legacy {
			return nil, fmt.Errorf("%w: specify targets or agents, not both", ErrMessageTarget)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed targets", ErrMessageTarget)
		}
		if err := json.Unmarshal(encoded, &targets); err != nil || targets == nil {
			return nil, fmt.Errorf("%w: targets must be a list", ErrMessageTarget)
		}
	} else if value, exists := event.Payload["agents"]; exists {
		agents, ok := eventStringSlice(value)
		if !ok {
			return nil, fmt.Errorf("%w: agents must be binding IDs", ErrMessageTarget)
		}
		for _, id := range agents {
			targets = append(targets, model.MessageTarget{Ref: model.MessageTargetRef{Kind: "agent", Id: id}})
		}
	}
	result := []model.MessageTarget{}
	seen := map[model.MessageTargetRef]bool{}
	for _, target := range targets {
		kind := messageTargetIDKind(target.Ref.Kind)
		if kind == "" || !typed_id.Valid(kind, target.Ref.Id) {
			return nil, ErrMessageTarget
		}
		if !seen[target.Ref] {
			seen[target.Ref] = true
			result = append(result, target)
		}
	}
	return result, nil
}

// messageTargetSource uses the same eligibility rules for lookup and send-time
// validation, including people whose access comes through a conversation group.
func (store *Store) messageTargetSource(kind string, outside bool, actor string) (string, error) {
	p := store.sessionParticipantPlaceholder
	filter := store.messageTargetMembershipSQL(kind, "subjects.id", p(1), p(2))
	if outside {
		var err error
		filter, err = chatInvitationEligibilitySQL(kind, p(1), actor, "subjects.id")
		if err != nil {
			return "", err
		}
	}
	switch kind {
	case "agent":
		return `SELECT subjects.id, subjects.alias, subjects.label FROM gatehouse_workspace_agents subjects
			JOIN gatehouse_agent_models models ON models.id = subjects.model_id
			JOIN gatehouse_agent_providers providers ON providers.id = models.provider_id
			WHERE subjects.workspace_id = ` + p(1) + ` AND subjects.enabled = TRUE AND models.enabled = TRUE AND providers.enabled = TRUE
			AND ` + p(2) + ` <> '' AND ` + filter, nil
	case "principal":
		return `SELECT subjects.id, subjects.alias, subjects.name FROM gatehouse_principals subjects
			WHERE subjects.enabled = TRUE AND ` + p(2) + ` <> '' AND ` + filter, nil
	case "group":
		return `SELECT subjects.id, subjects.alias, subjects.name FROM gatehouse_groups subjects
			WHERE subjects.workspace_id = ` + p(1) + ` AND subjects.enabled = TRUE AND ` + p(2) + ` <> '' AND ` + filter, nil
	default:
		return "", ErrMessageTarget
	}
}

func scanMessageTarget(row sessionEventRow, kind string) (model.MessageTarget, error) {
	target := model.MessageTarget{Ref: model.MessageTargetRef{Kind: kind}}
	var alias, name sql.NullString
	if err := row.Scan(&target.Ref.Id, &alias, &name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return target, ErrMessageTarget
		}
		return target, err
	}
	if alias.Valid {
		target.Alias = &alias.String
	}
	if name.Valid {
		target.Name = &name.String
	}
	return target, nil
}

func (store *Store) sessionMessageTargetsSnapshot(ctx context.Context, tx *sql.Tx, event model.SessionEvent) error {
	targets, err := SessionMessageTargets(event)
	if err != nil {
		return err
	}
	// Resolve groups first so their members inherit access regardless of the
	// order recipients were selected. Keep snapshots in the original order.
	order := make([]int, 0, len(targets))
	for index, target := range targets {
		if target.Ref.Kind == "group" {
			order = append(order, index)
		}
	}
	for index, target := range targets {
		if target.Ref.Kind != "group" {
			order = append(order, index)
		}
	}
	for _, index := range order {
		target := targets[index]
		query, err := store.messageTargetSource(target.Ref.Kind, false, "")
		if err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, query+` AND subjects.id = `+store.sessionParticipantPlaceholder(3), event.Ref.Session.Workspace.Id, event.Ref.Session.Id, target.Ref.Id)
		targets[index], err = scanMessageTarget(row, target.Ref.Kind)
		if errors.Is(err, ErrMessageTarget) {
			allowed, permissionErr := store.sessionPrincipalAllowed(ctx, tx, event.Ref.Session, event.AuthorPrincipal.Ref, true)
			if permissionErr != nil {
				return permissionErr
			}
			if !allowed {
				return ErrSessionParticipantForbidden
			}
			query, err = store.messageTargetSource(target.Ref.Kind, true, store.sessionParticipantPlaceholder(4))
			if err != nil {
				return err
			}
			targets[index], err = scanMessageTarget(tx.QueryRowContext(ctx, query+` AND subjects.id = `+store.sessionParticipantPlaceholder(3), event.Ref.Session.Workspace.Id, event.Ref.Session.Id, target.Ref.Id, event.AuthorPrincipal.Ref.Id), target.Ref.Kind)
			if err != nil {
				return err
			}
			if _, err = store.sessionParticipantGrantInsert(ctx, tx, event.Ref.Session, target.Ref, authz.Member, true); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	// Always snapshot from server data; caller-provided labels are never trusted.
	event.Payload["targets"] = targets
	delete(event.Payload, "agents")
	return nil
}

func (store *Store) SessionMessageTargetsSearch(ctx context.Context, session model.SessionRef, actor model.PrincipalRef, search SessionParticipantSearch) (error, []model.MessageTarget, string) {
	kind := messageTargetIDKind(search.Kind)
	if kind == "" || search.Limit < 1 || search.Limit > 100 || len(strings.TrimSpace(search.Name)) > 256 || (search.Cursor != "" && !typed_id.Valid(kind, search.Cursor)) {
		return ErrMessageTarget, nil, ""
	}
	if err, available := store.SessionGet(ctx, session, actor); err != nil {
		return err, nil, ""
	} else if available == nil {
		return ErrSessionParticipantNotFound, nil, ""
	}
	if search.Outside {
		if err, roles := store.SessionRolesGet(ctx, session, actor); err != nil {
			return err, nil, ""
		} else if !authz.SessionAllows(roles, authz.SessionGrantManage) {
			return ErrSessionParticipantForbidden, nil, ""
		}
	}
	p := store.sessionParticipantPlaceholder
	query, err := store.messageTargetSource(search.Kind, search.Outside, p(6))
	if err != nil {
		return err, nil, ""
	}
	if search.Outside {
		query += ` AND NOT ` + store.messageTargetMembershipSQL(search.Kind, "subjects.id", p(1), p(2))
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(search.Name)) + "%"
	nameColumn := "subjects.name"
	if search.Kind == "agent" {
		nameColumn = "subjects.label"
	}
	query += ` AND (LOWER(COALESCE(subjects.alias, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\' OR LOWER(COALESCE(` + nameColumn + `, '')) LIKE LOWER(` + p(3) + `) ESCAPE '\')
		AND (` + p(4) + ` = '' OR subjects.id < ` + p(4) + `) ORDER BY subjects.id DESC LIMIT ` + p(5)
	args := []any{session.Workspace.Id, session.Id, pattern, search.Cursor, search.Limit + 1}
	if search.Outside {
		args = append(args, actor.Id)
	}
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	targets := []model.MessageTarget{}
	for rows.Next() {
		target, err := scanMessageTarget(rows, search.Kind)
		if err != nil {
			return err, nil, ""
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(targets) > search.Limit {
		return nil, targets[:search.Limit], targets[search.Limit-1].Ref.Id
	}
	return nil, targets, ""
}
