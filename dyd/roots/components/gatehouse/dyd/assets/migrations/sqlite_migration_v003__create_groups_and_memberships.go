package migrations

func sqliteMigrationV003CreateGroupsAndMemberships() VersionedMigration {
	return VersionedMigration{
		Index:       3,
		Description: "create_groups_and_memberships",
		Builder: staticMigrationBuilder(`
			CREATE TABLE gatehouse_groups (
				workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
				id TEXT NOT NULL CHECK (length(id) = 30 AND substr(id, 1, 4) = 'grp_' AND substr(id, 5, 1) GLOB '[0-7]' AND substr(id, 5) NOT GLOB '*[^0-9a-hjkmnp-tv-z]*'),
				alias TEXT
					CHECK (alias IS NULL OR alias GLOB '[a-z]*')
					CHECK (alias IS NULL OR alias NOT GLOB '*[^a-z0-9_-]*'),
				name TEXT CHECK (name IS NULL OR length(trim(name)) > 0),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, alias)
			) STRICT;

			CREATE TABLE gatehouse_group_members (
				workspace_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				principal_id TEXT NOT NULL REFERENCES gatehouse_principals (id),
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				PRIMARY KEY (workspace_id, group_id, principal_id),
				FOREIGN KEY (workspace_id, group_id)
					REFERENCES gatehouse_groups (workspace_id, id)
			) STRICT;

			CREATE INDEX gatehouse_group_members_by_principal
			ON gatehouse_group_members (principal_id);
		`),
	}
}
