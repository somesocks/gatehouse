package database_test

import (
	"context"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestSystemAgentModelContextAlgorithms(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: "pbkdf2-hmac-sha256-v0"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("TADW_ADMIN_KEYCHAIN", "test passphrase")
	if err := migrateState(ctx, store, configuration, config.State{Keychains: []config.Keychain{{ID: "default", Sources: []config.KeychainPassphraseSource{"env:TADW_ADMIN_KEYCHAIN"}}}}); err != nil {
		t.Fatal(err)
	}
	endpoint := "https://example.invalid/responses"
	err, provider := store.SystemAgentProviderCreate(ctx, database.SystemAgentProviderInput{Alias: "test", Protocol: "openai-responses", Endpoint: &endpoint, Keychain: &model.KeychainRef{Id: "default", Version: 1}, EncryptedCredential: stringPointer("gh-enc:agent-key"), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	input := database.SystemAgentModelInput{Alias: "test", ProviderID: provider.ID, Model: "test-model", Parameters: `{}`, Compaction: `{"algorithm":"mcmtr","history_bytes":100,"buffer_bytes":50}`, MaxTurns: 3, MaxOutputTokens: 100, Enabled: true}
	err, record := store.SystemAgentModelCreate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Compaction = `{"algorithm":"tadw","context_max_bytes":8192,"history_max_bytes":4096,"history_min_bytes":1024}`
	err, record = store.SystemAgentModelUpdate(ctx, record.ID, record.Revision, input)
	if err != nil || record == nil || record.Compaction != input.Compaction {
		t.Fatal(record, err)
	}
	for _, invalid := range []string{`{"algorithm":"tadw","context_max_bytes":100,"history_max_bytes":101,"history_min_bytes":1}`, `{"algorithm":"mcmtr","history_bytes":100,"buffer_bytes":50,"history_min_bytes":1}`, `{"algorithm":"tadw","context_max_bytes":100,"history_max_bytes":90}`} {
		input.Compaction = invalid
		if err, _ := store.SystemAgentModelUpdate(ctx, record.ID, record.Revision, input); err == nil {
			t.Fatal("accepted invalid update", invalid)
		}
		input.Alias = "invalid"
		if err, _ := store.SystemAgentModelCreate(ctx, input); err == nil {
			t.Fatal("accepted invalid create", invalid)
		}
		input.Alias = "test"
	}
	err, got := store.SystemAgentModelGet(ctx, record.ID)
	if err != nil || got.Compaction != record.Compaction || got.Revision != record.Revision {
		t.Fatal("rejected updates changed model", got, err)
	}
}
