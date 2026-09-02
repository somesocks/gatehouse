package migrations

func postgresMigrationV026IndexLatestAgentContexts() VersionedMigration {
	return VersionedMigration{
		Index:       26,
		Description: "index_latest_agent_contexts",
		Builder: staticMigrationBuilder(`
				CREATE INDEX gatehouse_agent_contexts_latest
				ON gatehouse_agent_contexts (workspace, session, model, profile, updated_at DESC, root DESC);
			`),
	}
}
