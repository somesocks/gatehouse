package migrations

func postgresMigrationV064GroupMemberRoles() VersionedMigration {
	return VersionedMigration{Index: 64, Description: "group_member_roles", Builder: staticMigrationBuilder(`
		ALTER TABLE gatehouse_group_members ADD COLUMN role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'manager'));
	`)}
}
