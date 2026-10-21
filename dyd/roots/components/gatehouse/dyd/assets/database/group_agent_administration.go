package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrGroupAgentNotFound = errors.New("bot not found in this workspace")

type SystemGroupAgent struct {
	Agent        string  `json:"agent"`
	Alias        string  `json:"alias"`
	Label        *string `json:"label,omitempty"`
	AgentEnabled bool    `json:"agent_enabled"`
	Enabled      bool    `json:"enabled"`
	Revision     int     `json:"revision"`
}

func (store *Store) SystemGroupAgentsSearch(ctx context.Context, search SystemGroupSearch) (error, []SystemGroupAgent, string) {
	pattern, err := groupSearchValues(search, typed_id.WorkspaceAgent)
	if err != nil {
		return err, nil, ""
	}
	p := keychainPlaceholder(store.kind)
	query := `SELECT agents.id, agents.alias, agents.label, agents.enabled, members.enabled, members.revision
		FROM gatehouse_group_agents members JOIN gatehouse_workspace_agents agents
		ON agents.workspace_id = members.workspace_id AND agents.id = members.agent_id
		WHERE members.workspace_id = ` + p(1) + ` AND members.group_id = ` + p(2) + `
		AND (LOWER(agents.alias) LIKE LOWER(` + p(3) + `) ESCAPE '\' OR LOWER(COALESCE(agents.label, '')) LIKE LOWER(` + p(4) + `) ESCAPE '\' OR agents.id LIKE ` + p(5) + ` ESCAPE '\')
		AND (` + p(6) + ` = '' OR agents.id < ` + p(7) + `)`
	args := []any{search.Workspace, search.Group, pattern, pattern, pattern, search.Cursor, search.Cursor}
	if search.ActiveOnly {
		query += ` AND members.enabled = TRUE AND agents.enabled = TRUE`
	} else if search.Status != "" {
		query += ` AND members.enabled = ` + p(len(args)+1)
		args = append(args, search.Status == "enabled")
	}
	query += ` ORDER BY agents.id DESC LIMIT ` + p(len(args)+1)
	args = append(args, search.Limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	agents := []SystemGroupAgent{}
	for rows.Next() {
		var agent SystemGroupAgent
		var label sql.NullString
		if err := rows.Scan(&agent.Agent, &agent.Alias, &label, &agent.AgentEnabled, &agent.Enabled, &agent.Revision); err != nil {
			return err, nil, ""
		}
		if label.Valid {
			agent.Label = &label.String
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(agents) > search.Limit {
		return nil, agents[:search.Limit], agents[search.Limit-1].Agent
	}
	return nil, agents, ""
}

func (store *Store) SystemGroupAgentCreate(ctx context.Context, workspace, group, agent string) error {
	return store.GroupAgentCreate(ctx, workspace, group, agent, nil)
}

func (store *Store) GroupAgentCreate(ctx context.Context, workspace, group, agent string, actor *model.PrincipalRef) error {
	if !typed_id.Valid(typed_id.Workspace, workspace) || !typed_id.Valid(typed_id.Group, group) {
		return ErrGroupNotFound
	}
	if !typed_id.Valid(typed_id.WorkspaceAgent, agent) {
		return fmt.Errorf("%w: invalid bot", ErrGroupInput)
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
	activeFilter := ""
	if actor != nil {
		activeFilter = " AND enabled = TRUE"
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_workspace_agents WHERE workspace_id = `+p(1)+` AND id = `+p(2)+activeFilter+`)`, workspace, agent).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrGroupAgentNotFound
	}
	var memberRevision int
	err = tx.QueryRowContext(ctx, `INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, TRUE, 1)
		ON CONFLICT (workspace_id, group_id, agent_id) DO UPDATE SET enabled = TRUE, revision = gatehouse_group_agents.revision + 1
		WHERE gatehouse_group_agents.enabled = FALSE RETURNING revision`, workspace, group, agent).Scan(&memberRevision)
	if err == sql.ErrNoRows {
		return ErrGroupMemberExists
	}
	if err != nil {
		return err
	}
	event := "group_agent.create"
	if memberRevision > 1 {
		event = "group_agent.update"
	}
	if err := store.systemGroupAgentActivity(ctx, tx, workspace, group, agent, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) SystemGroupAgentUpdate(ctx context.Context, workspace, group, agent string, enabled bool, revision int) error {
	return store.GroupAgentUpdate(ctx, workspace, group, agent, enabled, revision, nil)
}

func (store *Store) GroupAgentUpdate(ctx context.Context, workspace, group, agent string, enabled bool, revision int, actor *model.PrincipalRef) error {
	if !typed_id.Valid(typed_id.Workspace, workspace) || !typed_id.Valid(typed_id.Group, group) || !typed_id.Valid(typed_id.WorkspaceAgent, agent) {
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
	if err := store.groupMutationAllowed(ctx, tx, workspace, group, actor); err != nil {
		return err
	}
	p := keychainPlaceholder(store.kind)
	var current bool
	var currentRevision int
	err = tx.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_group_agents WHERE workspace_id = `+p(1)+` AND group_id = `+p(2)+` AND agent_id = `+p(3), workspace, group, agent).Scan(&current, &currentRevision)
	if err == sql.ErrNoRows {
		return ErrGroupNotFound
	}
	if err != nil {
		return err
	}
	if currentRevision != revision {
		return ErrRevisionConflict
	}
	if current != enabled {
		result, err := tx.ExecContext(ctx, `UPDATE gatehouse_group_agents SET enabled = `+p(1)+`, revision = revision + 1 WHERE workspace_id = `+p(2)+` AND group_id = `+p(3)+` AND agent_id = `+p(4)+` AND revision = `+p(5), enabled, workspace, group, agent, revision)
		if err != nil {
			return err
		}
		if err := groupUpdatedOnce(result); err != nil {
			return err
		}
		if err := store.systemGroupAgentActivity(ctx, tx, workspace, group, agent, "group_agent.update"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (store *Store) systemGroupAgentActivity(ctx context.Context, tx *sql.Tx, workspace, group, agent, event string) error {
	topics, err := groupPrincipalTopics(ctx, tx, keychainPlaceholder(store.kind), workspace, group, group)
	if err != nil {
		return err
	}
	topics = append(topics, workspace+"/"+group, workspace+"/"+agent)
	err, _ = store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindGroupAgent, ResourceGroupAgentGroup: &group, ResourceGroupAgentAgent: &agent}, topics)
	return err
}
