package migrations

func sqliteMigrationV039CreateTasks() VersionedMigration {
	return VersionedMigration{
		Index:       39,
		Description: "create_tasks",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_project_tasks (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'ptk_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
				description TEXT CHECK (description IS NULL OR length(CAST(description AS BLOB)) <= 4096),
				sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)),
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'in_progress', 'done', 'cancelled')),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				creator_principal TEXT REFERENCES gatehouse_principals (id),
				creator_agent TEXT,
				creator_gateway TEXT CHECK (creator_gateway IS NULL OR (length(creator_gateway) = 30 AND substr(creator_gateway, 1, 4) = 'gwy_' AND substr(creator_gateway, 5, 1) GLOB '[0-7]' AND substr(creator_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				created_at TEXT NOT NULL,
				updater_principal TEXT REFERENCES gatehouse_principals (id),
				updater_agent TEXT,
				updater_gateway TEXT CHECK (updater_gateway IS NULL OR (length(updater_gateway) = 30 AND substr(updater_gateway, 1, 4) = 'gwy_' AND substr(updater_gateway, 5, 1) GLOB '[0-7]' AND substr(updater_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				updated_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, id),
				UNIQUE (id),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, creator_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				FOREIGN KEY (workspace, updater_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				CHECK (
					(creator_principal IS NOT NULL AND creator_agent IS NULL AND creator_gateway IS NULL)
					OR (creator_principal IS NULL AND creator_agent IS NOT NULL AND creator_gateway IS NULL)
					OR (creator_principal IS NULL AND creator_agent IS NULL AND creator_gateway IS NOT NULL)
				),
				CHECK (
					(updater_principal IS NOT NULL AND updater_agent IS NULL AND updater_gateway IS NULL)
					OR (updater_principal IS NULL AND updater_agent IS NOT NULL AND updater_gateway IS NULL)
					OR (updater_principal IS NULL AND updater_agent IS NULL AND updater_gateway IS NOT NULL)
				)
			) STRICT;
			CREATE INDEX gatehouse_project_tasks_by_project_created
			ON gatehouse_project_tasks (workspace, project, created_at DESC, id DESC);

			CREATE TABLE gatehouse_session_tasks (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'stk_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (length(CAST(title AS BLOB)) <= 256),
				description TEXT CHECK (description IS NULL OR length(CAST(description AS BLOB)) <= 4096),
				sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)),
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'in_progress', 'done', 'cancelled')),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				creator_principal TEXT REFERENCES gatehouse_principals (id),
				creator_agent TEXT,
				creator_gateway TEXT CHECK (creator_gateway IS NULL OR (length(creator_gateway) = 30 AND substr(creator_gateway, 1, 4) = 'gwy_' AND substr(creator_gateway, 5, 1) GLOB '[0-7]' AND substr(creator_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				created_at TEXT NOT NULL,
				updater_principal TEXT REFERENCES gatehouse_principals (id),
				updater_agent TEXT,
				updater_gateway TEXT CHECK (updater_gateway IS NULL OR (length(updater_gateway) = 30 AND substr(updater_gateway, 1, 4) = 'gwy_' AND substr(updater_gateway, 5, 1) GLOB '[0-7]' AND substr(updater_gateway, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*')),
				updated_at TEXT NOT NULL,
				PRIMARY KEY (workspace, session, id),
				UNIQUE (id),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, creator_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				FOREIGN KEY (workspace, updater_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				CHECK (
					(creator_principal IS NOT NULL AND creator_agent IS NULL AND creator_gateway IS NULL)
					OR (creator_principal IS NULL AND creator_agent IS NOT NULL AND creator_gateway IS NULL)
					OR (creator_principal IS NULL AND creator_agent IS NULL AND creator_gateway IS NOT NULL)
				),
				CHECK (
					(updater_principal IS NOT NULL AND updater_agent IS NULL AND updater_gateway IS NULL)
					OR (updater_principal IS NULL AND updater_agent IS NOT NULL AND updater_gateway IS NULL)
					OR (updater_principal IS NULL AND updater_agent IS NULL AND updater_gateway IS NOT NULL)
				)
			) STRICT;
			CREATE INDEX gatehouse_session_tasks_by_session_created
			ON gatehouse_session_tasks (workspace, session, created_at DESC, id DESC);

			CREATE TEMP TABLE gatehouse_activity_event_topics_migration_39 AS
			SELECT activity, topic FROM gatehouse_activity_event_topics;
			DROP TABLE gatehouse_activity_event_topics;
			ALTER TABLE gatehouse_activity_events RENAME TO gatehouse_activity_events_migration_39;
			CREATE TABLE gatehouse_activity_events (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				event TEXT NOT NULL CHECK (length(trim(event)) > 0),
				resource_kind TEXT NOT NULL CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_task', 'project_secret', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_task', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				resource_keychain_id TEXT,
				resource_keychain_version INTEGER,
				resource_agent_provider TEXT,
				resource_agent_model TEXT,
				resource_group TEXT,
				resource_group_member_group TEXT,
				resource_group_member_principal TEXT,
				resource_identity TEXT,
				resource_principal TEXT,
				resource_project TEXT,
				resource_project_file TEXT,
				resource_project_grant TEXT,
				resource_project_note TEXT,
				resource_project_task TEXT REFERENCES gatehouse_project_tasks (id),
				resource_project_secret TEXT,
				resource_session TEXT,
				resource_session_event TEXT,
				resource_session_file TEXT,
				resource_session_grant TEXT,
				resource_session_note TEXT,
				resource_session_task TEXT REFERENCES gatehouse_session_tasks (id),
				resource_session_secret TEXT,
				resource_storage_provider TEXT,
				resource_system_grant TEXT REFERENCES gatehouse_system_grants (id),
				resource_workspace TEXT REFERENCES gatehouse_workspaces (id),
				resource_workspace_agent_workspace TEXT,
				resource_workspace_agent_model TEXT,
				resource_workspace_grant TEXT,
				resource_workspace_storage_provider_workspace TEXT,
				resource_workspace_storage_provider_provider TEXT,
				created_at TEXT NOT NULL,
				FOREIGN KEY (resource_keychain_id, resource_keychain_version) REFERENCES gatehouse_keychains (id, version),
				FOREIGN KEY (resource_agent_provider) REFERENCES gatehouse_agent_providers (id),
				FOREIGN KEY (resource_agent_model) REFERENCES gatehouse_agent_models (id),
				FOREIGN KEY (resource_group) REFERENCES gatehouse_groups (id),
				FOREIGN KEY (resource_group_member_group, resource_group_member_principal) REFERENCES gatehouse_group_members (group_id, principal_id),
				FOREIGN KEY (resource_identity) REFERENCES gatehouse_identities (id),
				FOREIGN KEY (resource_principal) REFERENCES gatehouse_principals (id),
				FOREIGN KEY (resource_project) REFERENCES gatehouse_projects (id),
				FOREIGN KEY (resource_project_file) REFERENCES gatehouse_project_files (id),
				FOREIGN KEY (resource_project_grant) REFERENCES gatehouse_project_grants (id),
				FOREIGN KEY (resource_project_note) REFERENCES gatehouse_project_notes (id),
				FOREIGN KEY (resource_project_secret) REFERENCES gatehouse_project_secrets (id),
				FOREIGN KEY (resource_session) REFERENCES gatehouse_sessions (id),
				FOREIGN KEY (resource_session_event) REFERENCES gatehouse_session_events (id),
				FOREIGN KEY (resource_session_file) REFERENCES gatehouse_session_files (id),
				FOREIGN KEY (resource_session_grant) REFERENCES gatehouse_session_grants (id),
				FOREIGN KEY (resource_session_note) REFERENCES gatehouse_session_notes (id),
				FOREIGN KEY (resource_session_secret) REFERENCES gatehouse_session_secrets (id),
				FOREIGN KEY (resource_storage_provider) REFERENCES gatehouse_storage_providers (id),
				FOREIGN KEY (resource_workspace_agent_workspace, resource_workspace_agent_model) REFERENCES gatehouse_workspace_agents (workspace_id, model_id),
				FOREIGN KEY (resource_workspace_grant) REFERENCES gatehouse_workspace_grants (id),
				FOREIGN KEY (resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) REFERENCES gatehouse_workspace_storage_providers (workspace, provider),
				CHECK (
					CASE resource_kind
						WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL
						WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL
						WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_model IS NOT NULL
						WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL
						WHEN 'project_task' THEN resource_project_task IS NOT NULL
						WHEN 'session_task' THEN resource_session_task IS NOT NULL
						WHEN 'system_grant' THEN resource_system_grant IS NOT NULL
						WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL WHEN 'agent_model' THEN resource_agent_model IS NOT NULL WHEN 'group' THEN resource_group IS NOT NULL WHEN 'identity' THEN resource_identity IS NOT NULL WHEN 'principal' THEN resource_principal IS NOT NULL WHEN 'project' THEN resource_project IS NOT NULL WHEN 'project_file' THEN resource_project_file IS NOT NULL WHEN 'project_grant' THEN resource_project_grant IS NOT NULL WHEN 'project_note' THEN resource_project_note IS NOT NULL WHEN 'project_secret' THEN resource_project_secret IS NOT NULL WHEN 'session' THEN resource_session IS NOT NULL WHEN 'session_event' THEN resource_session_event IS NOT NULL WHEN 'session_file' THEN resource_session_file IS NOT NULL WHEN 'session_grant' THEN resource_session_grant IS NOT NULL WHEN 'session_note' THEN resource_session_note IS NOT NULL WHEN 'session_secret' THEN resource_session_secret IS NOT NULL WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL WHEN 'workspace' THEN resource_workspace IS NOT NULL WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL
					END
					AND (
					(resource_keychain_id IS NOT NULL) + (resource_keychain_version IS NOT NULL) + (resource_agent_provider IS NOT NULL) + (resource_agent_model IS NOT NULL) + (resource_group IS NOT NULL) + (resource_group_member_group IS NOT NULL) + (resource_group_member_principal IS NOT NULL) + (resource_identity IS NOT NULL) + (resource_principal IS NOT NULL) + (resource_project IS NOT NULL) + (resource_project_file IS NOT NULL) + (resource_project_grant IS NOT NULL) + (resource_project_note IS NOT NULL) + (resource_project_task IS NOT NULL) + (resource_project_secret IS NOT NULL) + (resource_session IS NOT NULL) + (resource_session_event IS NOT NULL) + (resource_session_file IS NOT NULL) + (resource_session_grant IS NOT NULL) + (resource_session_note IS NOT NULL) + (resource_session_task IS NOT NULL) + (resource_session_secret IS NOT NULL) + (resource_storage_provider IS NOT NULL) + (resource_system_grant IS NOT NULL) + (resource_workspace IS NOT NULL) + (resource_workspace_agent_workspace IS NOT NULL) + (resource_workspace_agent_model IS NOT NULL) + (resource_workspace_grant IS NOT NULL) + (resource_workspace_storage_provider_workspace IS NOT NULL) + (resource_workspace_storage_provider_provider IS NOT NULL)
					) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
				)
			) STRICT;
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_model, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at
			)
			SELECT id, event, resource_kind, resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_model, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at
			FROM gatehouse_activity_events_migration_39;
			DROP TABLE gatehouse_activity_events_migration_39;
			CREATE TABLE gatehouse_activity_event_topics (
				activity TEXT NOT NULL,
				topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
				PRIMARY KEY (activity, topic),
				FOREIGN KEY (activity) REFERENCES gatehouse_activity_events (id)
			) STRICT;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT activity, topic FROM gatehouse_activity_event_topics_migration_39;
			DROP TABLE gatehouse_activity_event_topics_migration_39;
			CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor ON gatehouse_activity_event_topics (topic, activity);
		`),
	}
}
