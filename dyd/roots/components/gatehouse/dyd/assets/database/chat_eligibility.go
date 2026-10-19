package database

// chatInvitationEligibilitySQL is intentionally independent of chat grants:
// sharing a conversation never establishes eligibility for a new invitation.
// Its arguments are trusted SQL expressions, not request values.
func chatInvitationEligibilitySQL(kind, workspace, actor, subject string) (string, error) {
	var eligible string
	switch kind {
	case "principal":
		eligible = sessionWorkspacePrincipalEligibleSQL(workspace, subject) + `
			AND EXISTS (SELECT 1 FROM gatehouse_principals target WHERE target.id = ` + subject + ` AND target.enabled = TRUE)
			AND (` + actor + ` = ` + subject + ` OR EXISTS (
				SELECT 1 FROM gatehouse_groups shared_group
				JOIN gatehouse_group_members author_member ON author_member.workspace_id = shared_group.workspace_id AND author_member.group_id = shared_group.id
				JOIN gatehouse_group_members target_member ON target_member.workspace_id = shared_group.workspace_id AND target_member.group_id = shared_group.id
				WHERE shared_group.workspace_id = ` + workspace + ` AND shared_group.enabled = TRUE
				AND author_member.principal_id = ` + actor + ` AND author_member.enabled = TRUE
				AND target_member.principal_id = ` + subject + ` AND target_member.enabled = TRUE))`
	case "group":
		eligible = `EXISTS (SELECT 1 FROM gatehouse_groups invited_group
			JOIN gatehouse_group_members author_member ON author_member.workspace_id = invited_group.workspace_id AND author_member.group_id = invited_group.id
			WHERE invited_group.workspace_id = ` + workspace + ` AND invited_group.id = ` + subject + ` AND invited_group.enabled = TRUE
			AND author_member.principal_id = ` + actor + ` AND author_member.enabled = TRUE)`
	case "agent":
		eligible = `EXISTS (SELECT 1 FROM gatehouse_workspace_agents invited_bot
			JOIN gatehouse_agent_models invited_model ON invited_model.id = invited_bot.model_id
			JOIN gatehouse_agent_providers invited_provider ON invited_provider.id = invited_model.provider_id
			WHERE invited_bot.workspace_id = ` + workspace + ` AND invited_bot.id = ` + subject + `
			AND invited_bot.enabled = TRUE AND invited_model.enabled = TRUE AND invited_provider.enabled = TRUE
			AND (invited_bot."default" = TRUE OR EXISTS (
				SELECT 1 FROM gatehouse_groups shared_group
				JOIN gatehouse_group_members author_member ON author_member.workspace_id = shared_group.workspace_id AND author_member.group_id = shared_group.id
				JOIN gatehouse_group_agents bot_member ON bot_member.workspace_id = shared_group.workspace_id AND bot_member.group_id = shared_group.id
				WHERE shared_group.workspace_id = ` + workspace + ` AND shared_group.enabled = TRUE
				AND author_member.principal_id = ` + actor + ` AND author_member.enabled = TRUE
				AND bot_member.agent_id = invited_bot.id AND bot_member.enabled = TRUE)))`
	default:
		return "", ErrMessageTarget
	}
	return `(EXISTS (SELECT 1 FROM gatehouse_principals inviter WHERE inviter.id = ` + actor + ` AND inviter.enabled = TRUE)
		AND EXISTS (SELECT 1 FROM gatehouse_workspaces invitation_workspace WHERE invitation_workspace.id = ` + workspace + ` AND invitation_workspace.enabled = TRUE)
		AND (` + eligible + `))`, nil
}
