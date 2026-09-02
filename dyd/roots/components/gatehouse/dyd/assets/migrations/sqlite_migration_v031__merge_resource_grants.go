package migrations

func sqliteMigrationV031MergeResourceGrants() VersionedMigration {
	return VersionedMigration{
		Index:       31,
		Description: "merge_resource_grants",
		Builder: staticMigrationBuilder(`
				DROP TABLE gatehouse_project_principal_grants;
				DROP TABLE gatehouse_project_group_grants;
				DROP TABLE gatehouse_session_principal_grants;
				DROP TABLE gatehouse_session_group_grants;

				CREATE TABLE gatehouse_project_grants (
					workspace TEXT NOT NULL,
					project TEXT NOT NULL,
					role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
					principal TEXT REFERENCES gatehouse_principals (id),
					"group" TEXT,
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					CHECK (
						(principal IS NOT NULL AND "group" IS NULL)
						OR (principal IS NULL AND "group" IS NOT NULL)
					),
					FOREIGN KEY (workspace, project) REFERENCES gatehouse_projects (workspace, id),
					FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
				) STRICT;

				CREATE UNIQUE INDEX gatehouse_project_grants_principal_once
				ON gatehouse_project_grants (workspace, project, role, principal)
				WHERE principal IS NOT NULL;

				CREATE UNIQUE INDEX gatehouse_project_grants_group_once
				ON gatehouse_project_grants (workspace, project, role, "group")
				WHERE "group" IS NOT NULL;

				CREATE INDEX gatehouse_project_grants_principal_enabled
				ON gatehouse_project_grants (principal, workspace, project)
				WHERE principal IS NOT NULL AND enabled = TRUE;

				CREATE INDEX gatehouse_project_grants_group_enabled
				ON gatehouse_project_grants (workspace, "group", project)
				WHERE "group" IS NOT NULL AND enabled = TRUE;

				CREATE TABLE gatehouse_session_grants (
					workspace TEXT NOT NULL,
					session TEXT NOT NULL,
					role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
					principal TEXT REFERENCES gatehouse_principals (id),
					"group" TEXT,
					enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
					CHECK (
						(principal IS NOT NULL AND "group" IS NULL)
						OR (principal IS NULL AND "group" IS NOT NULL)
					),
					FOREIGN KEY (workspace, session) REFERENCES gatehouse_sessions (workspace, id),
					FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
				) STRICT;

				CREATE UNIQUE INDEX gatehouse_session_grants_principal_once
				ON gatehouse_session_grants (workspace, session, role, principal)
				WHERE principal IS NOT NULL;

				CREATE UNIQUE INDEX gatehouse_session_grants_group_once
				ON gatehouse_session_grants (workspace, session, role, "group")
				WHERE "group" IS NOT NULL;

				CREATE INDEX gatehouse_session_grants_principal_enabled
				ON gatehouse_session_grants (principal, workspace, session)
				WHERE principal IS NOT NULL AND enabled = TRUE;

				CREATE INDEX gatehouse_session_grants_group_enabled
				ON gatehouse_session_grants (workspace, "group", session)
				WHERE "group" IS NOT NULL AND enabled = TRUE;
			`),
	}
}
