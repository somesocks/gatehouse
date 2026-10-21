package database

import (
	"context"
	"database/sql"
	"fmt"

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
	err := tx.QueryRowContext(ctx, `SELECT `+sessionPrincipalAccessSQL(p(1), p(2), p(3), roles), session.Workspace.Id, session.Id, actor.Id).Scan(&allowed)
	return allowed, err
}

func sessionPrincipalAccessSQL(workspace, session, principal, roles string) string {
	return `EXISTS (SELECT 1 FROM gatehouse_sessions admitted_session
		WHERE admitted_session.workspace = ` + workspace + ` AND admitted_session.id = ` + session + ` AND admitted_session.enabled = TRUE
		AND ` + workspacePrincipalAdmissionSQL(workspace, principal) + `
		AND EXISTS (SELECT 1 FROM gatehouse_session_grants access_grant
			WHERE access_grant.workspace = admitted_session.workspace AND access_grant.session = admitted_session.id AND access_grant.enabled = TRUE AND access_grant.role IN (` + roles + `)
			AND (access_grant.principal = ` + principal + ` OR EXISTS (
				SELECT 1 FROM gatehouse_groups access_group JOIN gatehouse_group_members access_member ON access_member.workspace_id = access_group.workspace_id AND access_member.group_id = access_group.id
				WHERE access_group.workspace_id = access_grant.workspace AND access_group.id = access_grant."group" AND access_group.enabled = TRUE AND access_member.enabled = TRUE AND access_member.principal_id = ` + principal + `)))
		AND (admitted_session.project IS NULL OR EXISTS (
			SELECT 1 FROM gatehouse_projects access_project JOIN gatehouse_project_grants access_grant ON access_grant.workspace = access_project.workspace AND access_grant.project = access_project.id AND access_grant.enabled = TRUE
			WHERE access_project.workspace = admitted_session.workspace AND access_project.id = admitted_session.project AND access_project.enabled = TRUE
			AND (access_grant.principal = ` + principal + ` OR EXISTS (
				SELECT 1 FROM gatehouse_groups access_group JOIN gatehouse_group_members access_member ON access_member.workspace_id = access_group.workspace_id AND access_member.group_id = access_group.id
				WHERE access_group.workspace_id = access_grant.workspace AND access_group.id = access_grant."group" AND access_group.enabled = TRUE AND access_member.enabled = TRUE AND access_member.principal_id = ` + principal + `)))))`
}

func (store *Store) SessionRequestAuthorizedGet(ctx context.Context, ref model.SessionEventRef) (error, bool) {
	return store.sessionRequestAuthorizedGet(ref, func(ref model.SessionEventRef) (error, *model.SessionEvent) { return store.SessionEventGet(ctx, ref) }, func(query string, args ...any) sessionEventRow { return store.QueryRowContext(ctx, query, args...) })
}

func (store *Store) SessionRequestAuthorizedGetInTransaction(ctx context.Context, tx dbos.Tx, ref model.SessionEventRef) (error, bool) {
	return store.sessionRequestAuthorizedGet(ref, func(ref model.SessionEventRef) (error, *model.SessionEvent) {
		return store.SessionEventGetInTransaction(ctx, tx, ref)
	}, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) sessionRequestAuthorizedGet(ref model.SessionEventRef, get func(model.SessionEventRef) (error, *model.SessionEvent), query sessionRowQuery) (error, bool) {
	seen := map[model.SessionEventRef]bool{}
	for !seen[ref] {
		seen[ref] = true
		err, event := get(ref)
		if err != nil || event == nil {
			return err, false
		}
		if event.AuthorPrincipal != nil {
			p := store.sessionParticipantPlaceholder
			var allowed bool
			err := query(`SELECT `+sessionPrincipalAccessSQL(p(1), p(2), p(3), "'contributor', 'manager'"), ref.Session.Workspace.Id, ref.Session.Id, event.AuthorPrincipal.Ref.Id).Scan(&allowed)
			return err, allowed
		}
		if event.Parent == nil || event.Parent.Session != ref.Session {
			return nil, false
		}
		ref = *event.Parent
	}
	return nil, false
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
	membership := `EXISTS (SELECT 1 FROM gatehouse_session_grants grants WHERE grants.workspace = ` + workspace + ` AND grants.session = ` + session + ` AND grants.enabled = TRUE AND (` + condition + `))`
	if kind == "principal" {
		membership = `(` + workspacePrincipalAdmissionSQL(workspace, subject) + ` AND ` + membership + `)`
	}
	return membership
}

func (store *Store) sessionAgentParticipationSQL(workspace, session, agent string) string {
	// Availability is checked when discovering/sending and resolving a live model.
	// Participation is independent of bot/model/provider availability. Losing the
	// last chat path prevents admission to the next round of an existing request.
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
