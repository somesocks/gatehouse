package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func sqliteMigrations(state config.State, keyring *keychain.Keyring) (error, Registry) {
	return nil, Registry{
		Init: sqliteMigrationInit(),
		Versioned: []VersionedMigration{
			sqliteMigrationV001CreateWorkspaces(),
			sqliteMigrationV002CreatePrincipalsAndIdentities(),
			sqliteMigrationV003CreateGroupsAndMemberships(),
			sqliteMigrationV005CreateKeychains(),
			sqliteMigrationV006AddIdentityRevisions(),
			sqliteMigrationV007CreateAgentsAndProjects(),
			sqliteMigrationV008CreateSessionsGrantsAndEvents(),
			sqliteMigrationV009CreateSessionEventReplyTasks(),
			sqliteMigrationV010AddWorkspaceAgentMaxTurns(),
			sqliteMigrationV011AddWorkspaceAgentSystemPrompt(),
			sqliteMigrationV012AddWorkspaceAgentLabel(),
			sqliteMigrationV013CreateStorageProviders(),
			sqliteMigrationV014CreateStorageObjectsAndFiles(),
			sqliteMigrationV015CreateActivityEvents(),
			sqliteMigrationV016CreateSessionNotes(),
			sqliteMigrationV017CreateSessionApprovalDecisions(),
			sqliteMigrationV018AddSessionNoteSensitivity(),
			sqliteMigrationV019AddProjectNoteSensitivity(),
			sqliteMigrationV020AddWorkspaceAgentProviderLimits(),
			sqliteMigrationV021AddAgentModelCompaction(),
			sqliteMigrationV022AddAgentModelExecutionLimits(),
			sqliteMigrationV023AddAgentContexts(),
			sqliteMigrationV024IncreaseDefaultAgentModelContextWindow(),
			sqliteMigrationV025AddSessionEventMetrics(),
			sqliteMigrationV026IndexLatestAgentContexts(),
			sqliteMigrationV027DropLegacyWorkspaceToolsAndResources(),
			sqliteMigrationV028CreateSessionSecrets(),
			sqliteMigrationV029CreateProjectSecrets(),
			sqliteMigrationV030CreateWorkspaceGrants(),
			sqliteMigrationV031MergeResourceGrants(),
			sqliteMigrationV032AddNoteRevisions(),
			sqliteMigrationV033AddWorkspaceActivitySubjects(),
		},
		Repeatable: []RepeatableMigration{
			sqliteMigrationR001PrepareKeychains(keyring),
			sqliteMigrationR002SeedGatehouseWorkspace(),
			sqliteMigrationR003ReconcileWorkspaces(state),
			sqliteMigrationR004ReconcilePrincipals(state),
			sqliteMigrationR005ReconcileIdentities(state),
			sqliteMigrationR006ReconcileGroupsAndMemberships(state.Groups),
			sqliteMigrationR007ReconcileWorkspaceGrants(state),
			sqliteMigrationR009ReconcileAgentProviders(state, keyring),
			sqliteMigrationR010ReconcileAgentModels(state),
			sqliteMigrationR011ReconcileWorkspaceAgents(state.WorkspaceAgents),
			sqliteMigrationR012ReconcileStorageProviders(state, keyring),
			sqliteMigrationR013ReconcileWorkspaceStorageProviders(state),
		},
	}
}
