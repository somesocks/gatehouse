package migrations

func postgresMigrationV003CreateGroupsAndMemberships() VersionedMigration {
	return VersionedMigration{
		Index:       3,
		Description: "create_groups_and_memberships",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_groups (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (id ~ '^grp_[0-7][0-9a-hjkmnp-tv-z]{25}$'),
				alias TEXT CHECK (alias IS NULL OR alias ~ '^[a-z][a-z0-9_-]*$'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, alias)
			);

			CREATE TABLE gatehouse_group_members (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled BOOLEAN NOT NULL,
				PRIMARY KEY (workspace_id, group_id, principal_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id)
			);

			CREATE INDEX gatehouse_group_members_by_principal
			ON gatehouse_group_members (principal_id);
		`),
	}
}
