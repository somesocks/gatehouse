package migrations

func postgresMigrationV049WorkspaceAgentDefaults() VersionedMigration {
	return VersionedMigration{
		Index:       49,
		Description: "workspace_agent_defaults",
		Builder: staticMigrationBuilder(`
			ALTER TABLE gatehouse_workspace_agents
				ADD COLUMN prelude TEXT,
				ADD COLUMN "default" BOOLEAN NOT NULL DEFAULT FALSE;
			WITH ranked AS (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY priority DESC, id) AS position
				FROM gatehouse_workspace_agents WHERE enabled
			) UPDATE gatehouse_workspace_agents AS bindings
			SET "default" = TRUE FROM ranked
			WHERE bindings.id = ranked.id AND ranked.position = 1;
			ALTER TABLE gatehouse_workspace_agents
				DROP COLUMN priority,
				DROP CONSTRAINT gatehouse_workspace_agents_alias_check,
				ADD CONSTRAINT gatehouse_workspace_agents_alias_check CHECK (alias ~ '^[a-z0-9_-]+(/[a-z0-9_-]+)*$'),
				ADD CONSTRAINT gatehouse_workspace_agents_default_enabled_check CHECK (NOT "default" OR enabled);
			CREATE UNIQUE INDEX gatehouse_workspace_agents_default_once
			ON gatehouse_workspace_agents (workspace_id) WHERE "default";
		`),
	}
}
