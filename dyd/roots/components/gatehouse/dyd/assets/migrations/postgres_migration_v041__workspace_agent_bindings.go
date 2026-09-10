package migrations

func postgresMigrationV041WorkspaceAgentBindings() VersionedMigration {
	return VersionedMigration{
		Index:       41,
		Description: "workspace_agent_bindings",
		Builder: staticMigrationBuilder(`
			CREATE FUNCTION gatehouse_migration_typed_id_new(kind TEXT) RETURNS TEXT AS $$
			DECLARE
				alphabet CONSTANT TEXT := '0123456789abcdefghjkmnpqrstvwxyz';
				milliseconds BIGINT := floor(extract(epoch FROM clock_timestamp()) * 1000)::BIGINT;
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
				RETURN kind || '_' || encoded;
			END;
			$$ LANGUAGE plpgsql VOLATILE;

			-- The old composite key is referenced from several historical tables.
			-- Remove those references before replacing it, then restore each one below.
			DO $$ DECLARE constraint_record RECORD; BEGIN
				FOR constraint_record IN SELECT conrelid::regclass AS table_name, conname FROM pg_constraint WHERE contype = 'f' AND confrelid = 'gatehouse_workspace_agents'::regclass LOOP
					EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', constraint_record.table_name, constraint_record.conname);
				END LOOP;
			END $$;
			ALTER TABLE gatehouse_workspace_agents ADD COLUMN id TEXT;
			ALTER TABLE gatehouse_workspace_agents ADD COLUMN alias TEXT;
			UPDATE gatehouse_workspace_agents AS bindings
			SET id = gatehouse_migration_typed_id_new('wag'), alias = COALESCE((SELECT alias FROM gatehouse_agent_models WHERE id = bindings.model_id), bindings.model_id);
			ALTER TABLE gatehouse_workspace_agents ALTER COLUMN id SET NOT NULL;
			ALTER TABLE gatehouse_workspace_agents ALTER COLUMN alias SET NOT NULL;
			ALTER TABLE gatehouse_workspace_agents ADD CONSTRAINT gatehouse_workspace_agents_id_check CHECK (id ~ '^wag_[0-7][0-9a-hjkmnp-tv-z]{25}$');
			ALTER TABLE gatehouse_workspace_agents ADD CONSTRAINT gatehouse_workspace_agents_alias_check CHECK (alias ~ '^[a-z][a-z0-9_-]*$');
			ALTER TABLE gatehouse_workspace_agents DROP CONSTRAINT gatehouse_workspace_agents_pkey;
			ALTER TABLE gatehouse_workspace_agents ADD PRIMARY KEY (id);
			ALTER TABLE gatehouse_workspace_agents ADD UNIQUE (workspace_id, id);
			ALTER TABLE gatehouse_workspace_agents ADD UNIQUE (workspace_id, alias);

			ALTER TABLE gatehouse_project_note_revisions DISABLE TRIGGER gatehouse_project_note_revisions_immutable;
			ALTER TABLE gatehouse_session_note_revisions DISABLE TRIGGER gatehouse_session_note_revisions_immutable;
			UPDATE gatehouse_sessions SET author_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_sessions.workspace AND bindings.model_id = gatehouse_sessions.author_agent) WHERE author_agent IS NOT NULL;
			UPDATE gatehouse_session_events SET author_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_session_events.workspace AND bindings.model_id = gatehouse_session_events.author_agent) WHERE author_agent IS NOT NULL;
			UPDATE gatehouse_agent_contexts SET model = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_agent_contexts.workspace AND bindings.model_id = gatehouse_agent_contexts.model);
			UPDATE gatehouse_project_notes SET author_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_project_notes.workspace AND bindings.model_id = gatehouse_project_notes.author_agent) WHERE author_agent IS NOT NULL;
			UPDATE gatehouse_session_notes SET author_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_session_notes.workspace AND bindings.model_id = gatehouse_session_notes.author_agent) WHERE author_agent IS NOT NULL;
			UPDATE gatehouse_project_note_revisions SET author_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_project_note_revisions.workspace AND bindings.model_id = gatehouse_project_note_revisions.author_agent) WHERE author_agent IS NOT NULL;
			UPDATE gatehouse_session_note_revisions SET author_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_session_note_revisions.workspace AND bindings.model_id = gatehouse_session_note_revisions.author_agent) WHERE author_agent IS NOT NULL;
			UPDATE gatehouse_project_tasks SET creator_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_project_tasks.workspace AND bindings.model_id = gatehouse_project_tasks.creator_agent) WHERE creator_agent IS NOT NULL;
			UPDATE gatehouse_project_tasks SET updater_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_project_tasks.workspace AND bindings.model_id = gatehouse_project_tasks.updater_agent) WHERE updater_agent IS NOT NULL;
			UPDATE gatehouse_session_tasks SET creator_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_session_tasks.workspace AND bindings.model_id = gatehouse_session_tasks.creator_agent) WHERE creator_agent IS NOT NULL;
			UPDATE gatehouse_session_tasks SET updater_agent = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_session_tasks.workspace AND bindings.model_id = gatehouse_session_tasks.updater_agent) WHERE updater_agent IS NOT NULL;
			ALTER TABLE gatehouse_project_note_revisions ENABLE TRIGGER gatehouse_project_note_revisions_immutable;
			ALTER TABLE gatehouse_session_note_revisions ENABLE TRIGGER gatehouse_session_note_revisions_immutable;
			ALTER TABLE gatehouse_activity_events DROP CONSTRAINT gatehouse_activity_events_resource_kind_check;
			ALTER TABLE gatehouse_activity_events DROP CONSTRAINT gatehouse_activity_events_check;
			ALTER TABLE gatehouse_activity_events RENAME COLUMN resource_workspace_agent_model TO resource_workspace_agent_id;
			UPDATE gatehouse_activity_events SET resource_workspace_agent_id = (SELECT id FROM gatehouse_workspace_agents AS bindings WHERE bindings.workspace_id = gatehouse_activity_events.resource_workspace_agent_workspace AND bindings.model_id = gatehouse_activity_events.resource_workspace_agent_id) WHERE resource_kind = 'workspace_agent';

			ALTER TABLE gatehouse_sessions ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_session_events ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_agent_contexts ADD FOREIGN KEY (workspace, model) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_project_notes ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_session_notes ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_project_note_revisions ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_session_note_revisions ADD FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_project_tasks ADD FOREIGN KEY (workspace, creator_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_project_tasks ADD FOREIGN KEY (workspace, updater_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_session_tasks ADD FOREIGN KEY (workspace, creator_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_session_tasks ADD FOREIGN KEY (workspace, updater_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_activity_events ADD FOREIGN KEY (resource_workspace_agent_workspace, resource_workspace_agent_id) REFERENCES gatehouse_workspace_agents (workspace_id, id);
			ALTER TABLE gatehouse_activity_events
				ADD CONSTRAINT gatehouse_activity_events_resource_kind_check CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_task', 'project_secret', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_task', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				ADD CONSTRAINT gatehouse_activity_events_check CHECK (
					CASE resource_kind
						WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL
						WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL
						WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_id IS NOT NULL
						WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL
						WHEN 'project_task' THEN resource_project_task IS NOT NULL
						WHEN 'session_task' THEN resource_session_task IS NOT NULL
						WHEN 'system_grant' THEN resource_system_grant IS NOT NULL
						WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL WHEN 'agent_model' THEN resource_agent_model IS NOT NULL WHEN 'group' THEN resource_group IS NOT NULL WHEN 'identity' THEN resource_identity IS NOT NULL WHEN 'principal' THEN resource_principal IS NOT NULL WHEN 'project' THEN resource_project IS NOT NULL WHEN 'project_file' THEN resource_project_file IS NOT NULL WHEN 'project_grant' THEN resource_project_grant IS NOT NULL WHEN 'project_note' THEN resource_project_note IS NOT NULL WHEN 'project_secret' THEN resource_project_secret IS NOT NULL WHEN 'session' THEN resource_session IS NOT NULL WHEN 'session_event' THEN resource_session_event IS NOT NULL WHEN 'session_file' THEN resource_session_file IS NOT NULL WHEN 'session_grant' THEN resource_session_grant IS NOT NULL WHEN 'session_note' THEN resource_session_note IS NOT NULL WHEN 'session_secret' THEN resource_session_secret IS NOT NULL WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL WHEN 'workspace' THEN resource_workspace IS NOT NULL WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL
					END
					AND num_nonnulls(resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
				);
			DROP FUNCTION gatehouse_migration_typed_id_new(TEXT);
		`),
	}
}
