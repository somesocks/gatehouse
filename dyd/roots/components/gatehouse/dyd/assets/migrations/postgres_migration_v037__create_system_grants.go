package migrations

func postgresMigrationV037CreateSystemGrants() VersionedMigration {
	return VersionedMigration{
		Index:       37,
		Description: "create_system_grants",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_system_grants (
				id TEXT PRIMARY KEY NOT NULL CHECK (id ~ '^syg_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				principal TEXT NOT NULL UNIQUE REFERENCES gatehouse_principals (id),
				role TEXT NOT NULL CHECK (role = 'manager'),
				enabled BOOLEAN NOT NULL,
				revision BIGINT NOT NULL CHECK (revision > 0)
			);
			ALTER TABLE gatehouse_activity_events
				ADD COLUMN resource_system_grant TEXT REFERENCES gatehouse_system_grants (id),
				DROP CONSTRAINT gatehouse_activity_events_resource_kind_check,
				DROP CONSTRAINT gatehouse_activity_events_check,
				ADD CONSTRAINT gatehouse_activity_events_resource_kind_check CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_secret', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				ADD CONSTRAINT gatehouse_activity_events_check CHECK (
					CASE resource_kind
						WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL
						WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL
						WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_model IS NOT NULL
						WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL
						WHEN 'system_grant' THEN resource_system_grant IS NOT NULL
						WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL WHEN 'agent_model' THEN resource_agent_model IS NOT NULL WHEN 'group' THEN resource_group IS NOT NULL WHEN 'identity' THEN resource_identity IS NOT NULL WHEN 'principal' THEN resource_principal IS NOT NULL WHEN 'project' THEN resource_project IS NOT NULL WHEN 'project_file' THEN resource_project_file IS NOT NULL WHEN 'project_grant' THEN resource_project_grant IS NOT NULL WHEN 'project_note' THEN resource_project_note IS NOT NULL WHEN 'project_secret' THEN resource_project_secret IS NOT NULL WHEN 'session' THEN resource_session IS NOT NULL WHEN 'session_event' THEN resource_session_event IS NOT NULL WHEN 'session_file' THEN resource_session_file IS NOT NULL WHEN 'session_grant' THEN resource_session_grant IS NOT NULL WHEN 'session_note' THEN resource_session_note IS NOT NULL WHEN 'session_secret' THEN resource_session_secret IS NOT NULL WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL WHEN 'workspace' THEN resource_workspace IS NOT NULL WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL
					END
					AND num_nonnulls(resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_model, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
				);
		`),
	}
}
