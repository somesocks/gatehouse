package migrations

func sqliteMigrationV034AddGrantIDs() VersionedMigration {
	return VersionedMigration{
		Index:       34,
		Description: "add_grant_ids",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_workspace_grants_migration_34 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id),
				"group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL CHECK (revision > 0),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_workspace_grants_migration_34 (id, workspace, role, principal, "group", enabled, revision)
			SELECT gh_id_new('wgr'), workspace, role, principal, "group", enabled, revision FROM gatehouse_workspace_grants;
			DROP TABLE gatehouse_workspace_grants;
			ALTER TABLE gatehouse_workspace_grants_migration_34 RENAME TO gatehouse_workspace_grants;
			CREATE UNIQUE INDEX gatehouse_workspace_grants_principal_once ON gatehouse_workspace_grants (workspace, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_workspace_grants_group_once ON gatehouse_workspace_grants (workspace, role, "group") WHERE "group" IS NOT NULL;
			CREATE INDEX gatehouse_workspace_grants_principal_enabled ON gatehouse_workspace_grants (principal, workspace) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_workspace_grants_group_enabled ON gatehouse_workspace_grants (workspace, "group") WHERE "group" IS NOT NULL AND enabled = TRUE;

			CREATE TABLE gatehouse_project_grants_migration_34 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL,
				project TEXT NOT NULL,
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id),
				"group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_project_grants_migration_34 (id, workspace, project, role, principal, "group", enabled)
			SELECT gh_id_new('pgr'), workspace, project, role, principal, "group", enabled FROM gatehouse_project_grants;
			DROP TABLE gatehouse_project_grants;
			ALTER TABLE gatehouse_project_grants_migration_34 RENAME TO gatehouse_project_grants;
			CREATE UNIQUE INDEX gatehouse_project_grants_principal_once ON gatehouse_project_grants (workspace, project, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_project_grants_group_once ON gatehouse_project_grants (workspace, project, role, "group") WHERE "group" IS NOT NULL;
			CREATE INDEX gatehouse_project_grants_principal_enabled ON gatehouse_project_grants (principal, workspace, project) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_project_grants_group_enabled ON gatehouse_project_grants (workspace, "group", project) WHERE "group" IS NOT NULL AND enabled = TRUE;

			CREATE TABLE gatehouse_session_grants_migration_34 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL,
				session TEXT NOT NULL,
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id),
				"group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_session_grants_migration_34 (id, workspace, session, role, principal, "group", enabled)
			SELECT gh_id_new('sgr'), workspace, session, role, principal, "group", enabled FROM gatehouse_session_grants;
			DROP TABLE gatehouse_session_grants;
			ALTER TABLE gatehouse_session_grants_migration_34 RENAME TO gatehouse_session_grants;
			CREATE UNIQUE INDEX gatehouse_session_grants_principal_once ON gatehouse_session_grants (workspace, session, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_session_grants_group_once ON gatehouse_session_grants (workspace, session, role, "group") WHERE "group" IS NOT NULL;
			CREATE INDEX gatehouse_session_grants_principal_enabled ON gatehouse_session_grants (principal, workspace, session) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_session_grants_group_enabled ON gatehouse_session_grants (workspace, "group", session) WHERE "group" IS NOT NULL AND enabled = TRUE;
		`),
	}
}
