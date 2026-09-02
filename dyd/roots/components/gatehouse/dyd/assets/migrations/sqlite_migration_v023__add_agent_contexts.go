package migrations

func sqliteMigrationV023AddAgentContexts() VersionedMigration {
	return VersionedMigration{
		Index:       23,
		Description: "add_agent_contexts",
		Builder: staticMigrationBuilder(`
				CREATE TABLE gatehouse_agent_contexts (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					root TEXT NOT NULL,
					model TEXT NOT NULL,
					profile TEXT NOT NULL CHECK (length(trim(profile)) > 0),
					state TEXT NOT NULL CHECK (json_valid(state)) CHECK (json_type(state) = 'object'),
					updated_at TEXT NOT NULL,
					PRIMARY KEY (workspace, session, root),
					FOREIGN KEY (workspace, session, root) REFERENCES gatehouse_session_events (workspace, session, id),
					FOREIGN KEY (workspace, model) REFERENCES gatehouse_workspace_agents (workspace_id, model_id)
				) STRICT;
			`),
	}
}
