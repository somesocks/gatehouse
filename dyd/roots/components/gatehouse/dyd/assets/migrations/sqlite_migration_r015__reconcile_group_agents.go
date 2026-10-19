package migrations

import (
	"context"

	"gatehouse/config"
)

func sqliteMigrationR015ReconcileGroupAgents(groups []config.Group) RepeatableMigration {
	return RepeatableMigration{
		Index:       15,
		Description: "reconcile_group_agents",
		Builder:     sqliteMigrationR015ReconcileGroupAgentsBuilder(groups),
	}
}

func sqliteMigrationR015ReconcileGroupAgentsBuilder(groups []config.Group) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_group_agents_desired;
			DROP TABLE IF EXISTS gatehouse_migration_group_agent_state;
			DROP TABLE IF EXISTS gatehouse_migration_group_agent_activities;
			CREATE TEMP TABLE gatehouse_migration_group_agents_desired (
				workspace_alias TEXT NOT NULL, group_alias TEXT NOT NULL, agent_alias TEXT NOT NULL,
				enabled INTEGER NOT NULL, revision INTEGER NOT NULL,
				PRIMARY KEY (workspace_alias, group_alias, agent_alias)
			) STRICT;
			CREATE TEMP TRIGGER gatehouse_migration_group_agent_validate
			BEFORE INSERT ON gatehouse_migration_group_agents_desired
			WHEN NOT EXISTS (
				SELECT 1 FROM gatehouse_workspaces workspaces
				JOIN gatehouse_groups groups ON groups.workspace_id = workspaces.id AND groups.alias = NEW.group_alias
				JOIN gatehouse_workspace_agents agents ON agents.workspace_id = workspaces.id AND agents.alias = NEW.agent_alias
				WHERE workspaces.alias = NEW.workspace_alias
			)
			BEGIN
				SELECT RAISE(ABORT, 'cannot reconcile group bot membership: workspace ' || quote(NEW.workspace_alias)
					|| ', group ' || quote(NEW.group_alias) || ', bot ' || quote(NEW.agent_alias) || ': '
					|| CASE
						WHEN NOT EXISTS (SELECT 1 FROM gatehouse_workspaces WHERE alias = NEW.workspace_alias)
							THEN 'workspace is unavailable'
						WHEN NOT EXISTS (SELECT 1 FROM gatehouse_groups groups JOIN gatehouse_workspaces workspaces ON workspaces.id = groups.workspace_id WHERE workspaces.alias = NEW.workspace_alias AND groups.alias = NEW.group_alias)
							THEN 'group is unavailable in this workspace'
						ELSE 'bot is unavailable in this workspace'
					END);
			END;
			{{ range . }}{{ $group := . }}{{ range .Agents }}
			INSERT INTO gatehouse_migration_group_agents_desired VALUES (
				{{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.Alias }}, {{ sqlLiteral .AgentID }},
				{{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }}
			);
			{{ end }}{{ end }}
			DROP TRIGGER gatehouse_migration_group_agent_validate;
			CREATE TEMP TABLE gatehouse_migration_group_agent_state AS
			SELECT workspaces.id AS workspace_id, groups.id AS group_id, agents.id AS agent_id,
				desired.enabled, desired.revision, members.revision AS existing_revision
			FROM gatehouse_migration_group_agents_desired desired
			LEFT JOIN gatehouse_workspaces workspaces ON workspaces.alias = desired.workspace_alias
			LEFT JOIN gatehouse_groups groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.group_alias
			LEFT JOIN gatehouse_workspace_agents agents ON agents.workspace_id = workspaces.id AND agents.alias = desired.agent_alias
			LEFT JOIN gatehouse_group_agents members ON members.workspace_id = workspaces.id AND members.group_id = groups.id AND members.agent_id = agents.id;
			CREATE TEMP TABLE gatehouse_migration_group_agent_activities AS
			SELECT *, gh_id_new('act') AS id FROM gatehouse_migration_group_agent_state
			WHERE existing_revision IS NULL OR existing_revision < revision;
			INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision)
			SELECT workspace_id, group_id, agent_id, enabled, revision FROM gatehouse_migration_group_agent_state WHERE TRUE
			ON CONFLICT (workspace_id, group_id, agent_id) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_group_agents.revision < excluded.revision;
			INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_agent_group, resource_group_agent_agent, created_at)
			SELECT id, CASE WHEN existing_revision IS NULL THEN 'group_agent.create' ELSE 'group_agent.update' END,
				'group_agent', group_id, agent_id, gh_id_timestamp(id)
			FROM gatehouse_migration_group_agent_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, workspace_id || '/' || group_id FROM gatehouse_migration_group_agent_activities
			UNION ALL
			SELECT id, workspace_id || '/' || agent_id FROM gatehouse_migration_group_agent_activities
			UNION ALL
			SELECT activities.id, members.principal_id || '/' || activities.group_id
			FROM gatehouse_migration_group_agent_activities activities
			JOIN gatehouse_group_members members ON members.workspace_id = activities.workspace_id AND members.group_id = activities.group_id;
			DROP TABLE gatehouse_migration_group_agent_activities;
			DROP TABLE gatehouse_migration_group_agent_state;
			DROP TABLE gatehouse_migration_group_agents_desired;
		`, groups)
	}
}
