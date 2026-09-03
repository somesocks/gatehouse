package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func postgresMigrations(state config.State, keyring *keychain.Keyring) (error, Registry) {
	return nil, Registry{
		Init: postgresMigrationInit(),
		Versioned: []VersionedMigration{
			postgresMigrationV001CreateWorkspaces(),
			postgresMigrationV002CreatePrincipalsAndIdentities(),
			postgresMigrationV003CreateGroupsAndMemberships(),
			postgresMigrationV005CreateKeychains(),
			postgresMigrationV006AddIdentityRevisions(),
			postgresMigrationV007CreateAgentsAndProjects(),
			postgresMigrationV008CreateSessionsGrantsAndEvents(),
			postgresMigrationV009CreateSessionEventReplyTasks(),
			postgresMigrationV010AddWorkspaceAgentMaxTurns(),
			postgresMigrationV011AddWorkspaceAgentSystemPrompt(),
			postgresMigrationV012AddWorkspaceAgentLabel(),
			postgresMigrationV013CreateStorageProviders(),
			postgresMigrationV014CreateStorageObjectsAndFiles(),
			postgresMigrationV015CreateActivityEvents(),
			postgresMigrationV016CreateSessionNotes(),
			postgresMigrationV017CreateSessionApprovalDecisions(),
			postgresMigrationV018AddSessionNoteSensitivity(),
			postgresMigrationV019AddProjectNoteSensitivity(),
			postgresMigrationV020AddWorkspaceAgentProviderLimits(),
			postgresMigrationV021AddAgentModelCompaction(),
			postgresMigrationV022AddAgentModelExecutionLimits(),
			postgresMigrationV023AddAgentContexts(),
			postgresMigrationV024IncreaseDefaultAgentModelContextWindow(),
			postgresMigrationV025AddSessionEventMetrics(),
			postgresMigrationV026IndexLatestAgentContexts(),
			postgresMigrationV027DropLegacyWorkspaceToolsAndResources(),
			postgresMigrationV028CreateSessionSecrets(),
			postgresMigrationV029CreateProjectSecrets(),
			postgresMigrationV030CreateWorkspaceGrants(),
			postgresMigrationV031MergeResourceGrants(),
			postgresMigrationV032AddNoteRevisions(),
			postgresMigrationV033AddWorkspaceActivitySubjects(),
		},
		Repeatable: []RepeatableMigration{
			postgresMigrationR001PrepareKeychains(keyring),
			postgresMigrationR002SeedGatehouseWorkspace(),
			postgresMigrationR003ReconcileWorkspaces(state),
			postgresMigrationR004ReconcilePrincipals(state),
			postgresMigrationR005ReconcileIdentities(state),
			postgresMigrationR006ReconcileGroupsAndMemberships(state),
			postgresMigrationR007ReconcileWorkspaceGrants(state),
			postgresMigrationR009ReconcileAgentProviders(state, keyring),
			postgresMigrationR010ReconcileAgentModels(state),
			postgresMigrationR011ReconcileWorkspaceAgents(state),
			postgresMigrationR012ReconcileStorageProviders(state, keyring),
			postgresMigrationR013ReconcileWorkspaceStorageProviders(state),
		},
	}
}
