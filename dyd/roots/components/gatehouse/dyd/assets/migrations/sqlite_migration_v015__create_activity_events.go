package migrations

func sqliteMigrationV015CreateActivityEvents() VersionedMigration {
	return VersionedMigration{
		Index:       15,
		Description: "create_activity_events",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_activity_events (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				event TEXT NOT NULL CHECK (length(trim(event)) > 0),
				resource_kind TEXT NOT NULL CHECK (resource_kind IN ('project', 'session', 'session_event')),
				project TEXT,
				session TEXT,
				session_event TEXT,
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, id),
				UNIQUE (workspace, id, created_at),
				FOREIGN KEY (workspace, session)
					REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, project)
					REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, session, session_event)
					REFERENCES gatehouse_session_events (workspace, session, id),
				CHECK (
					(resource_kind = 'project' AND project IS NOT NULL AND session IS NULL AND session_event IS NULL)
					OR (resource_kind = 'session' AND session IS NOT NULL AND session_event IS NULL)
					OR (resource_kind = 'session_event' AND session IS NOT NULL AND session_event IS NOT NULL)
				)
			) STRICT;

			CREATE TABLE gatehouse_activity_event_topics (
				workspace TEXT NOT NULL,
				activity TEXT NOT NULL,
				topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
				created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, activity, topic),
				FOREIGN KEY (workspace, activity, created_at)
					REFERENCES gatehouse_activity_events (workspace, id, created_at)
			) STRICT;

			CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor
			ON gatehouse_activity_event_topics (workspace, topic, activity);
		`),
	}
}
