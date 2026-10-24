package migrations

func postgresMigrationV069RenameRoles() VersionedMigration {
	return VersionedMigration{Index: 69, Description: "rename_roles", Builder: staticMigrationBuilder(`
		LOCK TABLE gatehouse_workspace_grants, gatehouse_project_grants, gatehouse_session_grants,
			gatehouse_group_members, gatehouse_system_grants IN ACCESS EXCLUSIVE MODE;
		ALTER TABLE gatehouse_workspace_grants DROP CONSTRAINT gatehouse_workspace_grants_role_check;
		ALTER TABLE gatehouse_project_grants DROP CONSTRAINT gatehouse_project_grants_role_check;
		ALTER TABLE gatehouse_session_grants DROP CONSTRAINT gatehouse_session_grants_role_check;
		ALTER TABLE gatehouse_group_members DROP CONSTRAINT gatehouse_group_members_role_check;
		ALTER TABLE gatehouse_system_grants DROP CONSTRAINT gatehouse_system_grants_role_check;

		-- Translate retired group grants as well, then restore their immutability.
		ALTER TABLE gatehouse_workspace_grants DISABLE TRIGGER gatehouse_workspace_grants_principal_only;
		-- Free the old member key before moving contributor into it. Unique role
		-- indexes include disabled grants, and one recipient can have every role.
		UPDATE gatehouse_workspace_grants SET role = 'guest' WHERE role = 'member';
		UPDATE gatehouse_workspace_grants SET role = 'member' WHERE role = 'contributor';
		UPDATE gatehouse_workspace_grants SET role = 'admin' WHERE role = 'manager';
		ALTER TABLE gatehouse_workspace_grants ENABLE TRIGGER gatehouse_workspace_grants_principal_only;
		UPDATE gatehouse_project_grants SET role = 'guest' WHERE role = 'member';
		UPDATE gatehouse_project_grants SET role = 'member' WHERE role = 'contributor';
		UPDATE gatehouse_project_grants SET role = 'admin' WHERE role = 'manager';
		UPDATE gatehouse_session_grants SET role = 'guest' WHERE role = 'member';
		UPDATE gatehouse_session_grants SET role = 'member' WHERE role = 'contributor';
		UPDATE gatehouse_session_grants SET role = 'admin' WHERE role = 'manager';
		UPDATE gatehouse_group_members SET role = 'admin' WHERE role = 'manager';
		UPDATE gatehouse_system_grants SET role = 'admin' WHERE role = 'manager';

		ALTER TABLE gatehouse_workspace_grants ADD CONSTRAINT gatehouse_workspace_grants_role_check CHECK (role IN ('guest', 'member', 'admin'));
		ALTER TABLE gatehouse_project_grants ADD CONSTRAINT gatehouse_project_grants_role_check CHECK (role IN ('guest', 'member', 'admin'));
		ALTER TABLE gatehouse_session_grants ADD CONSTRAINT gatehouse_session_grants_role_check CHECK (role IN ('guest', 'member', 'admin'));
		ALTER TABLE gatehouse_group_members ADD CONSTRAINT gatehouse_group_members_role_check CHECK (role IN ('member', 'admin'));
		ALTER TABLE gatehouse_system_grants ADD CONSTRAINT gatehouse_system_grants_role_check CHECK (role = 'admin');
	`)}
}
