package migrations

func postgresMigrationV034AddGrantIDs() VersionedMigration {
	return VersionedMigration{
		Index:       34,
		Description: "add_grant_ids",
		Builder: staticMigrationBuilder(`
			CREATE FUNCTION gatehouse_migration_typed_id_new(kind TEXT) RETURNS TEXT AS $$
			DECLARE
				alphabet CONSTANT TEXT := '0123456789abcdefghjkmnpqrstvwxyz';
				milliseconds BIGINT := floor(extract(epoch FROM clock_timestamp()) * 1000)::BIGINT;
				encoded TEXT := '';
				position INTEGER;
			BEGIN
				FOR position IN REVERSE 0..9 LOOP
					encoded := substr(alphabet, (milliseconds % 32)::INTEGER + 1, 1) || encoded;
					milliseconds := milliseconds / 32;
				END LOOP;
				FOR position IN 1..16 LOOP
					encoded := encoded || substr(alphabet, floor(random() * 32)::INTEGER + 1, 1);
				END LOOP;
				RETURN kind || '_' || encoded;
			END;
			$$ LANGUAGE plpgsql VOLATILE;

			ALTER TABLE gatehouse_workspace_grants ADD COLUMN id TEXT;
			UPDATE gatehouse_workspace_grants SET id = gatehouse_migration_typed_id_new('wgr');
			ALTER TABLE gatehouse_workspace_grants
				ALTER COLUMN id SET NOT NULL,
				ADD CONSTRAINT gatehouse_workspace_grants_id_once UNIQUE (id),
				ADD CONSTRAINT gatehouse_workspace_grants_id_check CHECK (id ~ '^wgr_[0-7][0-9a-hjkmnp-tv-z]{25}$');

			ALTER TABLE gatehouse_project_grants ADD COLUMN id TEXT;
			UPDATE gatehouse_project_grants SET id = gatehouse_migration_typed_id_new('pgr');
			ALTER TABLE gatehouse_project_grants
				ALTER COLUMN id SET NOT NULL,
				ADD CONSTRAINT gatehouse_project_grants_id_once UNIQUE (id),
				ADD CONSTRAINT gatehouse_project_grants_id_check CHECK (id ~ '^pgr_[0-7][0-9a-hjkmnp-tv-z]{25}$');

			ALTER TABLE gatehouse_session_grants ADD COLUMN id TEXT;
			UPDATE gatehouse_session_grants SET id = gatehouse_migration_typed_id_new('sgr');
			ALTER TABLE gatehouse_session_grants
				ALTER COLUMN id SET NOT NULL,
				ADD CONSTRAINT gatehouse_session_grants_id_once UNIQUE (id),
				ADD CONSTRAINT gatehouse_session_grants_id_check CHECK (id ~ '^sgr_[0-7][0-9a-hjkmnp-tv-z]{25}$');

			DROP FUNCTION gatehouse_migration_typed_id_new(TEXT);
		`),
	}
}
