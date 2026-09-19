package migrations

func sqliteMigrationV049WorkspaceAgentDefaults() VersionedMigration {
	return VersionedMigration{
		Index:       49,
		Description: "workspace_agent_defaults",
		Options: MigrationOptions{SQLite: SQLiteMigrationOptions{
			DisableForeignKeys: true,
		}},
		Builder: staticMigrationBuilder(`
			-- Keep dependent foreign keys pointed at the stable table name while replacing
			-- the legacy priority and alias constraints.
			CREATE TABLE gatehouse_workspace_agents_migration_49 (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wag_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				alias TEXT NOT NULL CHECK (alias <> '') CHECK (alias NOT GLOB '*[^a-z0-9_/-]*') CHECK (alias NOT GLOB '/*' AND alias NOT GLOB '*/' AND alias NOT GLOB '*//*'),
				model_id TEXT NOT NULL REFERENCES gatehouse_agent_models (id),
				revision INTEGER NOT NULL CHECK (revision > 0),
				label TEXT,
				system_prompt TEXT,
				prelude TEXT,
				"default" INTEGER NOT NULL CHECK ("default" IN (0, 1)),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				UNIQUE (workspace_id, id), UNIQUE (workspace_id, alias),
				CHECK ("default" = 0 OR enabled = 1)
			) STRICT;
			INSERT INTO gatehouse_workspace_agents_migration_49 (id, workspace_id, alias, model_id, revision, label, system_prompt, prelude, "default", enabled)
			SELECT id, workspace_id, alias, model_id, revision, label, system_prompt, NULL,
				CASE WHEN enabled = 1 AND ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY priority DESC, id) = 1 THEN 1 ELSE 0 END,
				enabled
			FROM gatehouse_workspace_agents;
			DROP TABLE gatehouse_workspace_agents;
			ALTER TABLE gatehouse_workspace_agents_migration_49 RENAME TO gatehouse_workspace_agents;
			CREATE UNIQUE INDEX gatehouse_workspace_agents_default_once
			ON gatehouse_workspace_agents (workspace_id) WHERE "default" = 1;
		`),
	}
}
