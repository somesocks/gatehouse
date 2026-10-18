package migrations

func sqliteMigrationV060ChatBotParticipants() VersionedMigration {
	return VersionedMigration{
		Index: 60, Description: "chat_bot_participants",
		Options: MigrationOptions{SQLite: SQLiteMigrationOptions{DisableForeignKeys: true}},
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_session_grants_migration_60 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL, session TEXT NOT NULL,
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id), "group" TEXT, agent TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
				CHECK ((principal IS NOT NULL) + ("group" IS NOT NULL) + (agent IS NOT NULL) = 1),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace, agent) REFERENCES gatehouse_workspace_agents (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_session_grants_migration_60 (id, workspace, session, role, principal, "group", enabled, revision)
			SELECT id, workspace, session, role, principal, "group", enabled, revision FROM gatehouse_session_grants;
			DROP TABLE gatehouse_session_grants;
			ALTER TABLE gatehouse_session_grants_migration_60 RENAME TO gatehouse_session_grants;
			CREATE UNIQUE INDEX gatehouse_session_grants_principal_once ON gatehouse_session_grants (workspace, session, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_session_grants_group_once ON gatehouse_session_grants (workspace, session, role, "group") WHERE "group" IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_session_grants_agent_once ON gatehouse_session_grants (workspace, session, role, agent) WHERE agent IS NOT NULL;
			CREATE INDEX gatehouse_session_grants_principal_enabled ON gatehouse_session_grants (principal, workspace, session) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_session_grants_group_enabled ON gatehouse_session_grants (workspace, "group", session) WHERE "group" IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_session_grants_agent_enabled ON gatehouse_session_grants (workspace, session, agent) WHERE agent IS NOT NULL AND enabled = TRUE;
			WITH addressed (workspace, session, agent) AS (
				SELECT workspace, session, author_agent FROM gatehouse_session_events WHERE author_agent IS NOT NULL
				UNION SELECT workspace, id, author_agent FROM gatehouse_sessions WHERE author_agent IS NOT NULL
				UNION SELECT workspace, session, json_extract(payload, '$.agent') FROM gatehouse_session_events WHERE kind = 'agent.request'
				UNION SELECT e.workspace, e.session, j.value FROM gatehouse_session_events e, json_each(CASE WHEN e.kind = 'message.text' AND json_type(e.payload, '$.agents') = 'array' THEN json_extract(e.payload, '$.agents') ELSE '[]' END) j
				UNION SELECT e.workspace, e.session, json_extract(j.value, '$.ref.id') FROM gatehouse_session_events e, json_each(CASE WHEN e.kind = 'message.text' AND json_type(e.payload, '$.targets') = 'array' THEN json_extract(e.payload, '$.targets') ELSE '[]' END) j WHERE json_extract(j.value, '$.ref.kind') = 'agent'
			)
			INSERT INTO gatehouse_session_grants (id, workspace, session, role, agent, enabled, revision)
			SELECT gh_id_new('sgr'), a.workspace, a.session, 'contributor', a.agent, TRUE, 1 FROM addressed a
			JOIN gatehouse_workspace_agents b ON b.workspace_id = a.workspace AND b.id = a.agent;
		`),
	}
}
