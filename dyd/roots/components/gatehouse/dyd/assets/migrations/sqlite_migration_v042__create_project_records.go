package migrations

func sqliteMigrationV042CreateProjectRecords() VersionedMigration {
	return VersionedMigration{
		Index:       42,
		Description: "create_project_records",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_project_record_schemas (
				workspace TEXT NOT NULL, project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prs_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT NOT NULL CHECK (name GLOB '[a-z]*') CHECK (name NOT GLOB '*[^a-z0-9_]*'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (length(CAST(label AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, id), UNIQUE (id), UNIQUE (workspace, project, name),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			CREATE INDEX gatehouse_project_record_schemas_by_project_created ON gatehouse_project_record_schemas (workspace, project, created_at DESC, id DESC);

			CREATE TABLE gatehouse_project_record_attributes (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pra_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				name TEXT NOT NULL CHECK (name GLOB '[a-z]*') CHECK (name NOT GLOB '*[^a-z0-9_]*'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (length(CAST(label AS BLOB)) <= 256),
				description TEXT NOT NULL CHECK (length(CAST(description AS BLOB)) <= 4096),
				type TEXT NOT NULL CHECK (type IN ('text', 'number', 'boolean', 'datetime', 'record')),
				target_schema TEXT,
				cardinality TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
				uniqueness TEXT NOT NULL CHECK (uniqueness IN ('none', 'record', 'global')),
				display TEXT NOT NULL CHECK (display IN ('none', 'primary', 'secondary')),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, id), UNIQUE (id), UNIQUE (workspace, project, schema, name),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, project, target_schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((type = 'record' AND target_schema IS NOT NULL) OR (type <> 'record' AND target_schema IS NULL)),
				CHECK (NOT (cardinality = 'one' AND uniqueness = 'record')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			CREATE INDEX gatehouse_project_record_attributes_by_schema_name ON gatehouse_project_record_attributes (workspace, project, schema, name);

			CREATE TABLE gatehouse_project_records (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, id), UNIQUE (id),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			CREATE INDEX gatehouse_project_records_by_schema_created ON gatehouse_project_records (workspace, project, schema, created_at DESC, id DESC);

			CREATE TABLE gatehouse_project_record_values (
				workspace TEXT NOT NULL, project TEXT NOT NULL, schema TEXT NOT NULL, record TEXT NOT NULL,
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'prv_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				attribute TEXT NOT NULL,
				value_type TEXT NOT NULL CHECK (value_type IN ('text', 'number', 'boolean', 'datetime', 'record')),
				value_text TEXT, value_number REAL, value_boolean INTEGER CHECK (value_boolean IN (0, 1)), value_datetime TEXT, value_reference TEXT,
				value_key TEXT NOT NULL, attribute_cardinality TEXT NOT NULL CHECK (attribute_cardinality IN ('one', 'many')), attribute_uniqueness TEXT NOT NULL CHECK (attribute_uniqueness IN ('none', 'record', 'global')),
				sensitive INTEGER NOT NULL CHECK (sensitive IN (0, 1)), author_principal TEXT REFERENCES gatehouse_principals (id), author_agent TEXT, created_at TEXT NOT NULL,
				PRIMARY KEY (workspace, project, schema, record, id), UNIQUE (id),
				FOREIGN KEY (workspace, project, schema, record) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute) REFERENCES gatehouse_project_record_attributes (workspace, project, schema, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((value_type = 'text' AND value_text IS NOT NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference IS NULL) OR (value_type = 'number' AND value_text IS NULL AND value_number IS NOT NULL AND value_number = value_number AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference IS NULL) OR (value_type = 'boolean' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NOT NULL AND value_datetime IS NULL AND value_reference IS NULL) OR (value_type = 'datetime' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NOT NULL AND value_reference IS NULL) OR (value_type = 'record' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference IS NOT NULL)),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			) STRICT;
			CREATE UNIQUE INDEX gatehouse_project_record_values_one ON gatehouse_project_record_values (workspace, project, schema, record, attribute) WHERE attribute_cardinality = 'one';
			CREATE UNIQUE INDEX gatehouse_project_record_values_record_unique ON gatehouse_project_record_values (workspace, project, schema, record, attribute, value_key) WHERE attribute_uniqueness = 'record';
			CREATE UNIQUE INDEX gatehouse_project_record_values_global_unique ON gatehouse_project_record_values (workspace, project, schema, attribute, value_key) WHERE attribute_uniqueness = 'global';
			CREATE INDEX gatehouse_project_record_values_by_record_created ON gatehouse_project_record_values (workspace, project, schema, record, created_at DESC, id DESC);

			CREATE TRIGGER gatehouse_project_record_values_validate_insert
			BEFORE INSERT ON gatehouse_project_record_values BEGIN
				SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM gatehouse_project_record_attributes WHERE workspace = NEW.workspace AND project = NEW.project AND schema = NEW.schema AND id = NEW.attribute AND type = NEW.value_type AND cardinality = NEW.attribute_cardinality AND uniqueness = NEW.attribute_uniqueness) THEN RAISE(ABORT, 'project record value does not match attribute') END;
				SELECT CASE WHEN NEW.sensitive = 1 AND NEW.attribute_uniqueness = 'global' THEN RAISE(ABORT, 'sensitive values cannot be globally unique') END;
				SELECT CASE WHEN NEW.value_type = 'record' AND NOT EXISTS (SELECT 1 FROM gatehouse_project_record_attributes AS attributes JOIN gatehouse_project_records AS records ON records.workspace = NEW.workspace AND records.project = NEW.project AND records.schema = attributes.target_schema AND records.id = NEW.value_reference WHERE attributes.workspace = NEW.workspace AND attributes.project = NEW.project AND attributes.schema = NEW.schema AND attributes.id = NEW.attribute) THEN RAISE(ABORT, 'project record reference target does not match attribute schema') END;
			END;
			CREATE TRIGGER gatehouse_project_record_attributes_validate_change
			BEFORE UPDATE OF type, target_schema, cardinality, uniqueness ON gatehouse_project_record_attributes
			WHEN EXISTS (SELECT 1 FROM gatehouse_project_record_values WHERE workspace = OLD.workspace AND project = OLD.project AND schema = OLD.schema AND attribute = OLD.id)
			BEGIN SELECT RAISE(ABORT, 'project record attribute structure cannot change while values exist'); END;

			ALTER TABLE gatehouse_agent_tasks__session_event_reply ADD COLUMN request_principal TEXT REFERENCES gatehouse_principals (id);
			ALTER TABLE gatehouse_agent_tasks__session_event_reply ADD COLUMN request_project TEXT;
			ALTER TABLE gatehouse_agent_tasks__session_event_reply ADD COLUMN request_project_actions TEXT;

			CREATE TEMP TABLE gatehouse_activity_event_topics_migration_42 AS SELECT activity, topic FROM gatehouse_activity_event_topics;
			DROP TABLE gatehouse_activity_event_topics;
			ALTER TABLE gatehouse_activity_events RENAME TO gatehouse_activity_events_migration_42;
			CREATE TABLE gatehouse_activity_events (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'act_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'), event TEXT NOT NULL CHECK (length(trim(event)) > 0),
				resource_kind TEXT NOT NULL CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_task', 'project_secret', 'project_record_schema', 'project_record_attribute', 'project_record', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_task', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				resource_keychain_id TEXT, resource_keychain_version INTEGER, resource_agent_provider TEXT, resource_agent_model TEXT, resource_group TEXT, resource_group_member_group TEXT, resource_group_member_principal TEXT, resource_identity TEXT, resource_principal TEXT, resource_project TEXT, resource_project_file TEXT, resource_project_grant TEXT, resource_project_note TEXT, resource_project_task TEXT REFERENCES gatehouse_project_tasks (id), resource_project_secret TEXT, resource_project_record_schema TEXT, resource_project_record_attribute TEXT, resource_project_record TEXT, resource_session TEXT, resource_session_event TEXT, resource_session_file TEXT, resource_session_grant TEXT, resource_session_note TEXT, resource_session_task TEXT REFERENCES gatehouse_session_tasks (id), resource_session_secret TEXT, resource_storage_provider TEXT, resource_system_grant TEXT REFERENCES gatehouse_system_grants (id), resource_workspace TEXT REFERENCES gatehouse_workspaces (id), resource_workspace_agent_workspace TEXT, resource_workspace_agent_id TEXT, resource_workspace_grant TEXT, resource_workspace_storage_provider_workspace TEXT, resource_workspace_storage_provider_provider TEXT, created_at TEXT NOT NULL,
				FOREIGN KEY (resource_keychain_id, resource_keychain_version) REFERENCES gatehouse_keychains (id, version), FOREIGN KEY (resource_agent_provider) REFERENCES gatehouse_agent_providers (id), FOREIGN KEY (resource_agent_model) REFERENCES gatehouse_agent_models (id), FOREIGN KEY (resource_group) REFERENCES gatehouse_groups (id), FOREIGN KEY (resource_group_member_group, resource_group_member_principal) REFERENCES gatehouse_group_members (group_id, principal_id), FOREIGN KEY (resource_identity) REFERENCES gatehouse_identities (id), FOREIGN KEY (resource_principal) REFERENCES gatehouse_principals (id), FOREIGN KEY (resource_project) REFERENCES gatehouse_projects (id), FOREIGN KEY (resource_project_file) REFERENCES gatehouse_project_files (id), FOREIGN KEY (resource_project_grant) REFERENCES gatehouse_project_grants (id), FOREIGN KEY (resource_project_note) REFERENCES gatehouse_project_notes (id), FOREIGN KEY (resource_project_secret) REFERENCES gatehouse_project_secrets (id), FOREIGN KEY (resource_session) REFERENCES gatehouse_sessions (id), FOREIGN KEY (resource_session_event) REFERENCES gatehouse_session_events (id), FOREIGN KEY (resource_session_file) REFERENCES gatehouse_session_files (id), FOREIGN KEY (resource_session_grant) REFERENCES gatehouse_session_grants (id), FOREIGN KEY (resource_session_note) REFERENCES gatehouse_session_notes (id), FOREIGN KEY (resource_session_secret) REFERENCES gatehouse_session_secrets (id), FOREIGN KEY (resource_storage_provider) REFERENCES gatehouse_storage_providers (id), FOREIGN KEY (resource_workspace_grant) REFERENCES gatehouse_workspace_grants (id), FOREIGN KEY (resource_workspace_agent_workspace, resource_workspace_agent_id) REFERENCES gatehouse_workspace_agents (workspace_id, id), FOREIGN KEY (resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) REFERENCES gatehouse_workspace_storage_providers (workspace, provider),
				CHECK (CASE resource_kind WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_id IS NOT NULL WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL WHEN 'project_record_schema' THEN resource_project_record_schema IS NOT NULL WHEN 'project_record_attribute' THEN resource_project_record_attribute IS NOT NULL WHEN 'project_record' THEN resource_project_record IS NOT NULL WHEN 'project_task' THEN resource_project_task IS NOT NULL WHEN 'session_task' THEN resource_session_task IS NOT NULL WHEN 'system_grant' THEN resource_system_grant IS NOT NULL WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL WHEN 'agent_model' THEN resource_agent_model IS NOT NULL WHEN 'group' THEN resource_group IS NOT NULL WHEN 'identity' THEN resource_identity IS NOT NULL WHEN 'principal' THEN resource_principal IS NOT NULL WHEN 'project' THEN resource_project IS NOT NULL WHEN 'project_file' THEN resource_project_file IS NOT NULL WHEN 'project_grant' THEN resource_project_grant IS NOT NULL WHEN 'project_note' THEN resource_project_note IS NOT NULL WHEN 'project_secret' THEN resource_project_secret IS NOT NULL WHEN 'session' THEN resource_session IS NOT NULL WHEN 'session_event' THEN resource_session_event IS NOT NULL WHEN 'session_file' THEN resource_session_file IS NOT NULL WHEN 'session_grant' THEN resource_session_grant IS NOT NULL WHEN 'session_note' THEN resource_session_note IS NOT NULL WHEN 'session_secret' THEN resource_session_secret IS NOT NULL WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL WHEN 'workspace' THEN resource_workspace IS NOT NULL WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL END AND ((resource_keychain_id IS NOT NULL) + (resource_keychain_version IS NOT NULL) + (resource_agent_provider IS NOT NULL) + (resource_agent_model IS NOT NULL) + (resource_group IS NOT NULL) + (resource_group_member_group IS NOT NULL) + (resource_group_member_principal IS NOT NULL) + (resource_identity IS NOT NULL) + (resource_principal IS NOT NULL) + (resource_project IS NOT NULL) + (resource_project_file IS NOT NULL) + (resource_project_grant IS NOT NULL) + (resource_project_note IS NOT NULL) + (resource_project_task IS NOT NULL) + (resource_project_secret IS NOT NULL) + (resource_project_record_schema IS NOT NULL) + (resource_project_record_attribute IS NOT NULL) + (resource_project_record IS NOT NULL) + (resource_session IS NOT NULL) + (resource_session_event IS NOT NULL) + (resource_session_file IS NOT NULL) + (resource_session_grant IS NOT NULL) + (resource_session_note IS NOT NULL) + (resource_session_task IS NOT NULL) + (resource_session_secret IS NOT NULL) + (resource_storage_provider IS NOT NULL) + (resource_system_grant IS NOT NULL) + (resource_workspace IS NOT NULL) + (resource_workspace_agent_workspace IS NOT NULL) + (resource_workspace_agent_id IS NOT NULL) + (resource_workspace_grant IS NOT NULL) + (resource_workspace_storage_provider_workspace IS NOT NULL) + (resource_workspace_storage_provider_provider IS NOT NULL)) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END)
			) STRICT;
			INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at)
			SELECT id, event, resource_kind, resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at FROM gatehouse_activity_events_migration_42;
			DROP TABLE gatehouse_activity_events_migration_42;
			CREATE TABLE gatehouse_activity_event_topics (activity TEXT NOT NULL, topic TEXT NOT NULL CHECK (length(trim(topic)) > 0), PRIMARY KEY (activity, topic), FOREIGN KEY (activity) REFERENCES gatehouse_activity_events (id)) STRICT;
			INSERT INTO gatehouse_activity_event_topics SELECT activity, topic FROM gatehouse_activity_event_topics_migration_42;
			DROP TABLE gatehouse_activity_event_topics_migration_42;
			CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor ON gatehouse_activity_event_topics (topic, activity);
		`),
	}
}
