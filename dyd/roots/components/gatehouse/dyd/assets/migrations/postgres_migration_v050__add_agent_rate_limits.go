package migrations

func postgresMigrationV050AddAgentRateLimits() VersionedMigration {
	return VersionedMigration{
		Index:       50,
		Description: "add_agent_rate_limits",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN rate_limits JSONB CHECK (rate_limits IS NULL OR jsonb_typeof(rate_limits) = 'object');
			CREATE TABLE gatehouse_workspace_agent_rate_buckets (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				binding_id TEXT NOT NULL,
				principal_id TEXT REFERENCES gatehouse_principals (id),
				available_input_tokens DOUBLE PRECISION NOT NULL,
				available_output_tokens DOUBLE PRECISION NOT NULL,
				accounted_at TEXT NOT NULL,
				UNIQUE (workspace_id, binding_id, principal_id),
				FOREIGN KEY (workspace_id, binding_id) REFERENCES gatehouse_workspace_agents (workspace_id, id)
			);
			CREATE UNIQUE INDEX gatehouse_workspace_agent_rate_buckets_global
				ON gatehouse_workspace_agent_rate_buckets (workspace_id, binding_id)
				WHERE principal_id IS NULL;
		`),
	}
}
