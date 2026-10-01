package database

import (
	"context"
	"database/sql"
	"fmt"

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
	return store.sessionAgentRequestEventsGet(ref, agent, func(query string, args ...any) (sessionEventRows, error) {
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

// AgentProviderCredentialsGet resolves credentials for the saved provider ID,
// independently of changes to the workspace agent's current model binding.
func (store *Store) AgentProviderCredentialsGet(ctx context.Context, id string) (error, *WorkspaceAgentModel) {
	p := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `SELECT alias, base_url, keychain_id, keychain_version, api_key
		FROM gatehouse_agent_providers WHERE id = `+p(1)+` AND enabled = TRUE`, id)
	var alias, baseURL, chain, apiKey sql.NullString
	var version sql.NullInt64
	if err := row.Scan(&alias, &baseURL, &chain, &version, &apiKey); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get provider credentials: %w", err), nil
	}
	if !baseURL.Valid || !chain.Valid || !version.Valid || !apiKey.Valid {
		return fmt.Errorf("provider credentials are unavailable"), nil
	}
	selected := &WorkspaceAgentModel{ProviderID: id, BaseURL: &baseURL.String, APIKey: &apiKey.String,
		Keychain: &model.KeychainRef{Id: chain.String, Version: int(version.Int64)}}
	if alias.Valid {
		selected.ProviderAlias = &alias.String
	}
	return nil, selected
}
