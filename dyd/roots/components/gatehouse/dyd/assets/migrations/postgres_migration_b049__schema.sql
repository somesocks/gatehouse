--
-- PostgreSQL database dump
--


-- Dumped from database version 16.15 (Debian 16.15-1.pgdg13+2)
-- Dumped by pg_dump version 16.15 (Debian 16.15-1.pgdg13+2)

--
-- Name: gatehouse_note_revision_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.gatehouse_note_revision_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
				BEGIN
					RAISE EXCEPTION 'note revisions are immutable';
				END;
				$$;


--
-- Name: gatehouse_project_file_remove_referenced_validate(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.gatehouse_project_file_remove_referenced_validate() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
			BEGIN
				IF OLD.enabled AND NOT NEW.enabled AND EXISTS (SELECT 1 FROM gatehouse_project_record_values WHERE workspace = OLD.workspace AND project = OLD.project AND value_type = 'file' AND value_reference_file = OLD.id) THEN
					RAISE EXCEPTION 'project file is referenced by a record value';
				END IF;
				RETURN NEW;
			END;
			$$;


--
-- Name: gatehouse_project_record_file_reference_validate(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.gatehouse_project_record_file_reference_validate() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
			BEGIN
				IF NEW.value_type = 'file' AND NOT EXISTS (SELECT 1 FROM gatehouse_project_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = NEW.workspace AND files.project = NEW.project AND files.id = NEW.value_reference_file AND files.enabled AND objects.state = 'success') THEN
					RAISE EXCEPTION 'project file reference is unavailable';
				END IF;
				RETURN NEW;
			END;
			$$;


--
-- Name: gatehouse_activity_event_topics; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_activity_event_topics (
    activity text NOT NULL,
    topic text NOT NULL,
    CONSTRAINT gatehouse_activity_event_topics_topic_check CHECK ((length(TRIM(BOTH FROM topic)) > 0))
);


--
-- Name: gatehouse_activity_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_activity_events (
    id text NOT NULL,
    event text NOT NULL,
    resource_kind text NOT NULL,
    resource_keychain_id text,
    resource_keychain_version integer,
    resource_agent_provider text,
    resource_agent_model text,
    resource_group text,
    resource_group_member_group text,
    resource_group_member_principal text,
    resource_identity text,
    resource_principal text,
    resource_project text,
    resource_project_file text,
    resource_project_grant text,
    resource_project_note text,
    resource_project_secret text,
    resource_session text,
    resource_session_event text,
    resource_session_file text,
    resource_session_grant text,
    resource_session_note text,
    resource_session_secret text,
    resource_storage_provider text,
    resource_workspace text,
    resource_workspace_agent_workspace text,
    resource_workspace_agent_id text,
    resource_workspace_grant text,
    resource_workspace_storage_provider_workspace text,
    resource_workspace_storage_provider_provider text,
    created_at timestamp with time zone NOT NULL,
    resource_system_grant text,
    resource_project_task text,
    resource_session_task text,
    resource_project_record_schema text,
    resource_project_record_attribute text,
    resource_project_record text,
    CONSTRAINT gatehouse_activity_events_check CHECK ((
CASE resource_kind
    WHEN 'keychain'::text THEN ((resource_keychain_id IS NOT NULL) AND (resource_keychain_version IS NOT NULL))
    WHEN 'group_member'::text THEN ((resource_group_member_group IS NOT NULL) AND (resource_group_member_principal IS NOT NULL))
    WHEN 'workspace_agent'::text THEN ((resource_workspace_agent_workspace IS NOT NULL) AND (resource_workspace_agent_id IS NOT NULL))
    WHEN 'workspace_storage_provider'::text THEN ((resource_workspace_storage_provider_workspace IS NOT NULL) AND (resource_workspace_storage_provider_provider IS NOT NULL))
    WHEN 'project_record_schema'::text THEN (resource_project_record_schema IS NOT NULL)
    WHEN 'project_record_attribute'::text THEN (resource_project_record_attribute IS NOT NULL)
    WHEN 'project_record'::text THEN (resource_project_record IS NOT NULL)
    WHEN 'project_task'::text THEN (resource_project_task IS NOT NULL)
    WHEN 'session_task'::text THEN (resource_session_task IS NOT NULL)
    WHEN 'system_grant'::text THEN (resource_system_grant IS NOT NULL)
    WHEN 'agent_provider'::text THEN (resource_agent_provider IS NOT NULL)
    WHEN 'agent_model'::text THEN (resource_agent_model IS NOT NULL)
    WHEN 'group'::text THEN (resource_group IS NOT NULL)
    WHEN 'identity'::text THEN (resource_identity IS NOT NULL)
    WHEN 'principal'::text THEN (resource_principal IS NOT NULL)
    WHEN 'project'::text THEN (resource_project IS NOT NULL)
    WHEN 'project_file'::text THEN (resource_project_file IS NOT NULL)
    WHEN 'project_grant'::text THEN (resource_project_grant IS NOT NULL)
    WHEN 'project_note'::text THEN (resource_project_note IS NOT NULL)
    WHEN 'project_secret'::text THEN (resource_project_secret IS NOT NULL)
    WHEN 'session'::text THEN (resource_session IS NOT NULL)
    WHEN 'session_event'::text THEN (resource_session_event IS NOT NULL)
    WHEN 'session_file'::text THEN (resource_session_file IS NOT NULL)
    WHEN 'session_grant'::text THEN (resource_session_grant IS NOT NULL)
    WHEN 'session_note'::text THEN (resource_session_note IS NOT NULL)
    WHEN 'session_secret'::text THEN (resource_session_secret IS NOT NULL)
    WHEN 'storage_provider'::text THEN (resource_storage_provider IS NOT NULL)
    WHEN 'workspace'::text THEN (resource_workspace IS NOT NULL)
    WHEN 'workspace_grant'::text THEN (resource_workspace_grant IS NOT NULL)
    ELSE NULL::boolean
END AND (num_nonnulls(resource_keychain_id, resource_keychain_version, resource_agent_provider, resource_agent_model, resource_group, resource_group_member_group, resource_group_member_principal, resource_identity, resource_principal, resource_project, resource_project_file, resource_project_grant, resource_project_note, resource_project_task, resource_project_secret, resource_project_record_schema, resource_project_record_attribute, resource_project_record, resource_session, resource_session_event, resource_session_file, resource_session_grant, resource_session_note, resource_session_task, resource_session_secret, resource_storage_provider, resource_system_grant, resource_workspace, resource_workspace_agent_workspace, resource_workspace_agent_id, resource_workspace_grant, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) =
CASE resource_kind
    WHEN 'keychain'::text THEN 2
    WHEN 'group_member'::text THEN 2
    WHEN 'workspace_agent'::text THEN 2
    WHEN 'workspace_storage_provider'::text THEN 2
    ELSE 1
END))),
    CONSTRAINT gatehouse_activity_events_event_check CHECK ((length(TRIM(BOTH FROM event)) > 0)),
    CONSTRAINT gatehouse_activity_events_id_check CHECK ((id ~ '^act_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_activity_events_resource_kind_check CHECK ((resource_kind = ANY (ARRAY['keychain'::text, 'agent_provider'::text, 'agent_model'::text, 'group'::text, 'group_member'::text, 'identity'::text, 'principal'::text, 'project'::text, 'project_file'::text, 'project_grant'::text, 'project_note'::text, 'project_task'::text, 'project_secret'::text, 'project_record_schema'::text, 'project_record_attribute'::text, 'project_record'::text, 'session'::text, 'session_event'::text, 'session_file'::text, 'session_grant'::text, 'session_note'::text, 'session_task'::text, 'session_secret'::text, 'storage_provider'::text, 'system_grant'::text, 'workspace'::text, 'workspace_agent'::text, 'workspace_grant'::text, 'workspace_storage_provider'::text])))
);


--
-- Name: gatehouse_agent_contexts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_agent_contexts (
    workspace text NOT NULL,
    session text NOT NULL,
    root text NOT NULL,
    model text NOT NULL,
    profile text NOT NULL,
    state jsonb NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_agent_contexts_profile_check CHECK ((length(TRIM(BOTH FROM profile)) > 0)),
    CONSTRAINT gatehouse_agent_contexts_state_check CHECK ((jsonb_typeof(state) = 'object'::text))
);


--
-- Name: gatehouse_agent_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_agent_models (
    id text NOT NULL,
    alias text,
    revision bigint NOT NULL,
    provider_id text NOT NULL,
    model text NOT NULL,
    parameters jsonb NOT NULL,
    enabled boolean NOT NULL,
    compaction jsonb DEFAULT '{"algorithm": "mcmtr", "buffer_bytes": 16384, "history_bytes": 98304}'::jsonb NOT NULL,
    max_turns bigint DEFAULT 127 NOT NULL,
    max_output_tokens bigint DEFAULT 16000 NOT NULL,
    CONSTRAINT gatehouse_agent_models_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_agent_models_compaction_check CHECK ((jsonb_typeof(compaction) = 'object'::text)),
    CONSTRAINT gatehouse_agent_models_id_check CHECK ((id ~ '^amd_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_agent_models_max_output_tokens_check CHECK ((max_output_tokens > 0)),
    CONSTRAINT gatehouse_agent_models_max_turns_check CHECK ((max_turns > 0)),
    CONSTRAINT gatehouse_agent_models_model_check CHECK ((length(TRIM(BOTH FROM model)) > 0)),
    CONSTRAINT gatehouse_agent_models_parameters_check CHECK ((jsonb_typeof(parameters) = 'object'::text)),
    CONSTRAINT gatehouse_agent_models_revision_check CHECK ((revision > 0))
);


--
-- Name: gatehouse_agent_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_agent_providers (
    id text NOT NULL,
    alias text,
    revision bigint NOT NULL,
    protocol text NOT NULL,
    base_url text,
    keychain_id text,
    keychain_version bigint,
    api_key text,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_agent_providers_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_agent_providers_api_key_check CHECK (((api_key IS NULL) OR (length(TRIM(BOTH FROM api_key)) > 0))),
    CONSTRAINT gatehouse_agent_providers_base_url_check CHECK (((base_url IS NULL) OR (length(TRIM(BOTH FROM base_url)) > 0))),
    CONSTRAINT gatehouse_agent_providers_check CHECK ((((protocol = 'builtin'::text) AND (base_url IS NULL) AND (keychain_id IS NULL) AND (keychain_version IS NULL) AND (api_key IS NULL)) OR ((protocol = ANY (ARRAY['openai-chat-completions'::text, 'openai-responses'::text])) AND (base_url IS NOT NULL) AND (keychain_id IS NOT NULL) AND (keychain_version IS NOT NULL) AND (api_key IS NOT NULL)))),
    CONSTRAINT gatehouse_agent_providers_id_check CHECK ((id ~ '^apr_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_agent_providers_keychain_version_check CHECK (((keychain_version IS NULL) OR (keychain_version > 0))),
    CONSTRAINT gatehouse_agent_providers_protocol_check CHECK ((protocol = ANY (ARRAY['builtin'::text, 'openai-chat-completions'::text, 'openai-responses'::text]))),
    CONSTRAINT gatehouse_agent_providers_revision_check CHECK ((revision > 0))
);


--
-- Name: gatehouse_agent_tasks__session_event_reply; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_agent_tasks__session_event_reply (
    workspace text NOT NULL,
    session text NOT NULL,
    event text NOT NULL,
    created_at timestamp with time zone NOT NULL
);


--
-- Name: gatehouse_agent_tasks__session_name; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_agent_tasks__session_name (
    workspace text NOT NULL,
    session text NOT NULL,
    created_at timestamp with time zone NOT NULL
);


--
-- Name: gatehouse_embedded_storage_object_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_embedded_storage_object_chunks (
    embedded_storage_object text NOT NULL,
    ordinal bigint NOT NULL,
    sha256 bytea NOT NULL,
    size bigint NOT NULL,
    bytes bytea NOT NULL,
    CONSTRAINT gatehouse_embedded_storage_object_chunks_check CHECK ((octet_length(bytes) = size)),
    CONSTRAINT gatehouse_embedded_storage_object_chunks_ordinal_check CHECK ((ordinal >= 0)),
    CONSTRAINT gatehouse_embedded_storage_object_chunks_sha256_check CHECK ((octet_length(sha256) = 32)),
    CONSTRAINT gatehouse_embedded_storage_object_chunks_size_check CHECK (((size > 0) AND (size <= 262144)))
);


--
-- Name: gatehouse_embedded_storage_objects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_embedded_storage_objects (
    id text NOT NULL
);


--
-- Name: gatehouse_group_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_group_members (
    workspace_id text NOT NULL,
    group_id text NOT NULL,
    principal_id text NOT NULL,
    enabled boolean NOT NULL
);


--
-- Name: gatehouse_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_groups (
    workspace_id text NOT NULL,
    id text NOT NULL,
    alias text,
    name text,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_groups_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_groups_id_check CHECK ((id ~ '^grp_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_groups_name_check CHECK (((name IS NULL) OR (length(TRIM(BOTH FROM name)) > 0)))
);


--
-- Name: gatehouse_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_identities (
    id text NOT NULL,
    alias text,
    key text NOT NULL,
    principal_id text NOT NULL,
    verifiers jsonb NOT NULL,
    enabled boolean NOT NULL,
    revision bigint DEFAULT 0 NOT NULL,
    CONSTRAINT gatehouse_identities_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_identities_id_check CHECK ((id ~ '^idt_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_identities_key_check CHECK ((POSITION((':'::text) IN (key)) > 1)),
    CONSTRAINT gatehouse_identities_revision_check CHECK ((revision >= 0)),
    CONSTRAINT gatehouse_identities_verifiers_check CHECK ((jsonb_typeof(verifiers) = 'array'::text)),
    CONSTRAINT gatehouse_identities_verifiers_check1 CHECK ((jsonb_array_length(verifiers) > 0))
);


--
-- Name: gatehouse_keychains; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_keychains (
    id text NOT NULL,
    version bigint NOT NULL,
    kek_kdf text NOT NULL,
    key text NOT NULL,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_keychains_id_check CHECK ((id ~ '^[a-z][a-z0-9_-]*$'::text)),
    CONSTRAINT gatehouse_keychains_kek_kdf_check CHECK ((length(TRIM(BOTH FROM kek_kdf)) > 0)),
    CONSTRAINT gatehouse_keychains_key_check CHECK ((length(TRIM(BOTH FROM key)) > 0)),
    CONSTRAINT gatehouse_keychains_version_check CHECK ((version > 0))
);


--
-- Name: gatehouse_principals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_principals (
    id text NOT NULL,
    alias text,
    name text,
    revision integer NOT NULL,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_principals_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_principals_id_check CHECK ((id ~ '^prn_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_principals_name_check CHECK (((name IS NULL) OR (length(TRIM(BOTH FROM name)) > 0))),
    CONSTRAINT gatehouse_principals_revision_check CHECK ((revision > 0))
);


--
-- Name: gatehouse_project_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_files (
    workspace text NOT NULL,
    project text NOT NULL,
    id text NOT NULL,
    storage_object text NOT NULL,
    name text NOT NULL,
    media_type text,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_project_files_id_check CHECK ((id ~ '^pfi_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_files_media_type_check CHECK (((media_type IS NULL) OR (length(TRIM(BOTH FROM media_type)) > 0))),
    CONSTRAINT gatehouse_project_files_name_check CHECK ((length(TRIM(BOTH FROM name)) > 0))
);


--
-- Name: gatehouse_project_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_grants (
    workspace text NOT NULL,
    project text NOT NULL,
    role text NOT NULL,
    principal text,
    "group" text,
    enabled boolean NOT NULL,
    id text NOT NULL,
    CONSTRAINT gatehouse_project_grants_check CHECK ((((principal IS NOT NULL) AND ("group" IS NULL)) OR ((principal IS NULL) AND ("group" IS NOT NULL)))),
    CONSTRAINT gatehouse_project_grants_id_check CHECK ((id ~ '^pgr_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_grants_role_check CHECK ((role = ANY (ARRAY['member'::text, 'contributor'::text, 'manager'::text])))
);


--
-- Name: gatehouse_project_note_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_note_revisions (
    workspace text NOT NULL,
    project text NOT NULL,
    note text NOT NULL,
    revision bigint NOT NULL,
    author_principal text,
    author_agent text,
    author_gateway text,
    title text NOT NULL,
    description text NOT NULL,
    body text NOT NULL,
    sensitive boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_project_note_revisions_author_gateway_check CHECK ((author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_note_revisions_body_check CHECK ((octet_length(body) <= 1048576)),
    CONSTRAINT gatehouse_project_note_revisions_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NULL) AND (author_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_project_note_revisions_description_check CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_project_note_revisions_revision_check CHECK ((revision > 0)),
    CONSTRAINT gatehouse_project_note_revisions_title_check CHECK ((length(TRIM(BOTH FROM title)) > 0)),
    CONSTRAINT gatehouse_project_note_revisions_title_check1 CHECK ((octet_length(title) <= 256))
);


--
-- Name: gatehouse_project_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_notes (
    workspace text NOT NULL,
    project text NOT NULL,
    id text NOT NULL,
    author_principal text,
    title text NOT NULL,
    description text NOT NULL,
    body text NOT NULL,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    author_agent text,
    author_gateway text,
    revision bigint DEFAULT 1 NOT NULL,
    CONSTRAINT gatehouse_project_notes_author_gateway_check CHECK ((author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_notes_body_check CHECK ((octet_length(body) <= 1048576)),
    CONSTRAINT gatehouse_project_notes_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NULL) AND (author_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_project_notes_description_check CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_project_notes_id_check CHECK ((id ~ '^pnt_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_notes_revision_check CHECK ((revision > 0)),
    CONSTRAINT gatehouse_project_notes_title_check CHECK ((length(TRIM(BOTH FROM title)) > 0)),
    CONSTRAINT gatehouse_project_notes_title_check1 CHECK ((octet_length(title) <= 256))
);


--
-- Name: gatehouse_project_record_attributes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_record_attributes (
    workspace text NOT NULL,
    project text NOT NULL,
    schema text NOT NULL,
    id text NOT NULL,
    name text NOT NULL,
    label text NOT NULL,
    description text NOT NULL,
    type text NOT NULL,
    target_schema text,
    cardinality text NOT NULL,
    uniqueness text NOT NULL,
    display text NOT NULL,
    author_principal text,
    author_agent text,
    created_at timestamp with time zone NOT NULL,
    display_order integer DEFAULT 0 NOT NULL,
    CONSTRAINT gatehouse_project_record_attributes_cardinality_check CHECK ((cardinality = ANY (ARRAY['one'::text, 'many'::text]))),
    CONSTRAINT gatehouse_project_record_attributes_check CHECK ((((type = 'record'::text) AND (target_schema IS NOT NULL)) OR ((type <> 'record'::text) AND (target_schema IS NULL)))),
    CONSTRAINT gatehouse_project_record_attributes_check1 CHECK ((NOT ((cardinality = 'one'::text) AND (uniqueness = 'record'::text)))),
    CONSTRAINT gatehouse_project_record_attributes_check2 CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL)))),
    CONSTRAINT gatehouse_project_record_attributes_description_check CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_project_record_attributes_display_check CHECK ((display = ANY (ARRAY['none'::text, 'primary'::text, 'secondary'::text]))),
    CONSTRAINT gatehouse_project_record_attributes_display_order_check CHECK ((display_order >= 0)),
    CONSTRAINT gatehouse_project_record_attributes_id_check CHECK ((id ~ '^pra_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_record_attributes_label_check CHECK ((length(TRIM(BOTH FROM label)) > 0)),
    CONSTRAINT gatehouse_project_record_attributes_label_check1 CHECK ((octet_length(label) <= 256)),
    CONSTRAINT gatehouse_project_record_attributes_name_check CHECK ((name ~ '^[a-z][a-z0-9_]*$'::text)),
    CONSTRAINT gatehouse_project_record_attributes_type_check CHECK ((type = ANY (ARRAY['text'::text, 'number'::text, 'boolean'::text, 'datetime'::text, 'record'::text, 'file'::text]))),
    CONSTRAINT gatehouse_project_record_attributes_uniqueness_check CHECK ((uniqueness = ANY (ARRAY['none'::text, 'record'::text, 'global'::text])))
);


--
-- Name: gatehouse_project_record_schemas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_record_schemas (
    workspace text NOT NULL,
    project text NOT NULL,
    id text NOT NULL,
    name text NOT NULL,
    label text NOT NULL,
    description text NOT NULL,
    author_principal text,
    author_agent text,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_project_record_schemas_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL)))),
    CONSTRAINT gatehouse_project_record_schemas_description_check CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_project_record_schemas_id_check CHECK ((id ~ '^prs_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_record_schemas_label_check CHECK ((length(TRIM(BOTH FROM label)) > 0)),
    CONSTRAINT gatehouse_project_record_schemas_label_check1 CHECK ((octet_length(label) <= 256)),
    CONSTRAINT gatehouse_project_record_schemas_name_check CHECK ((name ~ '^[a-z][a-z0-9_]*$'::text))
);


--
-- Name: gatehouse_project_record_values; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_record_values (
    workspace text NOT NULL,
    project text NOT NULL,
    schema text NOT NULL,
    record text NOT NULL,
    id text NOT NULL,
    attribute text NOT NULL,
    value_type text NOT NULL,
    value_text text,
    value_number double precision,
    value_boolean boolean,
    value_datetime timestamp with time zone,
    value_reference text,
    value_key text NOT NULL,
    attribute_cardinality text NOT NULL,
    attribute_uniqueness text NOT NULL,
    sensitive boolean NOT NULL,
    author_principal text,
    author_agent text,
    created_at timestamp with time zone NOT NULL,
    value_reference_schema text,
    value_reference_file text,
    CONSTRAINT gatehouse_project_record_values_attribute_cardinality_check CHECK ((attribute_cardinality = ANY (ARRAY['one'::text, 'many'::text]))),
    CONSTRAINT gatehouse_project_record_values_attribute_uniqueness_check CHECK ((attribute_uniqueness = ANY (ARRAY['none'::text, 'record'::text, 'global'::text]))),
    CONSTRAINT gatehouse_project_record_values_check CHECK ((((value_type = 'text'::text) AND (value_text IS NOT NULL) AND (value_number IS NULL) AND (value_boolean IS NULL) AND (value_datetime IS NULL) AND (value_reference_schema IS NULL) AND (value_reference IS NULL) AND (value_reference_file IS NULL)) OR ((value_type = 'number'::text) AND (value_text IS NULL) AND (value_number IS NOT NULL) AND (value_boolean IS NULL) AND (value_datetime IS NULL) AND (value_reference_schema IS NULL) AND (value_reference IS NULL) AND (value_reference_file IS NULL)) OR ((value_type = 'boolean'::text) AND (value_text IS NULL) AND (value_number IS NULL) AND (value_boolean IS NOT NULL) AND (value_datetime IS NULL) AND (value_reference_schema IS NULL) AND (value_reference IS NULL) AND (value_reference_file IS NULL)) OR ((value_type = 'datetime'::text) AND (value_text IS NULL) AND (value_number IS NULL) AND (value_boolean IS NULL) AND (value_datetime IS NOT NULL) AND (value_reference_schema IS NULL) AND (value_reference IS NULL) AND (value_reference_file IS NULL)) OR ((value_type = 'record'::text) AND (value_text IS NULL) AND (value_number IS NULL) AND (value_boolean IS NULL) AND (value_datetime IS NULL) AND (value_reference_schema IS NOT NULL) AND (value_reference IS NOT NULL) AND (value_reference_file IS NULL)) OR ((value_type = 'file'::text) AND (value_text IS NULL) AND (value_number IS NULL) AND (value_boolean IS NULL) AND (value_datetime IS NULL) AND (value_reference_schema IS NULL) AND (value_reference IS NULL) AND (value_reference_file IS NOT NULL)))),
    CONSTRAINT gatehouse_project_record_values_check1 CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL)))),
    CONSTRAINT gatehouse_project_record_values_id_check CHECK ((id ~ '^prv_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_record_values_reference_schema_check CHECK ((((value_type = 'record'::text) AND (value_reference_schema IS NOT NULL)) OR ((value_type <> 'record'::text) AND (value_reference_schema IS NULL)))),
    CONSTRAINT gatehouse_project_record_values_sensitive_global_check CHECK ((NOT (sensitive AND (attribute_uniqueness = 'global'::text)))),
    CONSTRAINT gatehouse_project_record_values_value_type_check CHECK ((value_type = ANY (ARRAY['text'::text, 'number'::text, 'boolean'::text, 'datetime'::text, 'record'::text, 'file'::text])))
);


--
-- Name: gatehouse_project_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_records (
    workspace text NOT NULL,
    project text NOT NULL,
    schema text NOT NULL,
    id text NOT NULL,
    author_principal text,
    author_agent text,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_project_records_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL)))),
    CONSTRAINT gatehouse_project_records_id_check CHECK ((id ~ '^prr_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text))
);


--
-- Name: gatehouse_project_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_secrets (
    workspace text NOT NULL,
    project text NOT NULL,
    id text NOT NULL,
    author_principal text NOT NULL,
    description text NOT NULL,
    ciphertext text NOT NULL,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_project_secrets_description_check CHECK ((length(TRIM(BOTH FROM description)) > 0)),
    CONSTRAINT gatehouse_project_secrets_description_check1 CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_project_secrets_id_check CHECK ((id ~ '^psc_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text))
);


--
-- Name: gatehouse_project_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_project_tasks (
    workspace text NOT NULL,
    project text NOT NULL,
    id text NOT NULL,
    title text NOT NULL,
    description text,
    sensitive boolean NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    enabled boolean NOT NULL,
    creator_principal text,
    creator_agent text,
    creator_gateway text,
    created_at timestamp with time zone NOT NULL,
    updater_principal text,
    updater_agent text,
    updater_gateway text,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_project_tasks_check CHECK ((((creator_principal IS NOT NULL) AND (creator_agent IS NULL) AND (creator_gateway IS NULL)) OR ((creator_principal IS NULL) AND (creator_agent IS NOT NULL) AND (creator_gateway IS NULL)) OR ((creator_principal IS NULL) AND (creator_agent IS NULL) AND (creator_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_project_tasks_check1 CHECK ((((updater_principal IS NOT NULL) AND (updater_agent IS NULL) AND (updater_gateway IS NULL)) OR ((updater_principal IS NULL) AND (updater_agent IS NOT NULL) AND (updater_gateway IS NULL)) OR ((updater_principal IS NULL) AND (updater_agent IS NULL) AND (updater_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_project_tasks_creator_gateway_check CHECK ((creator_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_tasks_description_check CHECK (((description IS NULL) OR (octet_length(description) <= 4096))),
    CONSTRAINT gatehouse_project_tasks_id_check CHECK ((id ~ '^ptk_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_project_tasks_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'ready'::text, 'in_progress'::text, 'done'::text, 'cancelled'::text]))),
    CONSTRAINT gatehouse_project_tasks_title_check CHECK ((length(TRIM(BOTH FROM title)) > 0)),
    CONSTRAINT gatehouse_project_tasks_title_check1 CHECK ((octet_length(title) <= 256)),
    CONSTRAINT gatehouse_project_tasks_updater_gateway_check CHECK ((updater_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text))
);


--
-- Name: gatehouse_projects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_projects (
    workspace text NOT NULL,
    id text NOT NULL,
    name text,
    description text,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_projects_description_check CHECK (((description IS NULL) OR (length(TRIM(BOTH FROM description)) > 0))),
    CONSTRAINT gatehouse_projects_id_check CHECK ((id ~ '^prj_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_projects_name_check CHECK (((name IS NULL) OR (length(TRIM(BOTH FROM name)) > 0)))
);


--
-- Name: gatehouse_session_approval_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_approval_decisions (
    workspace text NOT NULL,
    session text NOT NULL,
    approval text NOT NULL,
    response text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    delivered boolean NOT NULL
);


--
-- Name: gatehouse_session_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_events (
    workspace text NOT NULL,
    session text NOT NULL,
    id text NOT NULL,
    parent text,
    kind text NOT NULL,
    author_principal text,
    author_agent text,
    author_gateway text,
    payload jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    metrics jsonb,
    CONSTRAINT gatehouse_session_events_author_gateway_check CHECK ((author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_events_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NULL) AND (author_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_session_events_check1 CHECK (((parent IS NULL) OR (parent <> id))),
    CONSTRAINT gatehouse_session_events_id_check CHECK ((id ~ '^sev_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_events_kind_check CHECK ((length(TRIM(BOTH FROM kind)) > 0)),
    CONSTRAINT gatehouse_session_events_metrics_check CHECK (((metrics IS NULL) OR (jsonb_typeof(metrics) = 'object'::text))),
    CONSTRAINT gatehouse_session_events_payload_check CHECK ((jsonb_typeof(payload) = 'object'::text))
);


--
-- Name: gatehouse_session_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_files (
    workspace text NOT NULL,
    session text NOT NULL,
    id text NOT NULL,
    storage_object text NOT NULL,
    name text NOT NULL,
    media_type text,
    created_at timestamp with time zone NOT NULL,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_session_files_id_check CHECK ((id ~ '^sfi_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_files_media_type_check CHECK (((media_type IS NULL) OR (length(TRIM(BOTH FROM media_type)) > 0))),
    CONSTRAINT gatehouse_session_files_name_check CHECK ((length(TRIM(BOTH FROM name)) > 0))
);


--
-- Name: gatehouse_session_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_grants (
    workspace text NOT NULL,
    session text NOT NULL,
    role text NOT NULL,
    principal text,
    "group" text,
    enabled boolean NOT NULL,
    id text NOT NULL,
    CONSTRAINT gatehouse_session_grants_check CHECK ((((principal IS NOT NULL) AND ("group" IS NULL)) OR ((principal IS NULL) AND ("group" IS NOT NULL)))),
    CONSTRAINT gatehouse_session_grants_id_check CHECK ((id ~ '^sgr_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_grants_role_check CHECK ((role = ANY (ARRAY['member'::text, 'contributor'::text, 'manager'::text])))
);


--
-- Name: gatehouse_session_note_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_note_revisions (
    workspace text NOT NULL,
    session text NOT NULL,
    note text NOT NULL,
    revision bigint NOT NULL,
    author_principal text,
    author_agent text,
    author_gateway text,
    title text NOT NULL,
    description text NOT NULL,
    body text NOT NULL,
    sensitive boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_session_note_revisions_author_gateway_check CHECK ((author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_note_revisions_body_check CHECK ((octet_length(body) <= 1048576)),
    CONSTRAINT gatehouse_session_note_revisions_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NULL) AND (author_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_session_note_revisions_description_check CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_session_note_revisions_revision_check CHECK ((revision > 0)),
    CONSTRAINT gatehouse_session_note_revisions_title_check CHECK ((length(TRIM(BOTH FROM title)) > 0)),
    CONSTRAINT gatehouse_session_note_revisions_title_check1 CHECK ((octet_length(title) <= 256))
);


--
-- Name: gatehouse_session_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_notes (
    workspace text NOT NULL,
    session text NOT NULL,
    id text NOT NULL,
    author_principal text,
    title text NOT NULL,
    description text NOT NULL,
    body text NOT NULL,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    author_agent text,
    author_gateway text,
    revision bigint DEFAULT 1 NOT NULL,
    CONSTRAINT gatehouse_session_notes_author_gateway_check CHECK ((author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_notes_body_check CHECK ((octet_length(body) <= 1048576)),
    CONSTRAINT gatehouse_session_notes_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NULL) AND (author_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_session_notes_description_check CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_session_notes_id_check CHECK ((id ~ '^snt_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_notes_revision_check CHECK ((revision > 0)),
    CONSTRAINT gatehouse_session_notes_title_check CHECK ((length(TRIM(BOTH FROM title)) > 0)),
    CONSTRAINT gatehouse_session_notes_title_check1 CHECK ((octet_length(title) <= 256))
);


--
-- Name: gatehouse_session_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_secrets (
    workspace text NOT NULL,
    session text NOT NULL,
    id text NOT NULL,
    author_principal text NOT NULL,
    description text NOT NULL,
    ciphertext text NOT NULL,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_session_secrets_description_check CHECK ((length(TRIM(BOTH FROM description)) > 0)),
    CONSTRAINT gatehouse_session_secrets_description_check1 CHECK ((octet_length(description) <= 4096)),
    CONSTRAINT gatehouse_session_secrets_id_check CHECK ((id ~ '^ssc_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text))
);


--
-- Name: gatehouse_session_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_session_tasks (
    workspace text NOT NULL,
    session text NOT NULL,
    id text NOT NULL,
    title text NOT NULL,
    description text,
    sensitive boolean NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    enabled boolean NOT NULL,
    creator_principal text,
    creator_agent text,
    creator_gateway text,
    created_at timestamp with time zone NOT NULL,
    updater_principal text,
    updater_agent text,
    updater_gateway text,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_session_tasks_check CHECK ((((creator_principal IS NOT NULL) AND (creator_agent IS NULL) AND (creator_gateway IS NULL)) OR ((creator_principal IS NULL) AND (creator_agent IS NOT NULL) AND (creator_gateway IS NULL)) OR ((creator_principal IS NULL) AND (creator_agent IS NULL) AND (creator_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_session_tasks_check1 CHECK ((((updater_principal IS NOT NULL) AND (updater_agent IS NULL) AND (updater_gateway IS NULL)) OR ((updater_principal IS NULL) AND (updater_agent IS NOT NULL) AND (updater_gateway IS NULL)) OR ((updater_principal IS NULL) AND (updater_agent IS NULL) AND (updater_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_session_tasks_creator_gateway_check CHECK ((creator_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_tasks_description_check CHECK (((description IS NULL) OR (octet_length(description) <= 4096))),
    CONSTRAINT gatehouse_session_tasks_id_check CHECK ((id ~ '^stk_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_session_tasks_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'ready'::text, 'in_progress'::text, 'done'::text, 'cancelled'::text]))),
    CONSTRAINT gatehouse_session_tasks_title_check CHECK ((length(TRIM(BOTH FROM title)) > 0)),
    CONSTRAINT gatehouse_session_tasks_title_check1 CHECK ((octet_length(title) <= 256)),
    CONSTRAINT gatehouse_session_tasks_updater_gateway_check CHECK ((updater_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text))
);


--
-- Name: gatehouse_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_sessions (
    workspace text NOT NULL,
    project text,
    id text NOT NULL,
    name text,
    author_principal text,
    author_agent text,
    author_gateway text,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_sessions_author_gateway_check CHECK ((author_gateway ~ '^gwy_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_sessions_check CHECK ((((author_principal IS NOT NULL) AND (author_agent IS NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NOT NULL) AND (author_gateway IS NULL)) OR ((author_principal IS NULL) AND (author_agent IS NULL) AND (author_gateway IS NOT NULL)))),
    CONSTRAINT gatehouse_sessions_id_check CHECK ((id ~ '^ses_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_sessions_name_check CHECK (((name IS NULL) OR (length(TRIM(BOTH FROM name)) > 0)))
);


--
-- Name: gatehouse_storage_objects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_storage_objects (
    id text NOT NULL,
    provider text NOT NULL,
    object text NOT NULL,
    state text NOT NULL,
    sha256 bytea,
    size bigint,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT gatehouse_storage_objects_check CHECK ((((sha256 IS NULL) AND (size IS NULL)) OR ((sha256 IS NOT NULL) AND (size IS NOT NULL)))),
    CONSTRAINT gatehouse_storage_objects_id_check CHECK ((id ~ '^obj_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_storage_objects_object_check CHECK ((length(TRIM(BOTH FROM object)) > 0)),
    CONSTRAINT gatehouse_storage_objects_sha256_check CHECK (((sha256 IS NULL) OR (octet_length(sha256) = 32))),
    CONSTRAINT gatehouse_storage_objects_size_check CHECK (((size IS NULL) OR (size >= 0))),
    CONSTRAINT gatehouse_storage_objects_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'success'::text, 'failure'::text])))
);


--
-- Name: gatehouse_storage_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_storage_providers (
    id text NOT NULL,
    alias text,
    revision bigint NOT NULL,
    protocol text NOT NULL,
    endpoint text,
    region text,
    bucket text,
    access_key_id text,
    keychain_id text,
    keychain_version bigint,
    secret_access_key text,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_storage_providers_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_storage_providers_check CHECK ((((protocol = 'embedded'::text) AND (endpoint IS NULL) AND (region IS NULL) AND (bucket IS NULL) AND (access_key_id IS NULL) AND (keychain_id IS NULL) AND (keychain_version IS NULL) AND (secret_access_key IS NULL)) OR ((protocol = 's3'::text) AND (endpoint IS NOT NULL) AND (length(TRIM(BOTH FROM endpoint)) > 0) AND (region IS NOT NULL) AND (length(TRIM(BOTH FROM region)) > 0) AND (bucket IS NOT NULL) AND (length(TRIM(BOTH FROM bucket)) > 0) AND (access_key_id IS NOT NULL) AND (length(TRIM(BOTH FROM access_key_id)) > 0) AND (keychain_id IS NOT NULL) AND (keychain_version IS NOT NULL) AND (keychain_version > 0) AND (secret_access_key IS NOT NULL) AND (length(TRIM(BOTH FROM secret_access_key)) > 0)))),
    CONSTRAINT gatehouse_storage_providers_id_check CHECK ((id ~ '^stp_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_storage_providers_protocol_check CHECK ((protocol = ANY (ARRAY['embedded'::text, 's3'::text]))),
    CONSTRAINT gatehouse_storage_providers_revision_check CHECK ((revision > 0))
);


--
-- Name: gatehouse_system_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_system_grants (
    id text NOT NULL,
    principal text NOT NULL,
    role text NOT NULL,
    enabled boolean NOT NULL,
    revision bigint NOT NULL,
    CONSTRAINT gatehouse_system_grants_id_check CHECK ((id ~ '^syg_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_system_grants_revision_check CHECK ((revision > 0)),
    CONSTRAINT gatehouse_system_grants_role_check CHECK ((role = 'manager'::text))
);


--
-- Name: gatehouse_workspace_agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_workspace_agents (
    workspace_id text NOT NULL,
    model_id text NOT NULL,
    enabled boolean NOT NULL,
    system_prompt text,
    label text,
    max_output_tokens bigint DEFAULT 16000 NOT NULL,
    revision bigint NOT NULL,
    id text NOT NULL,
    alias text NOT NULL,
    prelude text,
    "default" boolean DEFAULT false NOT NULL,
    CONSTRAINT gatehouse_workspace_agents_alias_check CHECK ((alias ~ '^[a-z0-9_-]+(/[a-z0-9_-]+)*$'::text)),
    CONSTRAINT gatehouse_workspace_agents_default_enabled_check CHECK (((NOT "default") OR enabled)),
    CONSTRAINT gatehouse_workspace_agents_id_check CHECK ((id ~ '^wag_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_workspace_agents_max_output_tokens_check CHECK ((max_output_tokens > 0)),
    CONSTRAINT gatehouse_workspace_agents_revision_check CHECK ((revision > 0))
);


--
-- Name: gatehouse_workspace_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_workspace_grants (
    workspace text NOT NULL,
    role text NOT NULL,
    principal text,
    "group" text,
    enabled boolean NOT NULL,
    revision bigint NOT NULL,
    id text NOT NULL,
    CONSTRAINT gatehouse_workspace_grants_check CHECK ((((principal IS NOT NULL) AND ("group" IS NULL)) OR ((principal IS NULL) AND ("group" IS NOT NULL)))),
    CONSTRAINT gatehouse_workspace_grants_id_check CHECK ((id ~ '^wgr_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_workspace_grants_revision_check CHECK ((revision > 0)),
    CONSTRAINT gatehouse_workspace_grants_role_check CHECK ((role = ANY (ARRAY['member'::text, 'contributor'::text, 'manager'::text])))
);


--
-- Name: gatehouse_workspace_storage_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_workspace_storage_providers (
    workspace text NOT NULL,
    provider text NOT NULL,
    priority bigint NOT NULL,
    enabled boolean NOT NULL,
    revision bigint NOT NULL,
    CONSTRAINT gatehouse_workspace_storage_providers_priority_check CHECK ((priority > 0)),
    CONSTRAINT gatehouse_workspace_storage_providers_revision_check CHECK ((revision > 0))
);


--
-- Name: gatehouse_workspaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gatehouse_workspaces (
    id text NOT NULL,
    alias text,
    name text,
    enabled boolean NOT NULL,
    CONSTRAINT gatehouse_workspaces_alias_check CHECK (((alias IS NULL) OR (alias ~ '^[a-z][a-z0-9_-]*$'::text))),
    CONSTRAINT gatehouse_workspaces_id_check CHECK ((id ~ '^wsp_[0-7][0-9a-hjkmnp-tv-z]{25}$'::text)),
    CONSTRAINT gatehouse_workspaces_name_check CHECK (((name IS NULL) OR (length(TRIM(BOTH FROM name)) > 0)))
);


--
-- Name: gatehouse_activity_event_topics gatehouse_activity_event_topics_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_event_topics
    ADD CONSTRAINT gatehouse_activity_event_topics_pkey PRIMARY KEY (activity, topic);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_agent_contexts gatehouse_agent_contexts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_contexts
    ADD CONSTRAINT gatehouse_agent_contexts_pkey PRIMARY KEY (workspace, session, root);


--
-- Name: gatehouse_agent_models gatehouse_agent_models_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_models
    ADD CONSTRAINT gatehouse_agent_models_alias_key UNIQUE (alias);


--
-- Name: gatehouse_agent_models gatehouse_agent_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_models
    ADD CONSTRAINT gatehouse_agent_models_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_agent_providers gatehouse_agent_providers_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_providers
    ADD CONSTRAINT gatehouse_agent_providers_alias_key UNIQUE (alias);


--
-- Name: gatehouse_agent_providers gatehouse_agent_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_providers
    ADD CONSTRAINT gatehouse_agent_providers_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_agent_tasks__session_event_reply gatehouse_agent_tasks__session_event_reply_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_tasks__session_event_reply
    ADD CONSTRAINT gatehouse_agent_tasks__session_event_reply_pkey PRIMARY KEY (workspace, session, event);


--
-- Name: gatehouse_agent_tasks__session_name gatehouse_agent_tasks__session_name_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_tasks__session_name
    ADD CONSTRAINT gatehouse_agent_tasks__session_name_pkey PRIMARY KEY (workspace, session);


--
-- Name: gatehouse_embedded_storage_object_chunks gatehouse_embedded_storage_object_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_embedded_storage_object_chunks
    ADD CONSTRAINT gatehouse_embedded_storage_object_chunks_pkey PRIMARY KEY (embedded_storage_object, ordinal);


--
-- Name: gatehouse_embedded_storage_objects gatehouse_embedded_storage_objects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_embedded_storage_objects
    ADD CONSTRAINT gatehouse_embedded_storage_objects_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_group_members gatehouse_group_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_group_members
    ADD CONSTRAINT gatehouse_group_members_pkey PRIMARY KEY (workspace_id, group_id, principal_id);


--
-- Name: gatehouse_groups gatehouse_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_groups
    ADD CONSTRAINT gatehouse_groups_pkey PRIMARY KEY (workspace_id, id);


--
-- Name: gatehouse_groups gatehouse_groups_workspace_id_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_groups
    ADD CONSTRAINT gatehouse_groups_workspace_id_alias_key UNIQUE (workspace_id, alias);


--
-- Name: gatehouse_identities gatehouse_identities_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_identities
    ADD CONSTRAINT gatehouse_identities_alias_key UNIQUE (alias);


--
-- Name: gatehouse_identities gatehouse_identities_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_identities
    ADD CONSTRAINT gatehouse_identities_key_key UNIQUE (key);


--
-- Name: gatehouse_identities gatehouse_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_identities
    ADD CONSTRAINT gatehouse_identities_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_keychains gatehouse_keychains_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_keychains
    ADD CONSTRAINT gatehouse_keychains_pkey PRIMARY KEY (id, version);


--
-- Name: gatehouse_principals gatehouse_principals_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_principals
    ADD CONSTRAINT gatehouse_principals_alias_key UNIQUE (alias);


--
-- Name: gatehouse_principals gatehouse_principals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_principals
    ADD CONSTRAINT gatehouse_principals_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_project_files gatehouse_project_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_files
    ADD CONSTRAINT gatehouse_project_files_pkey PRIMARY KEY (workspace, project, id);


--
-- Name: gatehouse_project_files gatehouse_project_files_storage_object_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_files
    ADD CONSTRAINT gatehouse_project_files_storage_object_key UNIQUE (storage_object);


--
-- Name: gatehouse_project_grants gatehouse_project_grants_id_once; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_grants
    ADD CONSTRAINT gatehouse_project_grants_id_once UNIQUE (id);


--
-- Name: gatehouse_project_note_revisions gatehouse_project_note_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_note_revisions
    ADD CONSTRAINT gatehouse_project_note_revisions_pkey PRIMARY KEY (workspace, project, note, revision);


--
-- Name: gatehouse_project_notes gatehouse_project_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_notes
    ADD CONSTRAINT gatehouse_project_notes_pkey PRIMARY KEY (workspace, project, id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attr_workspace_project_schema_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attr_workspace_project_schema_name_key UNIQUE (workspace, project, schema, name);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attributes_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attributes_id_key UNIQUE (id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attributes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attributes_pkey PRIMARY KEY (workspace, project, schema, id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attributes_reference_target_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attributes_reference_target_unique UNIQUE (workspace, project, schema, id, target_schema);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attributes_value_compatibility_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attributes_value_compatibility_unique UNIQUE (workspace, project, schema, id, type, cardinality, uniqueness);


--
-- Name: gatehouse_project_record_schemas gatehouse_project_record_schemas_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_schemas
    ADD CONSTRAINT gatehouse_project_record_schemas_id_key UNIQUE (id);


--
-- Name: gatehouse_project_record_schemas gatehouse_project_record_schemas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_schemas
    ADD CONSTRAINT gatehouse_project_record_schemas_pkey PRIMARY KEY (workspace, project, id);


--
-- Name: gatehouse_project_record_schemas gatehouse_project_record_schemas_workspace_project_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_schemas
    ADD CONSTRAINT gatehouse_project_record_schemas_workspace_project_name_key UNIQUE (workspace, project, name);


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_id_key UNIQUE (id);


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_pkey PRIMARY KEY (workspace, project, schema, record, id);


--
-- Name: gatehouse_project_records gatehouse_project_records_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_records
    ADD CONSTRAINT gatehouse_project_records_id_key UNIQUE (id);


--
-- Name: gatehouse_project_records gatehouse_project_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_records
    ADD CONSTRAINT gatehouse_project_records_pkey PRIMARY KEY (workspace, project, schema, id);


--
-- Name: gatehouse_project_secrets gatehouse_project_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_secrets
    ADD CONSTRAINT gatehouse_project_secrets_pkey PRIMARY KEY (workspace, project, id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_id_key UNIQUE (id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_pkey PRIMARY KEY (workspace, project, id);


--
-- Name: gatehouse_projects gatehouse_projects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_projects
    ADD CONSTRAINT gatehouse_projects_pkey PRIMARY KEY (workspace, id);


--
-- Name: gatehouse_session_approval_decisions gatehouse_session_approval_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_approval_decisions
    ADD CONSTRAINT gatehouse_session_approval_decisions_pkey PRIMARY KEY (workspace, session, approval);


--
-- Name: gatehouse_session_events gatehouse_session_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_events
    ADD CONSTRAINT gatehouse_session_events_pkey PRIMARY KEY (workspace, session, id);


--
-- Name: gatehouse_session_files gatehouse_session_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_files
    ADD CONSTRAINT gatehouse_session_files_pkey PRIMARY KEY (workspace, session, id);


--
-- Name: gatehouse_session_files gatehouse_session_files_storage_object_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_files
    ADD CONSTRAINT gatehouse_session_files_storage_object_key UNIQUE (storage_object);


--
-- Name: gatehouse_session_grants gatehouse_session_grants_id_once; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_grants
    ADD CONSTRAINT gatehouse_session_grants_id_once UNIQUE (id);


--
-- Name: gatehouse_session_note_revisions gatehouse_session_note_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_note_revisions
    ADD CONSTRAINT gatehouse_session_note_revisions_pkey PRIMARY KEY (workspace, session, note, revision);


--
-- Name: gatehouse_session_notes gatehouse_session_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_notes
    ADD CONSTRAINT gatehouse_session_notes_pkey PRIMARY KEY (workspace, session, id);


--
-- Name: gatehouse_session_secrets gatehouse_session_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_secrets
    ADD CONSTRAINT gatehouse_session_secrets_pkey PRIMARY KEY (workspace, session, id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_id_key UNIQUE (id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_pkey PRIMARY KEY (workspace, session, id);


--
-- Name: gatehouse_sessions gatehouse_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_sessions
    ADD CONSTRAINT gatehouse_sessions_pkey PRIMARY KEY (workspace, id);


--
-- Name: gatehouse_storage_objects gatehouse_storage_objects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_storage_objects
    ADD CONSTRAINT gatehouse_storage_objects_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_storage_providers gatehouse_storage_providers_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_storage_providers
    ADD CONSTRAINT gatehouse_storage_providers_alias_key UNIQUE (alias);


--
-- Name: gatehouse_storage_providers gatehouse_storage_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_storage_providers
    ADD CONSTRAINT gatehouse_storage_providers_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_system_grants gatehouse_system_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_system_grants
    ADD CONSTRAINT gatehouse_system_grants_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_system_grants gatehouse_system_grants_principal_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_system_grants
    ADD CONSTRAINT gatehouse_system_grants_principal_key UNIQUE (principal);


--
-- Name: gatehouse_workspace_agents gatehouse_workspace_agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_agents
    ADD CONSTRAINT gatehouse_workspace_agents_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_workspace_agents gatehouse_workspace_agents_workspace_id_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_agents
    ADD CONSTRAINT gatehouse_workspace_agents_workspace_id_alias_key UNIQUE (workspace_id, alias);


--
-- Name: gatehouse_workspace_agents gatehouse_workspace_agents_workspace_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_agents
    ADD CONSTRAINT gatehouse_workspace_agents_workspace_id_id_key UNIQUE (workspace_id, id);


--
-- Name: gatehouse_workspace_grants gatehouse_workspace_grants_id_once; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_grants
    ADD CONSTRAINT gatehouse_workspace_grants_id_once UNIQUE (id);


--
-- Name: gatehouse_workspace_storage_providers gatehouse_workspace_storage_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_storage_providers
    ADD CONSTRAINT gatehouse_workspace_storage_providers_pkey PRIMARY KEY (workspace, provider);


--
-- Name: gatehouse_workspaces gatehouse_workspaces_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspaces
    ADD CONSTRAINT gatehouse_workspaces_alias_key UNIQUE (alias);


--
-- Name: gatehouse_workspaces gatehouse_workspaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspaces
    ADD CONSTRAINT gatehouse_workspaces_pkey PRIMARY KEY (id);


--
-- Name: gatehouse_activity_event_topics_by_topic_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_activity_event_topics_by_topic_cursor ON public.gatehouse_activity_event_topics USING btree (topic, activity);


--
-- Name: gatehouse_agent_contexts_latest; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_agent_contexts_latest ON public.gatehouse_agent_contexts USING btree (workspace, session, model, profile, updated_at DESC, root DESC);


--
-- Name: gatehouse_group_members_by_principal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_group_members_by_principal ON public.gatehouse_group_members USING btree (principal_id);


--
-- Name: gatehouse_group_members_group_principal_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_group_members_group_principal_once ON public.gatehouse_group_members USING btree (group_id, principal_id);


--
-- Name: gatehouse_groups_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_groups_id_once ON public.gatehouse_groups USING btree (id);


--
-- Name: gatehouse_identities_by_principal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_identities_by_principal ON public.gatehouse_identities USING btree (principal_id);


--
-- Name: gatehouse_project_files_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_files_id_once ON public.gatehouse_project_files USING btree (id);


--
-- Name: gatehouse_project_grants_group_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_grants_group_enabled ON public.gatehouse_project_grants USING btree (workspace, "group", project) WHERE (("group" IS NOT NULL) AND (enabled = true));


--
-- Name: gatehouse_project_grants_group_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_grants_group_once ON public.gatehouse_project_grants USING btree (workspace, project, role, "group") WHERE ("group" IS NOT NULL);


--
-- Name: gatehouse_project_grants_principal_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_grants_principal_enabled ON public.gatehouse_project_grants USING btree (principal, workspace, project) WHERE ((principal IS NOT NULL) AND (enabled = true));


--
-- Name: gatehouse_project_grants_principal_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_grants_principal_once ON public.gatehouse_project_grants USING btree (workspace, project, role, principal) WHERE (principal IS NOT NULL);


--
-- Name: gatehouse_project_notes_by_project_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_notes_by_project_created ON public.gatehouse_project_notes USING btree (workspace, project, created_at DESC, id DESC);


--
-- Name: gatehouse_project_notes_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_notes_id_once ON public.gatehouse_project_notes USING btree (id);


--
-- Name: gatehouse_project_record_attributes_by_schema_display_order_nam; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_record_attributes_by_schema_display_order_nam ON public.gatehouse_project_record_attributes USING btree (workspace, project, schema, display_order, name, id);


--
-- Name: gatehouse_project_record_attributes_by_schema_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_record_attributes_by_schema_name ON public.gatehouse_project_record_attributes USING btree (workspace, project, schema, name);


--
-- Name: gatehouse_project_record_schemas_by_project_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_record_schemas_by_project_created ON public.gatehouse_project_record_schemas USING btree (workspace, project, created_at DESC, id DESC);


--
-- Name: gatehouse_project_record_values_by_file_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_record_values_by_file_reference ON public.gatehouse_project_record_values USING btree (workspace, project, value_reference_file, id DESC) WHERE (value_type = 'file'::text);


--
-- Name: gatehouse_project_record_values_by_record_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_record_values_by_record_created ON public.gatehouse_project_record_values USING btree (workspace, project, schema, record, created_at DESC, id DESC);


--
-- Name: gatehouse_project_record_values_by_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_record_values_by_reference ON public.gatehouse_project_record_values USING btree (workspace, project, value_reference_schema, value_reference, id DESC) WHERE (value_type = 'record'::text);


--
-- Name: gatehouse_project_record_values_global_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_record_values_global_unique ON public.gatehouse_project_record_values USING btree (workspace, project, schema, attribute, value_key) WHERE (attribute_uniqueness = 'global'::text);


--
-- Name: gatehouse_project_record_values_one; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_record_values_one ON public.gatehouse_project_record_values USING btree (workspace, project, schema, record, attribute) WHERE (attribute_cardinality = 'one'::text);


--
-- Name: gatehouse_project_record_values_record_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_record_values_record_unique ON public.gatehouse_project_record_values USING btree (workspace, project, schema, record, attribute, value_key) WHERE (attribute_uniqueness = 'record'::text);


--
-- Name: gatehouse_project_records_by_schema_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_records_by_schema_created ON public.gatehouse_project_records USING btree (workspace, project, schema, created_at DESC, id DESC);


--
-- Name: gatehouse_project_secrets_by_project_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_secrets_by_project_created ON public.gatehouse_project_secrets USING btree (workspace, project, created_at DESC, id DESC);


--
-- Name: gatehouse_project_secrets_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_project_secrets_id_once ON public.gatehouse_project_secrets USING btree (id);


--
-- Name: gatehouse_project_tasks_by_project_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_project_tasks_by_project_created ON public.gatehouse_project_tasks USING btree (workspace, project, created_at DESC, id DESC);


--
-- Name: gatehouse_projects_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_projects_id_once ON public.gatehouse_projects USING btree (id);


--
-- Name: gatehouse_session_approval_decisions_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_approval_decisions_pending ON public.gatehouse_session_approval_decisions USING btree (delivered, created_at, response);


--
-- Name: gatehouse_session_events_by_session_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_events_by_session_order ON public.gatehouse_session_events USING btree (workspace, session, id);


--
-- Name: gatehouse_session_events_children; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_events_children ON public.gatehouse_session_events USING btree (workspace, session, parent, id);


--
-- Name: gatehouse_session_events_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_session_events_id_once ON public.gatehouse_session_events USING btree (id);


--
-- Name: gatehouse_session_events_roots; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_events_roots ON public.gatehouse_session_events USING btree (workspace, session, id) WHERE (parent IS NULL);


--
-- Name: gatehouse_session_files_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_session_files_id_once ON public.gatehouse_session_files USING btree (id);


--
-- Name: gatehouse_session_grants_group_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_grants_group_enabled ON public.gatehouse_session_grants USING btree (workspace, "group", session) WHERE (("group" IS NOT NULL) AND (enabled = true));


--
-- Name: gatehouse_session_grants_group_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_session_grants_group_once ON public.gatehouse_session_grants USING btree (workspace, session, role, "group") WHERE ("group" IS NOT NULL);


--
-- Name: gatehouse_session_grants_principal_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_grants_principal_enabled ON public.gatehouse_session_grants USING btree (principal, workspace, session) WHERE ((principal IS NOT NULL) AND (enabled = true));


--
-- Name: gatehouse_session_grants_principal_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_session_grants_principal_once ON public.gatehouse_session_grants USING btree (workspace, session, role, principal) WHERE (principal IS NOT NULL);


--
-- Name: gatehouse_session_notes_by_session_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_notes_by_session_created ON public.gatehouse_session_notes USING btree (workspace, session, created_at DESC, id DESC);


--
-- Name: gatehouse_session_notes_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_session_notes_id_once ON public.gatehouse_session_notes USING btree (id);


--
-- Name: gatehouse_session_secrets_by_session_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_secrets_by_session_created ON public.gatehouse_session_secrets USING btree (workspace, session, created_at DESC, id DESC);


--
-- Name: gatehouse_session_secrets_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_session_secrets_id_once ON public.gatehouse_session_secrets USING btree (id);


--
-- Name: gatehouse_session_tasks_by_session_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_session_tasks_by_session_created ON public.gatehouse_session_tasks USING btree (workspace, session, created_at DESC, id DESC);


--
-- Name: gatehouse_sessions_by_workspace; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_sessions_by_workspace ON public.gatehouse_sessions USING btree (workspace, created_at);


--
-- Name: gatehouse_sessions_id_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_sessions_id_once ON public.gatehouse_sessions USING btree (id);


--
-- Name: gatehouse_workspace_agents_default_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_workspace_agents_default_once ON public.gatehouse_workspace_agents USING btree (workspace_id) WHERE "default";


--
-- Name: gatehouse_workspace_grants_group_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_workspace_grants_group_enabled ON public.gatehouse_workspace_grants USING btree (workspace, "group") WHERE (("group" IS NOT NULL) AND (enabled = true));


--
-- Name: gatehouse_workspace_grants_group_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_workspace_grants_group_once ON public.gatehouse_workspace_grants USING btree (workspace, role, "group") WHERE ("group" IS NOT NULL);


--
-- Name: gatehouse_workspace_grants_principal_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gatehouse_workspace_grants_principal_enabled ON public.gatehouse_workspace_grants USING btree (principal, workspace) WHERE ((principal IS NOT NULL) AND (enabled = true));


--
-- Name: gatehouse_workspace_grants_principal_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX gatehouse_workspace_grants_principal_once ON public.gatehouse_workspace_grants USING btree (workspace, role, principal) WHERE (principal IS NOT NULL);


--
-- Name: gatehouse_project_files gatehouse_project_files_reject_remove_when_referenced; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER gatehouse_project_files_reject_remove_when_referenced BEFORE UPDATE OF enabled ON public.gatehouse_project_files FOR EACH ROW EXECUTE FUNCTION public.gatehouse_project_file_remove_referenced_validate();


--
-- Name: gatehouse_project_note_revisions gatehouse_project_note_revisions_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER gatehouse_project_note_revisions_immutable BEFORE DELETE OR UPDATE ON public.gatehouse_project_note_revisions FOR EACH ROW EXECUTE FUNCTION public.gatehouse_note_revision_immutable();


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_file_reference_validate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER gatehouse_project_record_values_file_reference_validate BEFORE INSERT OR UPDATE ON public.gatehouse_project_record_values FOR EACH ROW EXECUTE FUNCTION public.gatehouse_project_record_file_reference_validate();


--
-- Name: gatehouse_session_note_revisions gatehouse_session_note_revisions_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER gatehouse_session_note_revisions_immutable BEFORE DELETE OR UPDATE ON public.gatehouse_session_note_revisions FOR EACH ROW EXECUTE FUNCTION public.gatehouse_note_revision_immutable();


--
-- Name: gatehouse_activity_event_topics gatehouse_activity_event_topics_activity_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_event_topics
    ADD CONSTRAINT gatehouse_activity_event_topics_activity_fkey FOREIGN KEY (activity) REFERENCES public.gatehouse_activity_events(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_agent_model_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_agent_model_fkey FOREIGN KEY (resource_agent_model) REFERENCES public.gatehouse_agent_models(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_agent_provider_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_agent_provider_fkey FOREIGN KEY (resource_agent_provider) REFERENCES public.gatehouse_agent_providers(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_group_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_group_fkey FOREIGN KEY (resource_group) REFERENCES public.gatehouse_groups(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_group_member_group_reso_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_group_member_group_reso_fkey FOREIGN KEY (resource_group_member_group, resource_group_member_principal) REFERENCES public.gatehouse_group_members(group_id, principal_id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_identity_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_identity_fkey FOREIGN KEY (resource_identity) REFERENCES public.gatehouse_identities(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_keychain_id_resource_ke_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_keychain_id_resource_ke_fkey FOREIGN KEY (resource_keychain_id, resource_keychain_version) REFERENCES public.gatehouse_keychains(id, version);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_principal_fkey FOREIGN KEY (resource_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_project_file_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_project_file_fkey FOREIGN KEY (resource_project_file) REFERENCES public.gatehouse_project_files(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_project_fkey FOREIGN KEY (resource_project) REFERENCES public.gatehouse_projects(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_project_grant_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_project_grant_fkey FOREIGN KEY (resource_project_grant) REFERENCES public.gatehouse_project_grants(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_project_note_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_project_note_fkey FOREIGN KEY (resource_project_note) REFERENCES public.gatehouse_project_notes(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_project_secret_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_project_secret_fkey FOREIGN KEY (resource_project_secret) REFERENCES public.gatehouse_project_secrets(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_project_task_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_project_task_fkey FOREIGN KEY (resource_project_task) REFERENCES public.gatehouse_project_tasks(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_event_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_event_fkey FOREIGN KEY (resource_session_event) REFERENCES public.gatehouse_session_events(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_file_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_file_fkey FOREIGN KEY (resource_session_file) REFERENCES public.gatehouse_session_files(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_fkey FOREIGN KEY (resource_session) REFERENCES public.gatehouse_sessions(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_grant_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_grant_fkey FOREIGN KEY (resource_session_grant) REFERENCES public.gatehouse_session_grants(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_note_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_note_fkey FOREIGN KEY (resource_session_note) REFERENCES public.gatehouse_session_notes(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_secret_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_secret_fkey FOREIGN KEY (resource_session_secret) REFERENCES public.gatehouse_session_secrets(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_session_task_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_session_task_fkey FOREIGN KEY (resource_session_task) REFERENCES public.gatehouse_session_tasks(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_storage_provider_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_storage_provider_fkey FOREIGN KEY (resource_storage_provider) REFERENCES public.gatehouse_storage_providers(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_system_grant_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_system_grant_fkey FOREIGN KEY (resource_system_grant) REFERENCES public.gatehouse_system_grants(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_workspace_agent_workspa_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_workspace_agent_workspa_fkey FOREIGN KEY (resource_workspace_agent_workspace, resource_workspace_agent_id) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_workspace_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_workspace_fkey FOREIGN KEY (resource_workspace) REFERENCES public.gatehouse_workspaces(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_workspace_grant_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_workspace_grant_fkey FOREIGN KEY (resource_workspace_grant) REFERENCES public.gatehouse_workspace_grants(id);


--
-- Name: gatehouse_activity_events gatehouse_activity_events_resource_workspace_storage_provi_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_activity_events
    ADD CONSTRAINT gatehouse_activity_events_resource_workspace_storage_provi_fkey FOREIGN KEY (resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider) REFERENCES public.gatehouse_workspace_storage_providers(workspace, provider);


--
-- Name: gatehouse_agent_contexts gatehouse_agent_contexts_workspace_model_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_contexts
    ADD CONSTRAINT gatehouse_agent_contexts_workspace_model_fkey FOREIGN KEY (workspace, model) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_agent_contexts gatehouse_agent_contexts_workspace_session_root_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_contexts
    ADD CONSTRAINT gatehouse_agent_contexts_workspace_session_root_fkey FOREIGN KEY (workspace, session, root) REFERENCES public.gatehouse_session_events(workspace, session, id);


--
-- Name: gatehouse_agent_models gatehouse_agent_models_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_models
    ADD CONSTRAINT gatehouse_agent_models_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.gatehouse_agent_providers(id);


--
-- Name: gatehouse_agent_providers gatehouse_agent_providers_keychain_id_keychain_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_providers
    ADD CONSTRAINT gatehouse_agent_providers_keychain_id_keychain_version_fkey FOREIGN KEY (keychain_id, keychain_version) REFERENCES public.gatehouse_keychains(id, version);


--
-- Name: gatehouse_agent_tasks__session_event_reply gatehouse_agent_tasks__session_eve_workspace_session_event_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_tasks__session_event_reply
    ADD CONSTRAINT gatehouse_agent_tasks__session_eve_workspace_session_event_fkey FOREIGN KEY (workspace, session, event) REFERENCES public.gatehouse_session_events(workspace, session, id);


--
-- Name: gatehouse_agent_tasks__session_name gatehouse_agent_tasks__session_name_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_agent_tasks__session_name
    ADD CONSTRAINT gatehouse_agent_tasks__session_name_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_embedded_storage_object_chunks gatehouse_embedded_storage_object__embedded_storage_object_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_embedded_storage_object_chunks
    ADD CONSTRAINT gatehouse_embedded_storage_object__embedded_storage_object_fkey FOREIGN KEY (embedded_storage_object) REFERENCES public.gatehouse_embedded_storage_objects(id);


--
-- Name: gatehouse_group_members gatehouse_group_members_principal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_group_members
    ADD CONSTRAINT gatehouse_group_members_principal_id_fkey FOREIGN KEY (principal_id) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_group_members gatehouse_group_members_workspace_id_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_group_members
    ADD CONSTRAINT gatehouse_group_members_workspace_id_group_id_fkey FOREIGN KEY (workspace_id, group_id) REFERENCES public.gatehouse_groups(workspace_id, id);


--
-- Name: gatehouse_groups gatehouse_groups_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_groups
    ADD CONSTRAINT gatehouse_groups_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.gatehouse_workspaces(id);


--
-- Name: gatehouse_identities gatehouse_identities_principal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_identities
    ADD CONSTRAINT gatehouse_identities_principal_id_fkey FOREIGN KEY (principal_id) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_files gatehouse_project_files_storage_object_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_files
    ADD CONSTRAINT gatehouse_project_files_storage_object_fkey FOREIGN KEY (storage_object) REFERENCES public.gatehouse_storage_objects(id);


--
-- Name: gatehouse_project_files gatehouse_project_files_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_files
    ADD CONSTRAINT gatehouse_project_files_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_project_grants gatehouse_project_grants_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_grants
    ADD CONSTRAINT gatehouse_project_grants_principal_fkey FOREIGN KEY (principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_grants gatehouse_project_grants_workspace_group_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_grants
    ADD CONSTRAINT gatehouse_project_grants_workspace_group_fkey FOREIGN KEY (workspace, "group") REFERENCES public.gatehouse_groups(workspace_id, id);


--
-- Name: gatehouse_project_grants gatehouse_project_grants_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_grants
    ADD CONSTRAINT gatehouse_project_grants_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_project_note_revisions gatehouse_project_note_revisions_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_note_revisions
    ADD CONSTRAINT gatehouse_project_note_revisions_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_note_revisions gatehouse_project_note_revisions_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_note_revisions
    ADD CONSTRAINT gatehouse_project_note_revisions_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_note_revisions gatehouse_project_note_revisions_workspace_project_note_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_note_revisions
    ADD CONSTRAINT gatehouse_project_note_revisions_workspace_project_note_fkey FOREIGN KEY (workspace, project, note) REFERENCES public.gatehouse_project_notes(workspace, project, id);


--
-- Name: gatehouse_project_notes gatehouse_project_notes_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_notes
    ADD CONSTRAINT gatehouse_project_notes_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_notes gatehouse_project_notes_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_notes
    ADD CONSTRAINT gatehouse_project_notes_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_notes gatehouse_project_notes_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_notes
    ADD CONSTRAINT gatehouse_project_notes_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attr_workspace_project_target_sch_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attr_workspace_project_target_sch_fkey FOREIGN KEY (workspace, project, target_schema) REFERENCES public.gatehouse_project_record_schemas(workspace, project, id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attribut_workspace_project_schema_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attribut_workspace_project_schema_fkey FOREIGN KEY (workspace, project, schema) REFERENCES public.gatehouse_project_record_schemas(workspace, project, id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attributes_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attributes_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_record_attributes gatehouse_project_record_attributes_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_attributes
    ADD CONSTRAINT gatehouse_project_record_attributes_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_record_schemas gatehouse_project_record_schemas_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_schemas
    ADD CONSTRAINT gatehouse_project_record_schemas_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_record_schemas gatehouse_project_record_schemas_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_schemas
    ADD CONSTRAINT gatehouse_project_record_schemas_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_record_schemas gatehouse_project_record_schemas_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_schemas
    ADD CONSTRAINT gatehouse_project_record_schemas_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_project_record_values gatehouse_project_record_valu_workspace_project_schema_rec_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_valu_workspace_project_schema_rec_fkey FOREIGN KEY (workspace, project, schema, record) REFERENCES public.gatehouse_project_records(workspace, project, schema, id) ON DELETE CASCADE;


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_attribute_compatibility_foreign; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_attribute_compatibility_foreign FOREIGN KEY (workspace, project, schema, attribute, value_type, attribute_cardinality, attribute_uniqueness) REFERENCES public.gatehouse_project_record_attributes(workspace, project, schema, id, type, cardinality, uniqueness) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_attribute_reference_foreign_key; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_attribute_reference_foreign_key FOREIGN KEY (workspace, project, schema, attribute, value_reference_schema) REFERENCES public.gatehouse_project_record_attributes(workspace, project, schema, id, target_schema) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_file_reference_foreign_key; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_file_reference_foreign_key FOREIGN KEY (workspace, project, value_reference_file) REFERENCES public.gatehouse_project_files(workspace, project, id) ON DELETE RESTRICT;


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_reference_foreign_key; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_reference_foreign_key FOREIGN KEY (workspace, project, value_reference_schema, value_reference) REFERENCES public.gatehouse_project_records(workspace, project, schema, id) ON DELETE RESTRICT;


--
-- Name: gatehouse_project_record_values gatehouse_project_record_values_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_record_values
    ADD CONSTRAINT gatehouse_project_record_values_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_records gatehouse_project_records_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_records
    ADD CONSTRAINT gatehouse_project_records_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_records gatehouse_project_records_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_records
    ADD CONSTRAINT gatehouse_project_records_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_records gatehouse_project_records_workspace_project_schema_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_records
    ADD CONSTRAINT gatehouse_project_records_workspace_project_schema_fkey FOREIGN KEY (workspace, project, schema) REFERENCES public.gatehouse_project_record_schemas(workspace, project, id);


--
-- Name: gatehouse_project_secrets gatehouse_project_secrets_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_secrets
    ADD CONSTRAINT gatehouse_project_secrets_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_secrets gatehouse_project_secrets_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_secrets
    ADD CONSTRAINT gatehouse_project_secrets_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_creator_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_creator_principal_fkey FOREIGN KEY (creator_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_updater_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_updater_principal_fkey FOREIGN KEY (updater_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_workspace_creator_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_workspace_creator_agent_fkey FOREIGN KEY (workspace, creator_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_project_tasks gatehouse_project_tasks_workspace_updater_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_project_tasks
    ADD CONSTRAINT gatehouse_project_tasks_workspace_updater_agent_fkey FOREIGN KEY (workspace, updater_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_projects gatehouse_projects_workspace_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_projects
    ADD CONSTRAINT gatehouse_projects_workspace_fkey FOREIGN KEY (workspace) REFERENCES public.gatehouse_workspaces(id);


--
-- Name: gatehouse_session_approval_decisions gatehouse_session_approval_deci_workspace_session_approval_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_approval_decisions
    ADD CONSTRAINT gatehouse_session_approval_deci_workspace_session_approval_fkey FOREIGN KEY (workspace, session, approval) REFERENCES public.gatehouse_session_events(workspace, session, id);


--
-- Name: gatehouse_session_approval_decisions gatehouse_session_approval_deci_workspace_session_response_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_approval_decisions
    ADD CONSTRAINT gatehouse_session_approval_deci_workspace_session_response_fkey FOREIGN KEY (workspace, session, response) REFERENCES public.gatehouse_session_events(workspace, session, id);


--
-- Name: gatehouse_session_events gatehouse_session_events_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_events
    ADD CONSTRAINT gatehouse_session_events_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_events gatehouse_session_events_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_events
    ADD CONSTRAINT gatehouse_session_events_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_session_events gatehouse_session_events_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_events
    ADD CONSTRAINT gatehouse_session_events_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_session_events gatehouse_session_events_workspace_session_parent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_events
    ADD CONSTRAINT gatehouse_session_events_workspace_session_parent_fkey FOREIGN KEY (workspace, session, parent) REFERENCES public.gatehouse_session_events(workspace, session, id);


--
-- Name: gatehouse_session_files gatehouse_session_files_storage_object_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_files
    ADD CONSTRAINT gatehouse_session_files_storage_object_fkey FOREIGN KEY (storage_object) REFERENCES public.gatehouse_storage_objects(id);


--
-- Name: gatehouse_session_files gatehouse_session_files_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_files
    ADD CONSTRAINT gatehouse_session_files_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_session_grants gatehouse_session_grants_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_grants
    ADD CONSTRAINT gatehouse_session_grants_principal_fkey FOREIGN KEY (principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_grants gatehouse_session_grants_workspace_group_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_grants
    ADD CONSTRAINT gatehouse_session_grants_workspace_group_fkey FOREIGN KEY (workspace, "group") REFERENCES public.gatehouse_groups(workspace_id, id);


--
-- Name: gatehouse_session_grants gatehouse_session_grants_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_grants
    ADD CONSTRAINT gatehouse_session_grants_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_session_note_revisions gatehouse_session_note_revisions_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_note_revisions
    ADD CONSTRAINT gatehouse_session_note_revisions_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_note_revisions gatehouse_session_note_revisions_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_note_revisions
    ADD CONSTRAINT gatehouse_session_note_revisions_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_session_note_revisions gatehouse_session_note_revisions_workspace_session_note_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_note_revisions
    ADD CONSTRAINT gatehouse_session_note_revisions_workspace_session_note_fkey FOREIGN KEY (workspace, session, note) REFERENCES public.gatehouse_session_notes(workspace, session, id);


--
-- Name: gatehouse_session_notes gatehouse_session_notes_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_notes
    ADD CONSTRAINT gatehouse_session_notes_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_notes gatehouse_session_notes_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_notes
    ADD CONSTRAINT gatehouse_session_notes_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_session_notes gatehouse_session_notes_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_notes
    ADD CONSTRAINT gatehouse_session_notes_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_session_secrets gatehouse_session_secrets_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_secrets
    ADD CONSTRAINT gatehouse_session_secrets_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_secrets gatehouse_session_secrets_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_secrets
    ADD CONSTRAINT gatehouse_session_secrets_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_creator_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_creator_principal_fkey FOREIGN KEY (creator_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_updater_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_updater_principal_fkey FOREIGN KEY (updater_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_workspace_creator_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_workspace_creator_agent_fkey FOREIGN KEY (workspace, creator_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_workspace_session_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_workspace_session_fkey FOREIGN KEY (workspace, session) REFERENCES public.gatehouse_sessions(workspace, id);


--
-- Name: gatehouse_session_tasks gatehouse_session_tasks_workspace_updater_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_session_tasks
    ADD CONSTRAINT gatehouse_session_tasks_workspace_updater_agent_fkey FOREIGN KEY (workspace, updater_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_sessions gatehouse_sessions_author_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_sessions
    ADD CONSTRAINT gatehouse_sessions_author_principal_fkey FOREIGN KEY (author_principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_sessions gatehouse_sessions_workspace_author_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_sessions
    ADD CONSTRAINT gatehouse_sessions_workspace_author_agent_fkey FOREIGN KEY (workspace, author_agent) REFERENCES public.gatehouse_workspace_agents(workspace_id, id);


--
-- Name: gatehouse_sessions gatehouse_sessions_workspace_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_sessions
    ADD CONSTRAINT gatehouse_sessions_workspace_fkey FOREIGN KEY (workspace) REFERENCES public.gatehouse_workspaces(id);


--
-- Name: gatehouse_sessions gatehouse_sessions_workspace_project_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_sessions
    ADD CONSTRAINT gatehouse_sessions_workspace_project_fkey FOREIGN KEY (workspace, project) REFERENCES public.gatehouse_projects(workspace, id);


--
-- Name: gatehouse_storage_objects gatehouse_storage_objects_provider_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_storage_objects
    ADD CONSTRAINT gatehouse_storage_objects_provider_fkey FOREIGN KEY (provider) REFERENCES public.gatehouse_storage_providers(id);


--
-- Name: gatehouse_storage_providers gatehouse_storage_providers_keychain_id_keychain_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_storage_providers
    ADD CONSTRAINT gatehouse_storage_providers_keychain_id_keychain_version_fkey FOREIGN KEY (keychain_id, keychain_version) REFERENCES public.gatehouse_keychains(id, version);


--
-- Name: gatehouse_system_grants gatehouse_system_grants_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_system_grants
    ADD CONSTRAINT gatehouse_system_grants_principal_fkey FOREIGN KEY (principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_workspace_agents gatehouse_workspace_agents_model_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_agents
    ADD CONSTRAINT gatehouse_workspace_agents_model_id_fkey FOREIGN KEY (model_id) REFERENCES public.gatehouse_agent_models(id);


--
-- Name: gatehouse_workspace_agents gatehouse_workspace_agents_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_agents
    ADD CONSTRAINT gatehouse_workspace_agents_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.gatehouse_workspaces(id);


--
-- Name: gatehouse_workspace_grants gatehouse_workspace_grants_principal_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_grants
    ADD CONSTRAINT gatehouse_workspace_grants_principal_fkey FOREIGN KEY (principal) REFERENCES public.gatehouse_principals(id);


--
-- Name: gatehouse_workspace_grants gatehouse_workspace_grants_workspace_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_grants
    ADD CONSTRAINT gatehouse_workspace_grants_workspace_fkey FOREIGN KEY (workspace) REFERENCES public.gatehouse_workspaces(id);


--
-- Name: gatehouse_workspace_grants gatehouse_workspace_grants_workspace_group_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_grants
    ADD CONSTRAINT gatehouse_workspace_grants_workspace_group_fkey FOREIGN KEY (workspace, "group") REFERENCES public.gatehouse_groups(workspace_id, id);


--
-- Name: gatehouse_workspace_storage_providers gatehouse_workspace_storage_providers_provider_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_storage_providers
    ADD CONSTRAINT gatehouse_workspace_storage_providers_provider_fkey FOREIGN KEY (provider) REFERENCES public.gatehouse_storage_providers(id);


--
-- Name: gatehouse_workspace_storage_providers gatehouse_workspace_storage_providers_workspace_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gatehouse_workspace_storage_providers
    ADD CONSTRAINT gatehouse_workspace_storage_providers_workspace_fkey FOREIGN KEY (workspace) REFERENCES public.gatehouse_workspaces(id);


--
-- PostgreSQL database dump complete
--
