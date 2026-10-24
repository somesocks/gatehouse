package migrations

func sqliteMigrationV069RenameRoles() VersionedMigration {
	return VersionedMigration{
		Index: 69, Description: "rename_roles",
		Options: MigrationOptions{SQLite: SQLiteMigrationOptions{DisableForeignKeys: true}},
		Builder: staticMigrationBuilder(`
			-- Copy grants before recreating their unique indexes. A recipient may
			-- have both an old member and contributor grant, including disabled ones.
			CREATE TABLE gatehouse_workspace_grants_migration_69 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'wgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				role TEXT NOT NULL CHECK (role IN ('guest', 'member', 'admin')),
				principal TEXT REFERENCES gatehouse_principals (id), "group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL CHECK (revision > 0),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_workspace_grants_migration_69
			SELECT id, workspace, CASE role WHEN 'member' THEN 'guest' WHEN 'contributor' THEN 'member' WHEN 'manager' THEN 'admin' ELSE role END,
				principal, "group", enabled, revision FROM gatehouse_workspace_grants;
			DROP TABLE gatehouse_workspace_grants;
			ALTER TABLE gatehouse_workspace_grants_migration_69 RENAME TO gatehouse_workspace_grants;
			CREATE UNIQUE INDEX gatehouse_workspace_grants_principal_once ON gatehouse_workspace_grants (workspace, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_workspace_grants_group_once ON gatehouse_workspace_grants (workspace, role, "group") WHERE "group" IS NOT NULL;
			CREATE INDEX gatehouse_workspace_grants_principal_enabled ON gatehouse_workspace_grants (principal, workspace) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_workspace_grants_group_enabled ON gatehouse_workspace_grants (workspace, "group") WHERE "group" IS NOT NULL AND enabled = TRUE;
			-- Retired group grants are copied with their history, then protected again.
			CREATE TRIGGER gatehouse_workspace_grants_principal_insert BEFORE INSERT ON gatehouse_workspace_grants
			WHEN NEW.principal IS NULL OR NEW."group" IS NOT NULL BEGIN SELECT RAISE(ABORT, 'workspace grants require a principal'); END;
			CREATE TRIGGER gatehouse_workspace_grants_principal_update BEFORE UPDATE ON gatehouse_workspace_grants
			WHEN NEW.principal IS NULL OR NEW."group" IS NOT NULL OR OLD."group" IS NOT NULL BEGIN SELECT RAISE(ABORT, 'group workspace grants are retired'); END;

			CREATE TABLE gatehouse_project_grants_migration_69 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'pgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL, project TEXT NOT NULL,
				role TEXT NOT NULL CHECK (role IN ('guest', 'member', 'admin')),
				principal TEXT REFERENCES gatehouse_principals (id), "group" TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
				CHECK ((principal IS NOT NULL AND "group" IS NULL) OR (principal IS NULL AND "group" IS NOT NULL)),
				FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_project_grants_migration_69
			SELECT id, workspace, project, CASE role WHEN 'member' THEN 'guest' WHEN 'contributor' THEN 'member' WHEN 'manager' THEN 'admin' ELSE role END,
				principal, "group", enabled, revision FROM gatehouse_project_grants;
			DROP TABLE gatehouse_project_grants;
			ALTER TABLE gatehouse_project_grants_migration_69 RENAME TO gatehouse_project_grants;
			CREATE UNIQUE INDEX gatehouse_project_grants_principal_once ON gatehouse_project_grants (workspace, project, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_project_grants_group_once ON gatehouse_project_grants (workspace, project, role, "group") WHERE "group" IS NOT NULL;
			CREATE INDEX gatehouse_project_grants_principal_enabled ON gatehouse_project_grants (principal, workspace, project) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_project_grants_group_enabled ON gatehouse_project_grants (workspace, "group", project) WHERE "group" IS NOT NULL AND enabled = TRUE;

			CREATE TABLE gatehouse_session_grants_migration_69 (
				id TEXT NOT NULL UNIQUE CHECK (length(id) = 30 AND substr(id, 1, 4) = 'sgr_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				workspace TEXT NOT NULL, session TEXT NOT NULL,
				role TEXT NOT NULL CHECK (role IN ('guest', 'member', 'admin')),
				principal TEXT REFERENCES gatehouse_principals (id), "group" TEXT, agent TEXT,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
				CHECK ((principal IS NOT NULL) + ("group" IS NOT NULL) + (agent IS NOT NULL) = 1),
				FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id),
				FOREIGN KEY (workspace, agent) REFERENCES gatehouse_workspace_agents (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_session_grants_migration_69
			SELECT id, workspace, session, CASE role WHEN 'member' THEN 'guest' WHEN 'contributor' THEN 'member' WHEN 'manager' THEN 'admin' ELSE role END,
				principal, "group", agent, enabled, revision FROM gatehouse_session_grants;
			DROP TABLE gatehouse_session_grants;
			ALTER TABLE gatehouse_session_grants_migration_69 RENAME TO gatehouse_session_grants;
			CREATE UNIQUE INDEX gatehouse_session_grants_principal_once ON gatehouse_session_grants (workspace, session, role, principal) WHERE principal IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_session_grants_group_once ON gatehouse_session_grants (workspace, session, role, "group") WHERE "group" IS NOT NULL;
			CREATE UNIQUE INDEX gatehouse_session_grants_agent_once ON gatehouse_session_grants (workspace, session, role, agent) WHERE agent IS NOT NULL;
			CREATE INDEX gatehouse_session_grants_principal_enabled ON gatehouse_session_grants (principal, workspace, session) WHERE principal IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_session_grants_group_enabled ON gatehouse_session_grants (workspace, "group", session) WHERE "group" IS NOT NULL AND enabled = TRUE;
			CREATE INDEX gatehouse_session_grants_agent_enabled ON gatehouse_session_grants (workspace, session, agent) WHERE agent IS NOT NULL AND enabled = TRUE;

			CREATE TABLE gatehouse_group_members_migration_69 (
				workspace_id TEXT NOT NULL, group_id TEXT NOT NULL,
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
				role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin')),
				PRIMARY KEY (workspace_id, group_id, principal_id),
				FOREIGN KEY (workspace_id, group_id) REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;
			INSERT INTO gatehouse_group_members_migration_69
			SELECT workspace_id, group_id, principal_id, enabled, revision,
				CASE role WHEN 'manager' THEN 'admin' ELSE role END FROM gatehouse_group_members;
			DROP TABLE gatehouse_group_members;
			ALTER TABLE gatehouse_group_members_migration_69 RENAME TO gatehouse_group_members;
			CREATE INDEX gatehouse_group_members_by_principal ON gatehouse_group_members (principal_id);
			CREATE UNIQUE INDEX gatehouse_group_members_group_principal_once ON gatehouse_group_members (group_id, principal_id);

			CREATE TABLE gatehouse_system_grants_migration_69 (
				id TEXT PRIMARY KEY NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'syg_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				principal TEXT NOT NULL UNIQUE REFERENCES gatehouse_principals (id),
				role TEXT NOT NULL CHECK (role = 'admin'),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				revision INTEGER NOT NULL CHECK (revision > 0)
			) STRICT;
			INSERT INTO gatehouse_system_grants_migration_69
			SELECT id, principal, 'admin', enabled, revision FROM gatehouse_system_grants;
			DROP TABLE gatehouse_system_grants;
			ALTER TABLE gatehouse_system_grants_migration_69 RENAME TO gatehouse_system_grants;

			CREATE TEMP TABLE gatehouse_role_migration_integrity (violations INTEGER CHECK (violations = 0));
			INSERT INTO gatehouse_role_migration_integrity SELECT count(*) FROM pragma_foreign_key_check;
			DROP TABLE gatehouse_role_migration_integrity;
		`),
	}
}
