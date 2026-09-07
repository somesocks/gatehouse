package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/config"
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
	Model     string
}

type postgresMigrationR011StoredWorkspaceAgent struct {
	Revision     int
	Priority     int
	Label        sql.NullString
	SystemPrompt sql.NullString
	Enabled      bool
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
		desired := make([]config.WorkspaceAgent, 0, len(agents))
		events := []activityMigrationEvent{}
		for _, agent := range agents {
			key := postgresMigrationR011WorkspaceAgentAliasKey{Workspace: agent.WorkspaceID, Model: agent.ModelAlias}
			stored, exists := existing[key]
			if exists && stored.Revision >= agent.Revision {
				continue
			}
			desired = append(desired, agent)
			eventName := "workspace_agent.create"
			if exists {
				eventName = "workspace_agent.update"
			}
			modelID := modelIDs[agent.ModelAlias]
			workspaceID := workspaceIDs[agent.WorkspaceID]
			if workspaceID == "" {
				return fmt.Errorf("workspace %q is unavailable", agent.WorkspaceID), ""
			}
			event, err := newActivityMigrationEvent(agent.WorkspaceID, eventName, "workspace_agent", "", "", modelID, workspaceID)
			if err != nil {
				return err, ""
			}
			events = append(events, event)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Agents }}
			INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, revision, priority, label, system_prompt, enabled)
				VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), (SELECT id FROM gatehouse_agent_models WHERE alias = {{ sqlLiteral .ModelAlias }}), {{ sqlLiteral .Revision }}, {{ sqlLiteral .Priority }}, {{ sqlOptionalString .Label }}, {{ sqlOptionalString .SystemPrompt }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, model_id) DO UPDATE SET revision = excluded.revision, priority = excluded.priority, label = excluded.label, system_prompt = excluded.system_prompt, enabled = excluded.enabled
			WHERE gatehouse_workspace_agents.revision < excluded.revision;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_workspace_agent_workspace, resource_workspace_agent_model, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }},
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceAlias }}),
				{{ sqlLiteral .WorkspaceAgent }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Agents []config.WorkspaceAgent
			Events []activityMigrationEvent
		}{Agents: desired, Events: events})
	}
}

func postgresMigrationR011WorkspaceAgentsByAlias(ctx context.Context, session *MigrationSession) (map[postgresMigrationR011WorkspaceAgentAliasKey]postgresMigrationR011StoredWorkspaceAgent, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, models.alias, agents.revision, agents.priority, agents.label, agents.system_prompt, agents.enabled
		FROM gatehouse_workspace_agents AS agents
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = agents.workspace_id
		JOIN gatehouse_agent_models AS models ON models.id = agents.model_id
		WHERE workspaces.alias IS NOT NULL AND models.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get workspace agents by alias: %w", err)
	}
	defer rows.Close()

	agents := map[postgresMigrationR011WorkspaceAgentAliasKey]postgresMigrationR011StoredWorkspaceAgent{}
	for rows.Next() {
		var key postgresMigrationR011WorkspaceAgentAliasKey
		var agent postgresMigrationR011StoredWorkspaceAgent
		if err := rows.Scan(&key.Workspace, &key.Model, &agent.Revision, &agent.Priority, &agent.Label, &agent.SystemPrompt, &agent.Enabled); err != nil {
			return nil, fmt.Errorf("scan workspace agent: %w", err)
		}
		agents[key] = agent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace agents: %w", err)
	}
	return agents, nil
}
