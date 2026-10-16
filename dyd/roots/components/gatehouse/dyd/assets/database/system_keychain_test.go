package database_test

import (
	"context"
	"crypto/rand"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestConfiguredCredentialsUseCurrentSystemKey(t *testing.T) {
	verifyConfiguredCredentialsUseCurrentSystemKey(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresConfiguredCredentialsUseCurrentSystemKey(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_KEYCHAIN_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	verifyConfiguredCredentialsUseCurrentSystemKey(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_KEYCHAIN_POSTGRES_URL"})
}

func verifyConfiguredCredentialsUseCurrentSystemKey(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("CURRENT_SYSTEM_PASSPHRASE", "passphrase")
	t.Setenv("CURRENT_SYSTEM_CREDENTIAL", "secret credential")
	endpoint, region, bucket, access := "https://example.test", "region", "bucket", "access"
	state := config.State{
		Keychains:        []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:CURRENT_SYSTEM_PASSPHRASE"}}},
		AgentProviders:   []config.AgentProvider{{Alias: "api", Revision: 1, Protocol: "openai-responses", Endpoint: &endpoint, Sources: []config.AgentProviderAPIKeySource{"env:CURRENT_SYSTEM_CREDENTIAL"}, Enabled: true}},
		StorageProviders: []config.StorageProvider{{Alias: "s3", Revision: 1, Protocol: "s3", Endpoint: &endpoint, Region: &region, Bucket: &bucket, AccessKeyID: &access, SecretKeySources: []config.StorageProviderSecretKeySource{"env:CURRENT_SYSTEM_CREDENTIAL"}, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	// Install a valid second version. Both reconciliation paths must encrypt
	// with it, not merely label version-1 ciphertext as current.
	kdf := keychain.KDF{Salt: []byte("0123456789abcdef")}
	err, kek := keychain.DeriveKEK([]byte("passphrase"), kdf)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(kek)
	dek := []byte("fedcba9876543210")
	err, wrapped := keychain.Seal(rand.Reader, kek, []byte("gh=v1|key=default|ver=2"), dek)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.KeychainsInsert(ctx, []model.Keychain{{Ref: model.KeychainRef{Id: config.SystemKeychainID, Version: 2}, KekKdf: kdf.String(), Key: wrapped.String(), Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	state.AgentProviders[0].Revision = 2
	state.StorageProviders[0].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ table, column, alias, aad string }{
		{"gatehouse_agent_providers", "credential", "api", "gh=v1|agent-provider|alias=api"},
		{"gatehouse_storage_providers", "secret_access_key", "s3", "gh=v1|storage-provider|alias=s3"},
	} {
		var id, ciphertext string
		var version int
		if err := store.QueryRowContext(ctx, `SELECT keychain_id, keychain_version, `+test.column+` FROM `+test.table+` WHERE alias = '`+test.alias+`'`).Scan(&id, &version, &ciphertext); err != nil {
			t.Fatal(err)
		}
		if id != config.SystemKeychainID || version != 2 {
			t.Fatalf("%s used %s/%d", test.table, id, version)
		}
		err, encrypted := keychain.ParseKey(ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		err, plain := keychain.Open(dek, []byte(test.aad), encrypted)
		if err != nil || string(plain) != "secret credential" {
			t.Fatalf("%s ciphertext does not match its reference: %v", test.table, err)
		}
		clear(plain)
	}
}
