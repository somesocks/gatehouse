package database

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/config"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func (store *Store) sessionParticipantLock(ctx context.Context, tx *sql.Tx, session model.SessionRef) error {
	p := store.sessionParticipantPlaceholder
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_sessions SET enabled = enabled WHERE workspace = `+p(1)+` AND id = `+p(2)+` AND enabled = TRUE`, session.Workspace.Id, session.Id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrSessionParticipantNotFound
	}
	return err
}

func (store *Store) sessionPrincipalAllowed(ctx context.Context, tx *sql.Tx, session model.SessionRef, actor model.PrincipalRef, manage bool) (bool, error) {
	p := store.sessionParticipantPlaceholder
	roles := "'contributor', 'manager'"
	if manage {
		roles = "'manager'"
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM gatehouse_session_grants grants JOIN gatehouse_workspaces w ON w.id = grants.workspace
		WHERE grants.workspace = `+p(1)+` AND grants.session = `+p(2)+` AND w.enabled = TRUE AND grants.enabled = TRUE AND grants.role IN (`+roles+`)
		AND EXISTS (SELECT 1 FROM gatehouse_principals principal WHERE principal.id = `+p(3)+` AND principal.enabled = TRUE)
		AND (grants.principal = `+p(3)+` OR EXISTS (
			SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_members gm ON gm.workspace_id = g.workspace_id AND gm.group_id = g.id
			WHERE g.workspace_id = grants.workspace AND g.id = grants."group" AND g.enabled = TRUE AND gm.enabled = TRUE AND gm.principal_id = `+p(3)+`)))`, session.Workspace.Id, session.Id, actor.Id).Scan(&allowed)
	return allowed, err
}

func (store *Store) messageTargetMembershipSQL(kind, subject, workspace, session string) string {
	condition := "grants." + kind + " = " + subject
	if kind == "group" {
		condition = `grants."group" = ` + subject
	}
	if kind == "principal" {
		condition += ` OR EXISTS (SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_members gm ON gm.workspace_id = g.workspace_id AND gm.group_id = g.id
			WHERE g.workspace_id = grants.workspace AND g.id = grants."group" AND g.enabled = TRUE AND gm.enabled = TRUE AND gm.principal_id = ` + subject + `)`
	}
	if kind == "agent" {
		condition += ` OR EXISTS (SELECT 1 FROM gatehouse_groups g JOIN gatehouse_group_agents ga ON ga.workspace_id = g.workspace_id AND ga.group_id = g.id
			WHERE g.workspace_id = grants.workspace AND g.id = grants."group" AND g.enabled = TRUE AND ga.enabled = TRUE AND ga.agent_id = ` + subject + `)`
	}
	return `EXISTS (SELECT 1 FROM gatehouse_session_grants grants WHERE grants.workspace = ` + workspace + ` AND grants.session = ` + session + ` AND grants.enabled = TRUE AND (` + condition + `))`
}

func (store *Store) sessionAgentParticipationSQL(workspace, session, agent string) string {
	// Availability is checked when discovering/sending and resolving a live model.
	// Participation must still permit replay of committed decisions after a bot,
	// model, or provider is disabled; only losing its chat paths detaches a request.
	return `EXISTS (SELECT 1 FROM gatehouse_sessions participating_session
		JOIN gatehouse_workspaces participating_workspace ON participating_workspace.id = participating_session.workspace
		WHERE participating_session.workspace = ` + workspace + ` AND participating_session.id = ` + session + `
		AND participating_session.enabled = TRUE AND participating_workspace.enabled = TRUE
		AND ` + store.messageTargetMembershipSQL("agent", agent, workspace, session) + `)`
}

func (store *Store) SessionAgentParticipantGet(ctx context.Context, session model.SessionRef, agent model.WorkspaceAgentRef) (error, bool) {
	return store.sessionAgentParticipantGet(session, agent, func(query string, args ...any) sessionEventRow { return store.QueryRowContext(ctx, query, args...) })
}

func (store *Store) SessionAgentParticipantGetInTransaction(ctx context.Context, tx dbos.Tx, session model.SessionRef, agent model.WorkspaceAgentRef) (error, bool) {
	return store.sessionAgentParticipantGet(session, agent, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) sessionAgentParticipantGet(session model.SessionRef, agent model.WorkspaceAgentRef, query sessionRowQuery) (error, bool) {
	if session.Workspace != agent.Workspace {
		return nil, false
	}
	p := store.sessionParticipantPlaceholder
	var active bool
	err := query(`SELECT `+store.sessionAgentParticipationSQL(p(1), p(2), p(3)), session.Workspace.Id, session.Id, agent.Id).Scan(&active)
	if err != nil {
		return fmt.Errorf("check bot participation: %w", err), false
	}
	return nil, active
}

// Detached requests are reconciled through the same durable cancellation flow
// as an explicit user cancellation, including pending tool/input workflows.
func (store *Store) SessionDetachedAgentRequestsGet(ctx context.Context, limit int) (error, []model.SessionEventRef) {
	target := "json_extract(events.payload, '$.agent')"
	if store.kind != config.DatabaseKindSQLite {
		target = "events.payload ->> 'agent'"
	}
	rows, err := store.QueryContext(ctx, `SELECT events.workspace, events.session, events.id FROM gatehouse_session_events events
		WHERE events.kind = 'agent.request' AND events.author_principal IS NOT NULL
		AND NOT `+store.sessionAgentParticipationSQL("events.workspace", "events.session", target)+`
		AND NOT EXISTS (SELECT 1 FROM gatehouse_session_events child WHERE child.workspace = events.workspace AND child.session = events.session AND child.parent = events.id AND child.kind IN ('agent.success', 'agent.failure', 'cancel.request'))
		ORDER BY events.id LIMIT `+keychainPlaceholder(store.kind)(1), limit)
	if err != nil {
		return err, nil
	}
	defer rows.Close()
	refs := []model.SessionEventRef{}
	for rows.Next() {
		var ref model.SessionEventRef
		if err := rows.Scan(&ref.Session.Workspace.Id, &ref.Session.Id, &ref.Id); err != nil {
			return err, nil
		}
		refs = append(refs, ref)
	}
	return rows.Err(), refs
}
