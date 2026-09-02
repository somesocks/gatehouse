package migrations

func sqliteMigrationV025AddSessionEventMetrics() VersionedMigration {
	return VersionedMigration{
		Index:       25,
		Description: "add_session_event_metrics",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_session_events
				ADD COLUMN metrics TEXT CHECK (metrics IS NULL OR (json_valid(metrics) AND json_type(metrics) = 'object'));
			`),
	}
}
