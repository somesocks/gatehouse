package migrations

func postgresMigrationV033AddWorkspaceActivitySubjects() VersionedMigration {
	return VersionedMigration{
		Index:       33,
		Description: "add_workspace_activity_subjects",
		Builder: staticMigrationBuilder(`
				ALTER TABLE gatehouse_activity_events
					ADD COLUMN "group" TEXT,
					ADD COLUMN principal TEXT,
					ADD COLUMN workspace_agent TEXT,
					DROP CONSTRAINT gatehouse_activity_events_resource_kind_check,
					DROP CONSTRAINT gatehouse_activity_events_check,
					ADD CONSTRAINT gatehouse_activity_events_resource_kind_check
						CHECK (resource_kind IN ('project', 'session', 'session_event', 'group', 'group_member', 'workspace_agent', 'workspace', 'workspace_grant', 'workspace_storage_provider')),
					ADD CONSTRAINT gatehouse_activity_events_subject_check CHECK (
						(resource_kind = 'project' AND project IS NOT NULL AND session IS NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'session' AND session IS NOT NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'session_event' AND session IS NOT NULL AND session_event IS NOT NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'group' AND "group" IS NOT NULL AND project IS NULL AND session IS NULL AND session_event IS NULL AND principal IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'group_member' AND "group" IS NOT NULL AND principal IS NOT NULL AND project IS NULL AND session IS NULL AND session_event IS NULL AND workspace_agent IS NULL)
						OR (resource_kind = 'workspace_agent' AND workspace_agent IS NOT NULL AND project IS NULL AND session IS NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL)
						OR (resource_kind IN ('workspace', 'workspace_grant', 'workspace_storage_provider') AND project IS NULL AND session IS NULL AND session_event IS NULL AND "group" IS NULL AND principal IS NULL AND workspace_agent IS NULL)
					),
					ADD FOREIGN KEY (workspace, "group") REFERENCES gatehouse_groups (workspace_id, id),
					ADD FOREIGN KEY (principal) REFERENCES gatehouse_principals (id),
					ADD FOREIGN KEY (workspace, "group", principal) REFERENCES gatehouse_group_members (workspace_id, group_id, principal_id),
					ADD FOREIGN KEY (workspace, workspace_agent) REFERENCES gatehouse_workspace_agents (workspace_id, model_id);
			`),
	}
}
