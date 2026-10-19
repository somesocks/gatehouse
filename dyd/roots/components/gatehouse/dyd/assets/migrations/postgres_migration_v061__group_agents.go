package migrations

func postgresMigrationV061GroupAgents() VersionedMigration {
	return VersionedMigration{Index: 61, Description: "group_agents", Builder: staticMigrationBuilder(`
		CREATE TABLE gatehouse_group_agents (
			workspace_id TEXT NOT NULL, group_id TEXT NOT NULL, agent_id TEXT NOT NULL,
			enabled BOOLEAN NOT NULL, revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
			PRIMARY KEY (workspace_id, group_id, agent_id), UNIQUE (group_id, agent_id),
			FOREIGN KEY (workspace_id, group_id) REFERENCES gatehouse_groups (workspace_id, id),
			FOREIGN KEY (workspace_id, agent_id) REFERENCES gatehouse_workspace_agents (workspace_id, id)
		);
		CREATE INDEX gatehouse_group_agents_by_agent ON gatehouse_group_agents (workspace_id, agent_id, group_id);
		ALTER TABLE gatehouse_activity_events
			ADD COLUMN resource_group_agent_group TEXT,
			ADD COLUMN resource_group_agent_agent TEXT,
			ADD FOREIGN KEY (resource_group_agent_group, resource_group_agent_agent) REFERENCES gatehouse_group_agents (group_id, agent_id),
			DROP CONSTRAINT gatehouse_activity_events_resource_kind_check,
			DROP CONSTRAINT gatehouse_activity_events_check,
			ADD CONSTRAINT gatehouse_activity_events_check CHECK (
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
				AND num_nonnulls(
					resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model,
					resource_group, resource_group_member_group, resource_group_member_principal,
					resource_group_agent_group, resource_group_agent_agent,
					resource_principal, resource_project, resource_project_file, resource_project_grant,
					resource_project_note, resource_project_task, resource_project_secret,
					resource_project_record_schema, resource_project_record_attribute, resource_project_record,
					resource_session, resource_session_event, resource_session_file, resource_session_grant,
					resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider,
					resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id,
					resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider
				) = CASE resource_kind
					WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'group_agent' THEN 2
					WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
				AND (detail_identity IS NULL OR resource_kind = 'principal')
			);
	`)}
}
