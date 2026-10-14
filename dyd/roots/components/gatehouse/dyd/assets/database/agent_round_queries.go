package database

import (
	"context"

	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// These transaction-scoped queries let round preparation share one transaction
// with its event and DBOS checkpoint. They reuse the ordinary session queries,
// without acquiring a second connection while SQLite's transaction is open.
func (store *Store) SessionEventGetInTransaction(ctx context.Context, tx dbos.Tx, ref model.SessionEventRef) (error, *model.SessionEvent) {
	return store.sessionEventGet(ref, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) WorkspaceAgentModelGetInTransaction(ctx context.Context, tx dbos.Tx, workspace model.WorkspaceRef, id string) (error, *WorkspaceAgentModel) {
	return store.workspaceAgentModelGet(workspace, id, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) SessionProjectGetInTransaction(ctx context.Context, tx dbos.Tx, session model.SessionRef) (error, *model.ProjectRef) {
	return store.sessionProjectGet(session, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) SessionAgentRequestEventsGetInTransaction(ctx context.Context, tx dbos.Tx, ref model.SessionEventRef, agent model.WorkspaceAgentRef) (error, []model.SessionEvent) {
	return store.sessionAgentRequestEventsGet(ref, agent, "", func(query string, args ...any) (sessionEventRows, error) {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		return roundQueryRows{Rows: rows}, nil
	})
}

type roundQueryRows struct{ dbos.Rows }

func (rows roundQueryRows) Close() error { rows.Rows.Close(); return nil }

func (store *Store) AgentContextGetInTransaction(ctx context.Context, tx dbos.Tx, ref model.SessionEventRef) (error, *AgentContext) {
	return store.agentContextGet(ref, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) AgentContextPreviousGetInTransaction(ctx context.Context, tx dbos.Tx, session model.SessionRef, agent model.WorkspaceAgentRef, before string) (error, *AgentContext) {
	return store.agentContextPreviousGet(session, agent, before, func(query string, args ...any) sessionEventRow { return tx.QueryRow(ctx, query, args...) })
}

func (store *Store) AgentContextSetInTransaction(ctx context.Context, tx dbos.Tx, value AgentContext) error {
	return store.agentContextSet(value, func(query string, args ...any) error { _, err := tx.Exec(ctx, query, args...); return err })
}
