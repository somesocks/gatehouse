package migrations

func sqliteMigrationV033AddGroupAndWorkspaceAgentActivitySubjects() VersionedMigration {
	return VersionedMigration{
		Index:       33,
		Description: "add_group_and_workspace_agent_activity_subjects",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_activity_event_topics_migration_33 (
					workspace TEXT NOT NULL,
					activity TEXT NOT NULL,
					topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
					created_at TEXT NOT NULL,
					PRIMARY KEY (workspace, activity, topic)
				) STRICT;

				INSERT INTO gatehouse_activity_event_topics_migration_33 (workspace, activity, topic, created_at)
				SELECT workspace, activity, topic, created_at FROM gatehouse_activity_event_topics;
				DROP TABLE gatehouse_activity_event_topics;

				CREATE TABLE gatehouse_activity_events_migration_33 (
					workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
					id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
					event TEXT NOT NULL CHECK (length(trim(event)) > 0),
					resource_kind TEXT NOT NULL CHECK (resource_kind IN ('project', 'session', 'session_event', 'group', 'group_member', 'workspace_agent')),
					project TEXT,
					session TEXT,
					session_event TEXT,
					"group" TEXT,
					principal TEXT,
					workspace_agent TEXT,
					created_at TEXT NOT NULL,
					PRIMARY KEY (workspace, id),
					UNIQUE (workspace, id, created_at),
					FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
					FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
					FOREIGN KEY (workspace, session, session_event) REFERENCES gatehouse_session_events (workspace, session, id),
					FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id),
					FOREIGN KEY (principal) REFERENCES gatehouse_principals (id),
					FOREIGN KEY (workspace, "group", principal) REFERENCES gatehouse_group_members (workspace_id, group_id, principal_id),
					FOREIGN KEY (workspace, workspace_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
					CHECK (
						(resource_kind = 'project' AND project IS NOT NULL AND session IS NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'session' AND session IS NOT NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'session_event' AND session IS NOT NULL AND session_event IS NOT NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'group' AND "group" IS NOT NULL AND project IS NULL AND session IS NULL AND session_event IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'group_member' AND "group" IS NOT NULL AND principal IS NOT NULL AND project IS NULL AND session IS NULL AND session_event IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'workspace_agent' AND workspace_agent IS NOT NULL AND project IS NULL AND session IS NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL)
					)
				) STRICT;

				INSERT INTO gatehouse_activity_events_migration_33 (
					workspace, id, event, resource_kind, project, session, session_event, created_at
				)
				SELECT workspace, id, event, resource_kind, project, session, session_event, created_at
				FROM gatehouse_activity_events;
				DROP TABLE gatehouse_activity_events;
				ALTER TABLE gatehouse_activity_events_migration_33 RENAME TO gatehouse_activity_events;

				CREATE TABLE gatehouse_activity_event_topics (
					workspace TEXT NOT NULL,
					activity TEXT NOT NULL,
					topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
					created_at TEXT NOT NULL,
					PRIMARY KEY (workspace, activity, topic),
					FOREIGN KEY (workspace, activity, created_at)
						REFERENCES gatehouse_activity_events (workspace, id, created_at)
				) STRICT;
				INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
				SELECT workspace, activity, topic, created_at FROM gatehouse_activity_event_topics_migration_33;
				DROP TABLE gatehouse_activity_event_topics_migration_33;
				CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor
				ON gatehouse_activity_event_topics (workspace, topic, activity);
		`),
	}
}
