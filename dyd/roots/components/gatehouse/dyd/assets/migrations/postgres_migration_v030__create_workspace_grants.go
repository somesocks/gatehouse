package migrations

func postgresMigrationV030CreateWorkspaceGrants() VersionedMigration {
	return VersionedMigration{
		Index:       30,
		Description: "create_workspace_grants",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_workspace_grants (
				workspace TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				role TEXT NOT NULL CHECK (role IN ('member', 'contributor', 'manager')),
				principal TEXT REFERENCES gatehouse_principals (id),
				"group" TEXT,
				enabled BOOLEAN NOT NULL,
				revision BIGINT NOT NULL CHECK (revision > 0),
				CHECK (
					(principal IS NOT NULL AND "group" IS NULL)
					OR (principal IS NULL AND "group" IS NOT NULL)
				),
				FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id)
			);

			CREATE UNIQUE INDEX gatehouse_workspace_grants_principal_once
			ON gatehouse_workspace_grants (workspace, role, principal)
			WHERE principal IS NOT NULL;

			CREATE UNIQUE INDEX gatehouse_workspace_grants_group_once
			ON gatehouse_workspace_grants (workspace, role, "group")
			WHERE "group" IS NOT NULL;

			CREATE INDEX gatehouse_workspace_grants_principal_enabled
			ON gatehouse_workspace_grants (principal, workspace)
			WHERE principal IS NOT NULL AND enabled = TRUE;

			CREATE INDEX gatehouse_workspace_grants_group_enabled
			ON gatehouse_workspace_grants (workspace, "group")
			WHERE "group" IS NOT NULL AND enabled = TRUE;
		`),
	}
}
