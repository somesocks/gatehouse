package migrations

func sqliteMigrationV061GroupAgents() VersionedMigration {
	return VersionedMigration{
		Index: 61, Description: "group_agents",
		Options: MigrationOptions{SQLite: SQLiteMigrationOptions{DisableForeignKeys: true}},
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_group_agents (
				workspace_id TEXT NOT NULL, group_id TEXT NOT NULL, agent_id TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
				PRIMARY KEY (workspace_id, group_id, agent_id), UNIQUE (group_id, agent_id),
				FOREIGN KEY (workspace_id, group_id) REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace_id, agent_id) REFERENCES gatehouse_workspace_agents (workspace_id, id)
			) STRICT;
			CREATE INDEX gatehouse_group_agents_by_agent ON gatehouse_group_agents (workspace_id, agent_id, group_id);
			CREATE TABLE gatehouse_activity_events_migration_61 (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				event TEXT NOT NULL CHECK (length(trim(event)) > 0), resource_kind TEXT NOT NULL,
				detail_identity TEXT,
				resource_keychain_id TEXT, resource_keychain_version INTEGER,
				resource_agent_provider TEXT REFERENCES gatehouse_agent_providers (id), resource_agent_model TEXT REFERENCES gatehouse_agent_models (id),
				resource_group TEXT REFERENCES gatehouse_groups (id), resource_group_member_group TEXT, resource_group_member_principal TEXT,
				resource_group_agent_group TEXT, resource_group_agent_agent TEXT,
				resource_principal TEXT REFERENCES gatehouse_principals (id),
				resource_project TEXT REFERENCES gatehouse_projects (id), resource_project_file TEXT REFERENCES gatehouse_project_files (id),
				resource_project_grant TEXT REFERENCES gatehouse_project_grants (id), resource_project_note TEXT REFERENCES gatehouse_project_notes (id),
				resource_project_task TEXT REFERENCES gatehouse_project_tasks (id), resource_project_secret TEXT REFERENCES gatehouse_project_secrets (id),
				resource_project_record_schema TEXT, resource_project_record_attribute TEXT, resource_project_record TEXT,
				resource_session TEXT REFERENCES gatehouse_sessions (id), resource_session_event TEXT REFERENCES gatehouse_session_events (id),
				resource_session_file TEXT REFERENCES gatehouse_session_files (id), resource_session_grant TEXT REFERENCES gatehouse_session_grants (id),
				resource_session_note TEXT REFERENCES gatehouse_session_notes (id), resource_session_task TEXT REFERENCES gatehouse_session_tasks (id),
				resource_session_secret TEXT REFERENCES gatehouse_session_secrets (id), resource_storage_provider TEXT REFERENCES gatehouse_storage_providers (id),
				resource_system_grant TEXT REFERENCES gatehouse_system_grants (id), resource_workspace TEXT REFERENCES gatehouse_workspaces (id),
				resource_workspace_agent_workspace TEXT, resource_workspace_agent_id TEXT, resource_workspace_grant TEXT REFERENCES gatehouse_workspace_grants (id),
				resource_workspace_storage_provider_workspace TEXT, resource_workspace_storage_provider_provider TEXT, created_at TEXT NOT NULL,
				FOREIGN KEY (resource_keychain_id, resource_keychain_version) REFERENCES gatehouse_keychains (id, version),
				FOREIGN KEY (resource_group_member_group, resource_group_member_principal) REFERENCES gatehouse_group_members (group_id, principal_id),
				FOREIGN KEY (resource_group_agent_group, resource_group_agent_agent) REFERENCES gatehouse_group_agents (group_id, agent_id),
				FOREIGN KEY (resource_workspace_agent_workspace, resource_workspace_agent_id) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				FOREIGN KEY (resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) REFERENCES gatehouse_workspace_storage_providers (workspace, provider),
				CHECK (
					CASE resource_kind
						WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL
						WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL
						WHEN 'group_agent' THEN resource_group_agent_group IS NOT NULL AND resource_group_agent_agent IS NOT NULL
						WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_id IS NOT NULL
						WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL
						WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL
						WHEN 'agent_model' THEN resource_agent_model IS NOT NULL
						WHEN 'group' THEN resource_group IS NOT NULL
						WHEN 'principal' THEN resource_principal IS NOT NULL
						WHEN 'project' THEN resource_project IS NOT NULL
						WHEN 'project_file' THEN resource_project_file IS NOT NULL
						WHEN 'project_grant' THEN resource_project_grant IS NOT NULL
						WHEN 'project_note' THEN resource_project_note IS NOT NULL
						WHEN 'project_task' THEN resource_project_task IS NOT NULL
						WHEN 'project_secret' THEN resource_project_secret IS NOT NULL
						WHEN 'project_record_schema' THEN resource_project_record_schema IS NOT NULL
						WHEN 'project_record_attribute' THEN resource_project_record_attribute IS NOT NULL
						WHEN 'project_record' THEN resource_project_record IS NOT NULL
						WHEN 'session' THEN resource_session IS NOT NULL
						WHEN 'session_event' THEN resource_session_event IS NOT NULL
						WHEN 'session_file' THEN resource_session_file IS NOT NULL
						WHEN 'session_grant' THEN resource_session_grant IS NOT NULL
						WHEN 'session_note' THEN resource_session_note IS NOT NULL
						WHEN 'session_task' THEN resource_session_task IS NOT NULL
						WHEN 'session_secret' THEN resource_session_secret IS NOT NULL
						WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL
						WHEN 'system_grant' THEN resource_system_grant IS NOT NULL
						WHEN 'workspace' THEN resource_workspace IS NOT NULL
						WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL
						ELSE FALSE
					END
					AND (
						(resource_keychain_id IS NOT NULL) + (resource_keychain_version IS NOT NULL)
						+ (resource_agent_provider IS NOT NULL) + (resource_agent_model IS NOT NULL)
						+ (resource_group IS NOT NULL) + (resource_group_member_group IS NOT NULL) + (resource_group_member_principal IS NOT NULL)
						+ (resource_group_agent_group IS NOT NULL) + (resource_group_agent_agent IS NOT NULL)
						+ (resource_principal IS NOT NULL) + (resource_project IS NOT NULL) + (resource_project_file IS NOT NULL)
						+ (resource_project_grant IS NOT NULL) + (resource_project_note IS NOT NULL) + (resource_project_task IS NOT NULL)
						+ (resource_project_secret IS NOT NULL) + (resource_project_record_schema IS NOT NULL)
						+ (resource_project_record_attribute IS NOT NULL) + (resource_project_record IS NOT NULL)
						+ (resource_session IS NOT NULL) + (resource_session_event IS NOT NULL) + (resource_session_file IS NOT NULL)
						+ (resource_session_grant IS NOT NULL) + (resource_session_note IS NOT NULL) + (resource_session_task IS NOT NULL)
						+ (resource_session_secret IS NOT NULL) + (resource_storage_provider IS NOT NULL) + (resource_system_grant IS NOT NULL)
						+ (resource_workspace IS NOT NULL) + (resource_workspace_agent_workspace IS NOT NULL) + (resource_workspace_agent_id IS NOT NULL)
						+ (resource_workspace_grant IS NOT NULL) + (resource_workspace_storage_provider_workspace IS NOT NULL) + (resource_workspace_storage_provider_provider IS NOT NULL)
					) = CASE resource_kind
						WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'group_agent' THEN 2
						WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
					AND (detail_identity IS NULL OR resource_kind = 'principal')
				)
			) STRICT;
			INSERT INTO gatehouse_activity_events_migration_61 (
				id, event, resource_kind, detail_identity, resource_keychain_id, resource_keychain_version,
				resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal,
				resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task,
				resource_project_secret, resource_project_record_schema, resource_project_record_attribute, resource_project_record,
				resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task,
				resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace,
				resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant,
				resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at
			)
			SELECT id, event, resource_kind, detail_identity, resource_keychain_id, resource_keychain_version,
				resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal,
				resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task,
				resource_project_secret, resource_project_record_schema, resource_project_record_attribute, resource_project_record,
				resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task,
				resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace,
				resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant,
				resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at
			FROM gatehouse_activity_events;
			DROP TABLE gatehouse_activity_events;
			ALTER TABLE gatehouse_activity_events_migration_61 RENAME TO gatehouse_activity_events;
		`),
	}
}
