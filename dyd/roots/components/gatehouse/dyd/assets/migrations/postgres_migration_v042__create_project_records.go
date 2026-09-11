package migrations

func postgresMigrationV042CreateProjectRecords() VersionedMigration {
	return VersionedMigration{
		Index:       42,
		Description: "create_project_records",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_project_record_schemas (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^prs_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				name TEXT NOT NULL CHECK (name ~ '^[a-z][a-z0-9_]*$'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (octet_length(label) <= 256),
				description TEXT NOT NULL CHECK (octet_length(description) <= 4096),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, project, id),
				UNIQUE (id),
				UNIQUE (workspace, project, name),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			);
			CREATE INDEX gatehouse_project_record_schemas_by_project_created
			ON gatehouse_project_record_schemas (workspace, project, created_at DESC, id DESC);

			CREATE TABLE gatehouse_project_record_attributes (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^pra_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				name TEXT NOT NULL CHECK (name ~ '^[a-z][a-z0-9_]*$'),
				label TEXT NOT NULL CHECK (length(trim(label)) > 0) CHECK (octet_length(label) <= 256),
				description TEXT NOT NULL CHECK (octet_length(description) <= 4096),
				type TEXT NOT NULL CHECK (type IN ('text', 'number', 'boolean', 'datetime', 'record')),
				target_schema TEXT,
				cardinality TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
				uniqueness TEXT NOT NULL CHECK (uniqueness IN ('none', 'record', 'global')),
				display TEXT NOT NULL CHECK (display IN ('none', 'primary', 'secondary')),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, project, schema, id),
				UNIQUE (id),
				UNIQUE (workspace, project, schema, name),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, project, target_schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((type = 'record' AND target_schema IS NOT NULL) OR (type <> 'record' AND target_schema IS NULL)),
				CHECK (NOT (cardinality = 'one' AND uniqueness = 'record')),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			);
			CREATE INDEX gatehouse_project_record_attributes_by_schema_name
			ON gatehouse_project_record_attributes (workspace, project, schema, name);

			CREATE TABLE gatehouse_project_records (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				schema TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^prr_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, project, schema, id),
				UNIQUE (id),
				FOREIGN KEY (workspace, project, schema) REFERENCES gatehouse_project_record_schemas (workspace, project, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			);
			CREATE INDEX gatehouse_project_records_by_schema_created
			ON gatehouse_project_records (workspace, project, schema, created_at DESC, id DESC);

			CREATE TABLE gatehouse_project_record_values (
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				schema TEXT NOT NULL,
				record TEXT NOT NULL,
				id TEXT NOT NULL CHECK (id ~ '^prv_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				attribute TEXT NOT NULL,
				value_type TEXT NOT NULL CHECK (value_type IN ('text', 'number', 'boolean', 'datetime', 'record')),
				value_text TEXT,
				value_number DOUBLE PRECISION,
				value_boolean BOOLEAN,
				value_datetime TIMESTAMPTZ,
				value_reference TEXT,
				value_key TEXT NOT NULL,
				attribute_cardinality TEXT NOT NULL CHECK (attribute_cardinality IN ('one', 'many')),
				attribute_uniqueness TEXT NOT NULL CHECK (attribute_uniqueness IN ('none', 'record', 'global')),
				sensitive BOOLEAN NOT NULL,
				author_principal TEXT REFERENCES gatehouse_principals (id),
				author_agent TEXT,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (workspace, project, schema, record, id),
				UNIQUE (id),
				FOREIGN KEY (workspace, project, schema, record) REFERENCES gatehouse_project_records (workspace, project, schema, id) ON DELETE CASCADE,
				FOREIGN KEY (workspace, project, schema, attribute) REFERENCES gatehouse_project_record_attributes (workspace, project, schema, id),
				FOREIGN KEY (workspace, author_agent) REFERENCES gatehouse_workspace_agents (workspace_id, id),
				CHECK ((value_type = 'text' AND value_text IS NOT NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference IS NULL) OR (value_type = 'number' AND value_text IS NULL AND value_number IS NOT NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference IS NULL) OR (value_type = 'boolean' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NOT NULL AND value_datetime IS NULL AND value_reference IS NULL) OR (value_type = 'datetime' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NOT NULL AND value_reference IS NULL) OR (value_type = 'record' AND value_text IS NULL AND value_number IS NULL AND value_boolean IS NULL AND value_datetime IS NULL AND value_reference IS NOT NULL)),
				CHECK ((author_principal IS NOT NULL AND author_agent IS NULL) OR (author_principal IS NULL AND author_agent IS NOT NULL))
			);
			CREATE UNIQUE INDEX gatehouse_project_record_values_one
			ON gatehouse_project_record_values (workspace, project, schema, record, attribute)
			WHERE attribute_cardinality = 'one';
			CREATE UNIQUE INDEX gatehouse_project_record_values_record_unique
			ON gatehouse_project_record_values (workspace, project, schema, record, attribute, value_key)
			WHERE attribute_uniqueness = 'record';
			CREATE UNIQUE INDEX gatehouse_project_record_values_global_unique
			ON gatehouse_project_record_values (workspace, project, schema, attribute, value_key)
			WHERE attribute_uniqueness = 'global';
			CREATE INDEX gatehouse_project_record_values_by_record_created
			ON gatehouse_project_record_values (workspace, project, schema, record, created_at DESC, id DESC);

			CREATE FUNCTION gatehouse_project_record_value_validate() RETURNS TRIGGER AS $$
			DECLARE attribute_row RECORD;
			BEGIN
				SELECT type, target_schema, cardinality, uniqueness INTO attribute_row
				FROM gatehouse_project_record_attributes
				WHERE workspace = NEW.workspace AND project = NEW.project AND schema = NEW.schema AND id = NEW.attribute;
				IF NOT FOUND THEN RAISE EXCEPTION 'project record attribute is unavailable'; END IF;
				IF NEW.value_type <> attribute_row.type THEN RAISE EXCEPTION 'project record value type does not match attribute'; END IF;
				IF NEW.attribute_cardinality <> attribute_row.cardinality OR NEW.attribute_uniqueness <> attribute_row.uniqueness THEN RAISE EXCEPTION 'project record value constraints do not match attribute'; END IF;
				IF NEW.sensitive AND attribute_row.uniqueness = 'global' THEN RAISE EXCEPTION 'sensitive values cannot be globally unique'; END IF;
				IF NEW.value_type = 'record' AND NOT EXISTS (SELECT 1 FROM gatehouse_project_records WHERE workspace = NEW.workspace AND project = NEW.project AND schema = attribute_row.target_schema AND id = NEW.value_reference) THEN RAISE EXCEPTION 'project record reference target does not match attribute schema'; END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;
			CREATE TRIGGER gatehouse_project_record_values_validate
			BEFORE INSERT OR UPDATE ON gatehouse_project_record_values
			FOR EACH ROW EXECUTE FUNCTION gatehouse_project_record_value_validate();

			CREATE FUNCTION gatehouse_project_record_attribute_validate_change() RETURNS TRIGGER AS $$
			BEGIN
				IF (NEW.type, NEW.target_schema, NEW.cardinality, NEW.uniqueness) IS DISTINCT FROM (OLD.type, OLD.target_schema, OLD.cardinality, OLD.uniqueness)
					AND EXISTS (SELECT 1 FROM gatehouse_project_record_values WHERE workspace = OLD.workspace AND project = OLD.project AND schema = OLD.schema AND attribute = OLD.id) THEN
					RAISE EXCEPTION 'project record attribute structure cannot change while values exist';
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;
			CREATE TRIGGER gatehouse_project_record_attributes_validate_change
			BEFORE UPDATE ON gatehouse_project_record_attributes
			FOR EACH ROW EXECUTE FUNCTION gatehouse_project_record_attribute_validate_change();

			ALTER TABLE gatehouse_agent_tasks__session_event_reply
				ADD COLUMN request_principal TEXT REFERENCES gatehouse_principals (id),
				ADD COLUMN request_project TEXT,
				ADD COLUMN request_project_actions TEXT,
				ADD FOREIGN KEY (workspace, request_project) REFERENCES gatehouse_projects (workspace, id);

			ALTER TABLE gatehouse_activity_events
				ADD COLUMN resource_project_record_schema TEXT,
				ADD COLUMN resource_project_record_attribute TEXT,
				ADD COLUMN resource_project_record TEXT,
				DROP CONSTRAINT gatehouse_activity_events_resource_kind_check,
				DROP CONSTRAINT gatehouse_activity_events_check,
				ADD CONSTRAINT gatehouse_activity_events_resource_kind_check CHECK (resource_kind IN ('keychain', 'agent_provider', 'agent_model', 'group', 'group_member', 'identity', 'principal', 'project', 'project_file', 'project_grant', 'project_note', 'project_task', 'project_secret', 'project_record_schema', 'project_record_attribute', 'project_record', 'session', 'session_event', 'session_file', 'session_grant', 'session_note', 'session_task', 'session_secret', 'storage_provider', 'system_grant', 'workspace', 'workspace_agent', 'workspace_grant', 'workspace_storage_provider')),
				ADD CONSTRAINT gatehouse_activity_events_check CHECK (
					CASE resource_kind
						WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL
						WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL
						WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_id IS NOT NULL
						WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL
						WHEN 'project_record_schema' THEN resource_project_record_schema IS NOT NULL
						WHEN 'project_record_attribute' THEN resource_project_record_attribute IS NOT NULL
						WHEN 'project_record' THEN resource_project_record IS NOT NULL
						WHEN 'project_task' THEN resource_project_task IS NOT NULL
						WHEN 'session_task' THEN resource_session_task IS NOT NULL
						WHEN 'system_grant' THEN resource_system_grant IS NOT NULL
						WHEN 'agent_provider' THEN resource_agent_provider IS NOT NULL WHEN 'agent_model' THEN resource_agent_model IS NOT NULL WHEN 'group' THEN resource_group IS NOT NULL WHEN 'identity' THEN resource_identity IS NOT NULL WHEN 'principal' THEN resource_principal IS NOT NULL WHEN 'project' THEN resource_project IS NOT NULL WHEN 'project_file' THEN resource_project_file IS NOT NULL WHEN 'project_grant' THEN resource_project_grant IS NOT NULL WHEN 'project_note' THEN resource_project_note IS NOT NULL WHEN 'project_secret' THEN resource_project_secret IS NOT NULL WHEN 'session' THEN resource_session IS NOT NULL WHEN 'session_event' THEN resource_session_event IS NOT NULL WHEN 'session_file' THEN resource_session_file IS NOT NULL WHEN 'session_grant' THEN resource_session_grant IS NOT NULL WHEN 'session_note' THEN resource_session_note IS NOT NULL WHEN 'session_secret' THEN resource_session_secret IS NOT NULL WHEN 'storage_provider' THEN resource_storage_provider IS NOT NULL WHEN 'workspace' THEN resource_workspace IS NOT NULL WHEN 'workspace_grant' THEN resource_workspace_grant IS NOT NULL
					END
					AND num_nonnulls(resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_project_record_schema, resource_project_record_attribute, resource_project_record, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END
				);
		`),
	}
}
