package migrations

func postgresMigrationV039CreateTasks() VersionedMigration {
	return VersionedMigration{
		Index:       39,
		Description: "create_tasks",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_project_tasks (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^ptk_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (octet_length(title) <= 256),
				description TEXT CHECK (description IS NULL OR octet_length(description) <= 4096),
				sensitive BOOLEAN NOT NULL,
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'in_progress', 'done', 'cancelled')),
				enabled BOOLEAN NOT NULL,
				creator_principal TEXT REFERENCES gatehouse_principals (id),
				creator_agent TEXT,
				creator_gateway TEXT CHECK (creator_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				created_at TIMESTAMPTZ NOT NULL,
				updater_principal TEXT REFERENCES gatehouse_principals (id),
				updater_agent TEXT,
				updater_gateway TEXT CHECK (updater_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				updated_at TIMESTAMPTZ NOT NULL,
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
			);
			CREATE INDEX gatehouse_project_tasks_by_project_created
			ON gatehouse_project_tasks (workspace, project, created_at DESC, id DESC);

			CREATE TABLE gatehouse_session_tasks (
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^stk_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				title TEXT NOT NULL CHECK (length(trim(title)) > 0) CHECK (octet_length(title) <= 256),
				description TEXT CHECK (description IS NULL OR octet_length(description) <= 4096),
				sensitive BOOLEAN NOT NULL,
				status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'in_progress', 'done', 'cancelled')),
				enabled BOOLEAN NOT NULL,
				creator_principal TEXT REFERENCES gatehouse_principals (id),
				creator_agent TEXT,
				creator_gateway TEXT CHECK (creator_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				created_at TIMESTAMPTZ NOT NULL,
				updater_principal TEXT REFERENCES gatehouse_principals (id),
				updater_agent TEXT,
				updater_gateway TEXT CHECK (updater_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				updated_at TIMESTAMPTZ NOT NULL,
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
			);
			CREATE INDEX gatehouse_session_tasks_by_session_created
			ON gatehouse_session_tasks (workspace, session, created_at DESC, id DESC);

			ALTER TABLE gatehouse_activity_events
				ADD COLUMN resource_project_task TEXT REFERENCES gatehouse_project_tasks (id),
				ADD COLUMN resource_session_task TEXT REFERENCES gatehouse_session_tasks (id),
				DROP CONSTRAINT gatehouse_activity_events_resource_kind_check,
				DROP CONSTRAINT gatehouse_activity_events_check,
				ADD CONSTRAINT gatehouse_activity_events_resource_kind_check CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_task', 'project_secret', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_task', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				ADD CONSTRAINT gatehouse_activity_events_check CHECK (
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
					AND num_nonnulls(resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_model, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
				);
		`),
	}
}
