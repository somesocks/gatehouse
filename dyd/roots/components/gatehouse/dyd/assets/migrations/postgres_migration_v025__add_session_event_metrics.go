package migrations

func postgresMigrationV025AddSessionEventMetrics() VersionedMigration {
	return VersionedMigration{
		Index:       25,
		Description: "add_session_event_metrics",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_session_events
				ADD COLUMN metrics JSONB CHECK (metrics IS NULL OR jsonb_typeof(metrics) = 'object');
			`),
	}
}
