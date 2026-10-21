package migrations

func sqliteMigrationV068GroupSecrets() VersionedMigration {
	const columns = `id, event, resource_kind, detail_identity, resource_keychain_id, resource_keychain_version,
		resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal,
		resource_group_agent_group, resource_group_agent_agent,
		resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task,
		resource_project_record_schema, resource_project_record_attribute, resource_project_record,
		resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task,
		resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace,
		resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant,
		resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at`
	return VersionedMigration{
		Index: 68, Description: "group_secrets",
		Options: MigrationOptions{SQLite: SQLiteMigrationOptions{DisableForeignKeys: true}},
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_group_secrets (
				workspace TEXT NOT NULL, "group" TEXT NOT NULL,
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'gsc_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				author_principal TEXT NOT NULL REFERENCES gatehouse_principals (id),
				description TEXT NOT NULL CHECK (length(trim(description)) > 0 AND length(CAST(description AS BLOB)) <= 4096),
				ciphertext TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL CHECK (revision > 0),
				created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
				PRIMARY KEY (workspace, "group", id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id),
				CHECK (enabled = FALSE OR length(ciphertext) > 0)
			) STRICT;
			CREATE TABLE gatehouse_activity_events_migration_68 (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				event TEXT NOT NULL CHECK (length(trim(event)) > 0), resource_kind TEXT NOT NULL,
				detail_identity TEXT,
				resource_keychain_id TEXT, resource_keychain_version INTEGER,
				resource_agent_provider TEXT REFERENCES gatehouse_agent_providers (id), resource_agent_model TEXT REFERENCES gatehouse_agent_models (id),
				resource_group TEXT REFERENCES gatehouse_groups (id), resource_group_member_group TEXT, resource_group_member_principal TEXT,
				resource_group_agent_group TEXT, resource_group_agent_agent TEXT,
				resource_group_secret TEXT REFERENCES gatehouse_group_secrets (id),
				resource_principal TEXT REFERENCES gatehouse_principals (id),
				resource_project TEXT REFERENCES gatehouse_projects (id), resource_project_file TEXT REFERENCES gatehouse_project_files (id),
				resource_project_grant TEXT REFERENCES gatehouse_project_grants (id), resource_project_note TEXT REFERENCES gatehouse_project_notes (id),
				resource_project_task TEXT REFERENCES gatehouse_project_tasks (id),
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
				CHECK (` + groupSecretsV068ActivityCheck(false) + `)
			) STRICT;
			INSERT INTO gatehouse_activity_events_migration_68 (` + columns + `) SELECT ` + columns + ` FROM gatehouse_activity_events;
			DROP TABLE gatehouse_activity_events;
			ALTER TABLE gatehouse_activity_events_migration_68 RENAME TO gatehouse_activity_events;
		`),
	}
}
