package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR011ReconcileWorkspaceAgents(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       11,
		Description: "reconcile_workspace_agents",
		Builder:     postgresMigrationR011ReconcileWorkspaceAgentsBuilder(state.WorkspaceAgents),
	}
}

type postgresMigrationR011WorkspaceAgentAliasKey struct {
	Workspace string
	Alias     string
}

type postgresMigrationR011StoredWorkspaceAgent struct {
	Revision     int
	Priority     int
	Label        sql.NullString
	SystemPrompt sql.NullString
	Enabled      bool
}

type postgresMigrationR011WorkspaceAgentValue struct {
	ID string
	config.WorkspaceAgent
}

func postgresMigrationR011ReconcileWorkspaceAgentsBuilder(agents []config.WorkspaceAgent) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := postgresMigrationR011WorkspaceAgentsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		workspaceIDs, err := workspaceIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		modelIDs, err := postgresMigrationR010AgentModelIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		desired := make([]postgresMigrationR011WorkspaceAgentValue, 0, len(agents))
		events := []activityMigrationEvent{}
		for _, agent := range agents {
			key := postgresMigrationR011WorkspaceAgentAliasKey{Workspace: agent.WorkspaceID, Alias: agent.Alias}
			stored, exists := existing[key]
			if exists && stored.Revision >= agent.Revision {
				continue
			}
			eventName := "workspace_agent.create"
			if exists {
				eventName = "workspace_agent.update"
			}
			modelID := modelIDs[agent.ModelAlias]
			if modelID == "" {
				return fmt.Errorf("agent model %q is unavailable", agent.ModelAlias), ""
			}
			workspaceID := workspaceIDs[agent.WorkspaceID]
			if workspaceID == "" {
				return fmt.Errorf("workspace %q is unavailable", agent.WorkspaceID), ""
			}
			bindingID := ""
			if exists {
				bindingID, err = postgresMigrationR011WorkspaceAgentIDByAlias(ctx, session, workspaceID, agent.Alias)
				if err != nil { return err, "" }
			} else {
				bindingID, err = typed_id.New(typed_id.WorkspaceAgent)
				if err != nil {
					return fmt.Errorf("create workspace agent binding ID: %w", err), ""
				}
			}
			desired = append(desired, postgresMigrationR011WorkspaceAgentValue{ID: bindingID, WorkspaceAgent: agent})
			event, err := newActivityMigrationEvent(agent.WorkspaceID, eventName, "workspace_agent", "", "", bindingID, workspaceID)
			if err != nil {
				return err, ""
			}
			event.WorkspaceAgentAlias = agent.Alias
			events = append(events, event)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Agents }}
			INSERT INTO gatehouse_workspace_agents (id, workspace_id, alias, model_id, revision, priority, label, system_prompt, enabled)
				VALUES ({{ sqlLiteral .ID }}, (SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .Alias }}, (SELECT id FROM gatehouse_agent_models WHERE alias = {{ sqlLiteral .ModelAlias }}), {{ sqlLiteral .Revision }}, {{ sqlLiteral .Priority }}, {{ sqlOptionalString .Label }}, {{ sqlOptionalString .SystemPrompt }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, alias) DO UPDATE SET model_id = excluded.model_id, revision = excluded.revision, priority = excluded.priority, label = excluded.label, system_prompt = excluded.system_prompt, enabled = excluded.enabled
			WHERE gatehouse_workspace_agents.revision < excluded.revision;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_workspace_agent_workspace, resource_workspace_agent_id, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }},
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceAlias }}),
				COALESCE(NULLIF({{ sqlLiteral .WorkspaceAgent }}, ''), (SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = (SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceAlias }}) AND alias = {{ sqlLiteral .WorkspaceAgentAlias }})), {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Agents []postgresMigrationR011WorkspaceAgentValue
			Events []activityMigrationEvent
		}{Agents: desired, Events: events})
	}
}

func postgresMigrationR011WorkspaceAgentsByAlias(ctx context.Context, session *MigrationSession) (map[postgresMigrationR011WorkspaceAgentAliasKey]postgresMigrationR011StoredWorkspaceAgent, error) {
	rows, err := session.QueryContext(ctx, `
			SELECT workspaces.alias, agents.alias, agents.revision, agents.priority, agents.label, agents.system_prompt, agents.enabled
		FROM gatehouse_workspace_agents AS agents
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = agents.workspace_id
		WHERE workspaces.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get workspace agents by alias: %w", err)
	}
	defer rows.Close()

	agents := map[postgresMigrationR011WorkspaceAgentAliasKey]postgresMigrationR011StoredWorkspaceAgent{}
	for rows.Next() {
		var key postgresMigrationR011WorkspaceAgentAliasKey
		var agent postgresMigrationR011StoredWorkspaceAgent
		if err := rows.Scan(&key.Workspace, &key.Alias, &agent.Revision, &agent.Priority, &agent.Label, &agent.SystemPrompt, &agent.Enabled); err != nil {
			return nil, fmt.Errorf("scan workspace agent: %w", err)
		}
		agents[key] = agent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace agents: %w", err)
	}
	return agents, nil
}

func postgresMigrationR011WorkspaceAgentIDByAlias(ctx context.Context, session *MigrationSession, workspaceID, alias string) (string, error) {
	rows, err := session.QueryContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = $1 AND alias = $2`, workspaceID, alias)
	if err != nil { return "", fmt.Errorf("get workspace agent binding ID: %w", err) }
	defer rows.Close()
	if !rows.Next() { return "", fmt.Errorf("get workspace agent binding ID: binding not found") }
	var id string
	if err := rows.Scan(&id); err != nil { return "", fmt.Errorf("scan workspace agent binding ID: %w", err) }
	return id, nil
}
