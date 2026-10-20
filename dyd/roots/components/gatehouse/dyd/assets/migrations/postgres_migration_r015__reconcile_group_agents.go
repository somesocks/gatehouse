package migrations

import (
	"context"

	"gatehouse/config"
)

func postgresMigrationR015ReconcileGroupAgents(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       15,
		Description: "reconcile_group_agents",
		Builder:     postgresMigrationR015ReconcileGroupAgentsBuilder(state.Groups),
	}
}

func postgresMigrationR015ReconcileGroupAgentsBuilder(groups []config.Group) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			CREATE TEMP TABLE gatehouse_migration_group_agents_desired (
				workspace_alias TEXT NOT NULL, group_alias TEXT NOT NULL, agent_alias TEXT NOT NULL,
				enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL,
				PRIMARY KEY (workspace_alias, group_alias, agent_alias)
			) ON COMMIT DROP;
			{{ range . }}{{ $group := . }}{{ range .Agents }}
			INSERT INTO gatehouse_migration_group_agents_desired VALUES (
				{{ sqlLiteral $group.WorkspaceID }}, {{ sqlLiteral $group.Alias }}, {{ sqlLiteral .AgentID }},
				{{ sqlBool .Enabled }}, {{ if .Revision }}{{ .Revision }}{{ else }}1{{ end }}
			);
			{{ end }}{{ end }}
			LOCK TABLE gatehouse_groups, gatehouse_workspace_agents, gatehouse_group_agents IN SHARE ROW EXCLUSIVE MODE;
			DO $$
			DECLARE missing RECORD;
			BEGIN
				SELECT desired.*, CASE
					WHEN workspaces.id IS NULL THEN 'workspace is unavailable'
					WHEN groups.id IS NULL THEN 'group is unavailable in this workspace'
					ELSE 'bot is unavailable in this workspace'
				END AS reason INTO missing
				FROM gatehouse_migration_group_agents_desired desired
				LEFT JOIN gatehouse_workspaces workspaces ON workspaces.alias = desired.workspace_alias
				LEFT JOIN gatehouse_groups groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.group_alias
				LEFT JOIN gatehouse_workspace_agents agents ON agents.workspace_id = workspaces.id AND agents.alias = desired.agent_alias
				WHERE workspaces.id IS NULL OR groups.id IS NULL OR agents.id IS NULL
				ORDER BY desired.workspace_alias, desired.group_alias, desired.agent_alias LIMIT 1;
				IF FOUND THEN
					RAISE EXCEPTION 'cannot reconcile group bot membership: workspace %, group %, bot %: %',
						quote_literal(missing.workspace_alias), quote_literal(missing.group_alias), quote_literal(missing.agent_alias), missing.reason
						USING ERRCODE = '23503';
				END IF;
			END;
			$$;
			CREATE FUNCTION pg_temp.gatehouse_group_agent_migration_id(at_time TIMESTAMPTZ) RETURNS TEXT AS $$
			DECLARE
				alphabet CONSTANT TEXT := '0123456789abcdefghjkmnpqrstvwxyz';
				milliseconds BIGINT := floor(extract(epoch FROM at_time) * 1000)::BIGINT;
				encoded TEXT := '';
				position INTEGER;
			BEGIN
				FOR position IN REVERSE 9..0 LOOP
					encoded := substr(alphabet, (milliseconds % 32)::INTEGER + 1, 1) || encoded;
					milliseconds := milliseconds / 32;
				END LOOP;
				FOR position IN 1..16 LOOP
					encoded := encoded || substr(alphabet, floor(random() * 32)::INTEGER + 1, 1);
				END LOOP;
				RETURN 'act_' || encoded;
			END;
			$$ LANGUAGE plpgsql VOLATILE;
			CREATE TEMP TABLE gatehouse_migration_group_agent_state ON COMMIT DROP AS
			SELECT workspaces.id AS workspace_id, groups.id AS group_id, agents.id AS agent_id,
				desired.enabled, desired.revision, members.revision AS existing_revision
			FROM gatehouse_migration_group_agents_desired desired
			LEFT JOIN gatehouse_workspaces workspaces ON workspaces.alias = desired.workspace_alias
			LEFT JOIN gatehouse_groups groups ON groups.workspace_id = workspaces.id AND groups.alias = desired.group_alias
			LEFT JOIN gatehouse_workspace_agents agents ON agents.workspace_id = workspaces.id AND agents.alias = desired.agent_alias
			LEFT JOIN gatehouse_group_agents members ON members.workspace_id = workspaces.id AND members.group_id = groups.id AND members.agent_id = agents.id;
			CREATE TEMP TABLE gatehouse_migration_group_agent_activities ON COMMIT DROP AS
			WITH changes AS MATERIALIZED (
				SELECT *, date_trunc('milliseconds', clock_timestamp()) AS event_time
				FROM gatehouse_migration_group_agent_state WHERE existing_revision IS NULL OR existing_revision < revision
			)
			SELECT *, pg_temp.gatehouse_group_agent_migration_id(event_time) AS id FROM changes;
			INSERT INTO gatehouse_group_agents (workspace_id, group_id, agent_id, enabled, revision)
			SELECT workspace_id, group_id, agent_id, enabled, revision FROM gatehouse_migration_group_agent_state
			ON CONFLICT (workspace_id, group_id, agent_id) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_group_agents.revision < excluded.revision;
			INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_group_agent_group, resource_group_agent_agent, created_at)
			SELECT id, CASE WHEN existing_revision IS NULL THEN 'group_agent.create' ELSE 'group_agent.update' END,
				'group_agent', group_id, agent_id, event_time
			FROM gatehouse_migration_group_agent_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, workspace_id || '/' || group_id FROM gatehouse_migration_group_agent_activities
			UNION ALL
			SELECT id, workspace_id || '/' || agent_id FROM gatehouse_migration_group_agent_activities
			UNION ALL
			SELECT activities.id, members.principal_id || '/' || activities.group_id
			FROM gatehouse_migration_group_agent_activities activities
			JOIN gatehouse_group_members members ON members.workspace_id = activities.workspace_id AND members.group_id = activities.group_id AND members.enabled = TRUE
			JOIN gatehouse_principals principal ON principal.id = members.principal_id AND principal.enabled = TRUE
			JOIN gatehouse_workspaces workspace ON workspace.id = members.workspace_id AND workspace.enabled = TRUE
			WHERE EXISTS (SELECT 1 FROM gatehouse_workspace_grants grants WHERE grants.workspace = members.workspace_id AND grants.principal = members.principal_id AND grants.enabled = TRUE);
			DROP FUNCTION pg_temp.gatehouse_group_agent_migration_id(TIMESTAMPTZ);
		`, groups)
	}
}
