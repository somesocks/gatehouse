package migrations

func sqliteMigrationV050AddAgentRateLimits() VersionedMigration {
	return VersionedMigration{
		Index:       50,
		Description: "add_agent_rate_limits",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN rate_limits TEXT CHECK (rate_limits IS NULL OR (json_valid(rate_limits) AND json_type(rate_limits) = 'object'));
			CREATE TABLE gatehouse_workspace_agent_rate_buckets (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				binding_id TEXT NOT NULL,
				principal_id TEXT REFERENCES gatehouse_principals (id),
				available_input_tokens REAL NOT NULL,
				available_output_tokens REAL NOT NULL,
				accounted_at TEXT NOT NULL,
				UNIQUE (workspace_id, binding_id, principal_id),
				FOREIGN KEY (workspace_id, binding_id) REFERENCES gatehouse_workspace_agents (workspace_id, id)
			) STRICT;
			CREATE UNIQUE INDEX gatehouse_workspace_agent_rate_buckets_global
				ON gatehouse_workspace_agent_rate_buckets (workspace_id, binding_id)
				WHERE principal_id IS NULL;
		`),
	}
}
