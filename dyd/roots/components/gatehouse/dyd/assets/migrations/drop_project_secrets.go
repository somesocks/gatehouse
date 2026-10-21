package migrations

import (
	"fmt"
	"strings"
)

// Keep this version's activity constraint independent of application models and
// later migrations so upgrades retain a stable schema and checksum.
func projectSecretsV065ActivityCheck(postgres bool) string {
	resources := [][]string{
		{"keychain", "resource_keychain_id", "resource_keychain_version"},
		{"group_member", "resource_group_member_group", "resource_group_member_principal"},
		{"group_agent", "resource_group_agent_group", "resource_group_agent_agent"},
		{"workspace_agent", "resource_workspace_agent_workspace", "resource_workspace_agent_id"},
		{"workspace_storage_provider", "resource_workspace_storage_provider_workspace", "resource_workspace_storage_provider_provider"},
	}
	for _, kind := range []string{"agent_provider", "agent_model", "group", "principal", "project", "project_file", "project_grant", "project_note", "project_task", "project_record_schema", "project_record_attribute", "project_record", "session", "session_event", "session_file", "session_grant", "session_note", "session_task", "session_secret", "storage_provider", "system_grant", "workspace", "workspace_grant"} {
		resources = append(resources, []string{kind, "resource_" + kind})
	}
	check, expected := "CASE resource_kind", "CASE resource_kind"
	var columns []string
	for _, resource := range resources {
		var required []string
		for _, column := range resource[1:] {
			columns = append(columns, column)
			required = append(required, column+" IS NOT NULL")
		}
		check += " WHEN '" + resource[0] + "' THEN " + strings.Join(required, " AND ")
		expected += fmt.Sprintf(" WHEN '%s' THEN %d", resource[0], len(required))
	}
	check += " ELSE FALSE END AND "
	if postgres {
		check += "num_nonnulls(" + strings.Join(columns, ", ") + ")"
	} else {
		counts := make([]string, len(columns))
		for index, column := range columns {
			counts[index] = "(" + column + " IS NOT NULL)"
		}
		check += "(" + strings.Join(counts, " + ") + ")"
	}
	return check + " = " + expected + " ELSE 0 END AND (detail_identity IS NULL OR resource_kind = 'principal')"
}
