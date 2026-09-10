package database_test

import (
	"context"
	"errors"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestSystemAdministrationResourcesAreRevisionedAndRedacted(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("ADMINISTRATION_KEYCHAIN", "passphrase")
	state := config.State{
		Keychains:  []config.Keychain{{ID: "default", Sources: []config.KeychainPassphraseSource{"env:ADMINISTRATION_KEYCHAIN"}}},
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	keychain := &model.KeychainRef{Id: "default", Version: 1}
	baseURL := "https://api.example.test/v1"
	agentInput := database.SystemAgentProviderInput{Alias: "openai", Protocol: "openai-responses", BaseURL: &baseURL, Keychain: keychain, EncryptedAPIKey: stringPointer("gh-enc:agent-key"), Enabled: true}
	err, provider := store.SystemAgentProviderCreate(ctx, agentInput)
	if err != nil || provider == nil || provider.Revision != 1 || !provider.CredentialConfigured {
		t.Fatalf("SystemAgentProviderCreate() = (%#v, %v)", provider, err)
	}
	if providersErr, providers := store.SystemAgentProvidersGet(ctx); providersErr != nil || len(providers) != 1 || providers[0].CredentialConfigured != true || providers[0].Keychain == nil || providers[0].Keychain.Id != keychain.Id || providers[0].Keychain.Version != keychain.Version {
		t.Fatalf("SystemAgentProvidersGet() = (%#v, %v)", providers, providersErr)
	}
	err, provider = store.SystemAgentProviderUpdate(ctx, provider.ID, 1, database.SystemAgentProviderInput{Alias: "openai", Protocol: "openai-responses", BaseURL: &baseURL, Keychain: keychain, Enabled: false})
	if err != nil || provider == nil || provider.Revision != 2 || provider.Enabled || !provider.CredentialConfigured {
		t.Fatalf("SystemAgentProviderUpdate() = (%#v, %v)", provider, err)
	}
	if err, _ := store.SystemAgentProviderUpdate(ctx, provider.ID, 1, database.SystemAgentProviderInput{Alias: "openai", Protocol: "openai-responses", BaseURL: &baseURL, Keychain: keychain, Enabled: true}); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale SystemAgentProviderUpdate() error = %v", err)
	}

	modelInput := database.SystemAgentModelInput{Alias: "assistant", ProviderID: provider.ID, Model: "gpt-test", Parameters: `{}`, Compaction: `{"algorithm":"mcmtr","history_bytes":96,"buffer_bytes":16}`, MaxTurns: 3, MaxOutputTokens: 1000, Enabled: true}
	err, agentModel := store.SystemAgentModelCreate(ctx, modelInput)
	if err != nil || agentModel == nil || agentModel.Revision != 1 {
		t.Fatalf("SystemAgentModelCreate() = (%#v, %v)", agentModel, err)
	}
	modelInput.Enabled = false
	err, agentModel = store.SystemAgentModelUpdate(ctx, agentModel.ID, 1, modelInput)
	if err != nil || agentModel == nil || agentModel.Revision != 2 || agentModel.Enabled {
		t.Fatalf("SystemAgentModelUpdate() = (%#v, %v)", agentModel, err)
	}
	if err, _ := store.SystemAgentModelUpdate(ctx, agentModel.ID, 1, modelInput); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale SystemAgentModelUpdate() error = %v", err)
	}

	err, binding := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "assistant", ModelID: agentModel.ID, Priority: 2, Enabled: true})
	if err != nil || binding == nil || binding.Revision != 1 || binding.Alias != "assistant" {
		t.Fatalf("SystemWorkspaceAgentCreate() = (%#v, %v)", binding, err)
	}
	if err, binding := store.SystemWorkspaceAgentCreate(ctx, workspace.Id, database.SystemWorkspaceAgentInput{Alias: "Invalid Alias", ModelID: agentModel.ID, Priority: 2, Enabled: true}); err != nil || binding != nil {
		t.Fatalf("SystemWorkspaceAgentCreate() accepted an invalid alias = (%#v, %v)", binding, err)
	}
	if err, _ := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, binding.ID, 1, database.SystemWorkspaceAgentInput{Alias: "renamed", ModelID: agentModel.ID, Priority: 3, Enabled: false}); err == nil {
		t.Fatal("SystemWorkspaceAgentUpdate() accepted an alias rename")
	}
	label := "Assistant"
	err, binding = store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, binding.ID, 1, database.SystemWorkspaceAgentInput{ModelID: agentModel.ID, Priority: 3, Label: &label, Enabled: false})
	if err != nil || binding == nil || binding.Revision != 2 || binding.Label == nil || *binding.Label != label || binding.Enabled {
		t.Fatalf("SystemWorkspaceAgentUpdate() = (%#v, %v)", binding, err)
	}
	if err, _ := store.SystemWorkspaceAgentUpdate(ctx, workspace.Id, binding.ID, 1, database.SystemWorkspaceAgentInput{ModelID: agentModel.ID, Priority: 1, Enabled: true}); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale SystemWorkspaceAgentUpdate() error = %v", err)
	}

	endpoint, region, bucket, accessKeyID := "https://s3.example.test", "us-east-1", "documents", "access-key"
	storageInput := database.SystemStorageProviderInput{Alias: "documents", Protocol: "s3", Endpoint: &endpoint, Region: &region, Bucket: &bucket, AccessKeyID: &accessKeyID, Keychain: keychain, EncryptedSecretAccessKey: stringPointer("gh-enc:storage-key"), Enabled: true}
	err, storageProvider := store.SystemStorageProviderCreate(ctx, storageInput)
	if err != nil || storageProvider == nil || storageProvider.Revision != 1 || !storageProvider.CredentialConfigured {
		t.Fatalf("SystemStorageProviderCreate() = (%#v, %v)", storageProvider, err)
	}
	storageInput.EncryptedSecretAccessKey = nil
	storageInput.Enabled = false
	err, storageProvider = store.SystemStorageProviderUpdate(ctx, storageProvider.ID, 1, storageInput)
	if err != nil || storageProvider == nil || storageProvider.Revision != 2 || storageProvider.Enabled || !storageProvider.CredentialConfigured {
		t.Fatalf("SystemStorageProviderUpdate() = (%#v, %v)", storageProvider, err)
	}
	if err, _ := store.SystemStorageProviderUpdate(ctx, storageProvider.ID, 1, storageInput); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale SystemStorageProviderUpdate() error = %v", err)
	}

	err, storageBinding := store.SystemWorkspaceStorageProviderCreate(ctx, workspace.Id, storageProvider.ID, database.SystemWorkspaceStorageProviderInput{Priority: 1, Enabled: true})
	if err != nil || storageBinding == nil || storageBinding.Revision != 1 {
		t.Fatalf("SystemWorkspaceStorageProviderCreate() = (%#v, %v)", storageBinding, err)
	}
	err, storageBinding = store.SystemWorkspaceStorageProviderUpdate(ctx, workspace.Id, storageProvider.ID, 1, database.SystemWorkspaceStorageProviderInput{Priority: 2, Enabled: false})
	if err != nil || storageBinding == nil || storageBinding.Revision != 2 || storageBinding.Enabled {
		t.Fatalf("SystemWorkspaceStorageProviderUpdate() = (%#v, %v)", storageBinding, err)
	}
	if err, _ := store.SystemWorkspaceStorageProviderUpdate(ctx, workspace.Id, storageProvider.ID, 1, database.SystemWorkspaceStorageProviderInput{Priority: 1, Enabled: true}); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale SystemWorkspaceStorageProviderUpdate() error = %v", err)
	}

	var activities int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE event IN ('agent_provider.create', 'agent_provider.update', 'agent_model.create', 'agent_model.update', 'workspace_agent.create', 'workspace_agent.update', 'storage_provider.create', 'storage_provider.update', 'workspace_storage_provider.create', 'workspace_storage_provider.update')`).Scan(&activities); err != nil || activities != 10 {
		t.Fatalf("administration activity count = (%d, %v), want 10", activities, err)
	}
}
