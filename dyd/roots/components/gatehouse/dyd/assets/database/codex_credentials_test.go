package database_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestCodexReconciliationPreservesLiveCredentialAndEncryptionReference(t *testing.T) {
	verifyCodexCredentialReconciliation(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestCodexConcurrentCredentialInstallHasOneWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "codex.sqlite")}
	err, first := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	t.Setenv("CODEX_CONCURRENT_KEYCHAIN", "passphrase")
	id, endpoint := config.SystemKeychainID, config.OpenAICodexEndpoint
	state := config.State{Keychains: []config.Keychain{{ID: id, Sources: []config.KeychainPassphraseSource{"env:CODEX_CONCURRENT_KEYCHAIN"}}}, AgentProviders: []config.AgentProvider{{Alias: "chatgpt", Revision: 1, Protocol: config.AgentProviderProtocolOpenAICodex, Endpoint: &endpoint, Enabled: true}}}
	if err := migrateState(ctx, first, configuration, state); err != nil {
		t.Fatal(err)
	}
	var providerID string
	if err := first.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_providers WHERE alias='chatgpt'`).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	err, second := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, store := range []*database.Store{first, second} {
		go func() {
			<-start
			accountID := []string{"a", "b"}[index]
			ciphertext := "gh-enc:" + accountID
			err, _ := store.SystemAgentProviderCodexCredentialSet(ctx, providerID, 1, &model.KeychainRef{Id: id, Version: 1}, &ciphertext, &database.SystemAgentProviderAccount{ID: accountID, ExpiresAt: time.Now().Add(time.Hour)})
			results <- err
		}()
	}
	close(start)
	winners, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			winners++
		} else if errors.Is(err, database.ErrRevisionConflict) {
			conflicts++
		} else {
			t.Fatalf("concurrent install: %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winners, conflicts)
	}
	err, record := first.AgentProviderCredentialGet(ctx, providerID)
	if err != nil || record.Provider.Revision != 2 || record.Provider.Account == nil || *record.Encrypted != "gh-enc:"+record.Provider.Account.ID {
		t.Fatal("credential/account snapshot did not commit atomically")
	}
}

func TestPostgresCodexCredentialReconciliation(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	verifyCodexCredentialReconciliation(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func verifyCodexCredentialReconciliation(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("CODEX_KEYCHAIN_TEST", "passphrase")
	id, endpoint := config.SystemKeychainID, config.OpenAICodexEndpoint
	state := config.State{
		Keychains:      []config.Keychain{{ID: id, Sources: []config.KeychainPassphraseSource{"env:CODEX_KEYCHAIN_TEST"}}},
		AgentProviders: []config.AgentProvider{{Alias: "chatgpt", Revision: 1, Protocol: config.AgentProviderProtocolOpenAICodex, Endpoint: &endpoint, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	var providerID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_providers WHERE alias='chatgpt'`).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	// Simulate a credential written under a newer key version. Reconciliation
	// must preserve the reference paired with its ciphertext rather than reset 1.
	placeholder := "?"
	if configuration.Kind == config.DatabaseKindPostgres {
		placeholder = "$1"
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_keychains (id,version,kek_kdf,key,enabled) SELECT id,2,kek_kdf,key,enabled FROM gatehouse_keychains WHERE id=`+placeholder+` AND version=1`, id); err != nil {
		t.Fatal(err)
	}
	ciphertext := "gh-enc:opaque-codex-ciphertext"
	account := &database.SystemAgentProviderAccount{ID: "account", Email: "user@example.com", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	err, connected := store.SystemAgentProviderCodexCredentialSet(ctx, providerID, 1, &model.KeychainRef{Id: id, Version: 2}, &ciphertext, account)
	if err != nil || connected.Revision != 2 {
		t.Fatalf("connect = %#v %v", connected, err)
	}
	err, edited := store.SystemAgentProviderUpdate(ctx, providerID, 2, database.SystemAgentProviderInput{Alias: "chatgpt", Protocol: config.AgentProviderProtocolOpenAICodex, Keychain: connected.Keychain, Enabled: true})
	if err != nil || edited.Revision != 3 || edited.Account == nil {
		t.Fatalf("metadata edit = %#v %v", edited, err)
	}
	state.AgentProviders[0].Revision = 4
	state.AgentProviders[0].Enabled = false
	for iteration := 0; iteration < 2; iteration++ {
		if err := migrateState(ctx, store, configuration, state); err != nil {
			t.Fatal(err)
		}
		err, stored := store.AgentProviderCredentialGet(ctx, providerID)
		if err != nil || stored.Encrypted == nil || *stored.Encrypted != ciphertext || stored.Provider.Keychain.Version != 2 || stored.Provider.Revision != 4 || stored.Provider.Enabled || stored.Provider.Account == nil || stored.Provider.Account.Email != account.Email {
			t.Fatalf("reconciled = %#v %v", stored, err)
		}
	}
	// Adding another keychain cannot change the system provider's reference.
	other := "other"
	state.Keychains = append(state.Keychains, config.Keychain{ID: other, Sources: []config.KeychainPassphraseSource{"env:CODEX_KEYCHAIN_TEST"}})
	state.AgentProviders[0].Revision = 5
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	err, stored := store.AgentProviderCredentialGet(ctx, providerID)
	if err != nil || stored.Provider.Revision != 5 || stored.Provider.Keychain.Id != id || *stored.Encrypted != ciphertext {
		t.Fatal("reconciliation corrupted credential")
	}
	// Disconnect removes both the secret and derived public account metadata.
	err, disconnected := store.SystemAgentProviderCodexCredentialSet(ctx, providerID, 5, nil, nil, nil)
	if err != nil || disconnected.CredentialConfigured || disconnected.Account != nil || disconnected.Keychain.Version != 2 {
		t.Fatalf("disconnect = %#v %v", disconnected, err)
	}
}
