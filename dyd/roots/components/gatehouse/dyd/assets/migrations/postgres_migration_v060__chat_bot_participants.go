package migrations

func postgresMigrationV060ChatBotParticipants() VersionedMigration {
	return VersionedMigration{Index: 60, Description: "chat_bot_participants", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_session_grants ADD COLUMN agent TEXT;
		ALTER TABLE gatehouse_session_grants DROP CONSTRAINT gatehouse_session_grants_check;
		ALTER TABLE gatehouse_session_grants ADD CONSTRAINT gatehouse_session_grants_check CHECK (num_nonnulls(principal, "group", agent) = 1);
		ALTER TABLE gatehouse_session_grants ADD FOREIGN KEY (workspace, agent) REFERENCES gatehouse_workspace_agents (workspace_id, id);
		CREATE UNIQUE INDEX gatehouse_session_grants_agent_once ON gatehouse_session_grants (workspace, session, role, agent) WHERE agent IS NOT NULL;
		CREATE INDEX gatehouse_session_grants_agent_enabled ON gatehouse_session_grants (workspace, session, agent) WHERE agent IS NOT NULL AND enabled = TRUE;
		CREATE FUNCTION gatehouse_migration_typed_id_new(kind TEXT) RETURNS TEXT AS $$
		DECLARE
			alphabet CONSTANT TEXT := '0123456789abcdefghjkmnpqrstvwxyz';
			milliseconds BIGINT := floor(extract(epoch FROM clock_timestamp()) * 1000)::BIGINT;
			encoded TEXT := ''; position INTEGER;
		BEGIN
			FOR position IN 1..10 LOOP
				encoded := substr(alphabet, (milliseconds % 32)::INTEGER + 1, 1) || encoded;
				milliseconds := milliseconds / 32;
			END LOOP;
			FOR position IN 1..16 LOOP
				encoded := encoded || substr(alphabet, floor(random() * 32)::INTEGER + 1, 1);
			END LOOP;
			RETURN kind || '_' || encoded;
		END;
		$$ LANGUAGE plpgsql VOLATILE;
		WITH addressed (workspace, session, agent) AS (
			SELECT workspace, session, author_agent FROM gatehouse_session_events WHERE author_agent IS NOT NULL
			UNION SELECT workspace, id, author_agent FROM gatehouse_sessions WHERE author_agent IS NOT NULL
			UNION SELECT workspace, session, payload ->> 'agent' FROM gatehouse_session_events WHERE kind = 'agent.request'
			UNION SELECT e.workspace, e.session, j.value FROM gatehouse_session_events e CROSS JOIN LATERAL jsonb_array_elements_text(CASE WHEN e.kind = 'message.text' AND jsonb_typeof(e.payload -> 'agents') = 'array' THEN e.payload -> 'agents' ELSE '[]'::jsonb END) j
			UNION SELECT e.workspace, e.session, j.value -> 'ref' ->> 'id' FROM gatehouse_session_events e CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN e.kind = 'message.text' AND jsonb_typeof(e.payload -> 'targets') = 'array' THEN e.payload -> 'targets' ELSE '[]'::jsonb END) j WHERE j.value -> 'ref' ->> 'kind' = 'agent'
		)
		INSERT INTO gatehouse_session_grants (id, workspace, session, role, agent, enabled, revision)
		SELECT gatehouse_migration_typed_id_new('sgr'), a.workspace, a.session, 'contributor', a.agent, TRUE, 1 FROM addressed a
		JOIN gatehouse_workspace_agents b ON b.workspace_id = a.workspace AND b.id = a.agent;
		DROP FUNCTION gatehouse_migration_typed_id_new(TEXT);
	`)}
}
