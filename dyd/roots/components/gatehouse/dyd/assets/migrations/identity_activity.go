package migrations

import "strings"

// Version 57 keeps identity IDs as historical detail, owned by the principal.
// Keep these definitions local to this migration so later schema changes cannot
// change its checksum or the shape of an upgrade from version 56.
var identityActivityV057Targets = []string{
	"resource_keychain_id", "resource_keychain_version", "resource_agent_provider", "resource_agent_model", "resource_group", "resource_group_member_group", "resource_group_member_principal", "resource_principal", "resource_project", "resource_project_file", "resource_project_grant", "resource_project_note", "resource_project_task", "resource_project_secret", "resource_project_record_schema", "resource_project_record_attribute", "resource_project_record", "resource_session", "resource_session_event", "resource_session_file", "resource_session_grant", "resource_session_note", "resource_session_task", "resource_session_secret", "resource_storage_provider", "resource_system_grant", "resource_workspace", "resource_workspace_agent_workspace", "resource_workspace_agent_id", "resource_workspace_grant", "resource_workspace_storage_provider_workspace", "resource_workspace_storage_provider_provider",
}

func identityActivityV057Check(postgres bool) string {
	check := `CASE resource_kind
		WHEN 'keychain' THEN resource_keychain_id IS NOT NULL AND resource_keychain_version IS NOT NULL
		WHEN 'group_member' THEN resource_group_member_group IS NOT NULL AND resource_group_member_principal IS NOT NULL
		WHEN 'workspace_agent' THEN resource_workspace_agent_workspace IS NOT NULL AND resource_workspace_agent_id IS NOT NULL
		WHEN 'workspace_storage_provider' THEN resource_workspace_storage_provider_workspace IS NOT NULL AND resource_workspace_storage_provider_provider IS NOT NULL`
	for _, kind := range []string{"agent_provider", "agent_model", "group", "principal", "project", "project_file", "project_grant", "project_note", "project_task", "project_secret", "project_record_schema", "project_record_attribute", "project_record", "session", "session_event", "session_file", "session_grant", "session_note", "session_task", "session_secret", "storage_provider", "system_grant", "workspace", "workspace_grant"} {
		check += " WHEN '" + kind + "' THEN resource_" + kind + " IS NOT NULL"
	}
	check += " ELSE FALSE END AND "
	if postgres {
		check += "num_nonnulls(" + strings.Join(identityActivityV057Targets, ", ") + ")"
	} else {
		counts := make([]string, len(identityActivityV057Targets))
		for index, column := range identityActivityV057Targets {
			counts[index] = "(" + column + " IS NOT NULL)"
		}
		check += "(" + strings.Join(counts, " + ") + ")"
	}
	return check + " = CASE resource_kind WHEN 'keychain' THEN 2 WHEN 'group_member' THEN 2 WHEN 'workspace_agent' THEN 2 WHEN 'workspace_storage_provider' THEN 2 ELSE 1 END AND (detail_identity IS NULL OR resource_kind = 'principal')"
}
