package auth

import (
	"context"
	"errors"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestBearerTokensRoundTripWithSelectedKeychain(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, migrations := database.BuildMigrations(config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, config.State{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, store, migrations); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_principals (id, enabled) VALUES ('alice', TRUE)`); err != nil {
		t.Fatal(err)
	}
	if err := store.IdentitiesUpsertRevisions(ctx, []model.Identity{{
		Id: "gatehouse:alice", Principal: model.PrincipalRef{Id: "alice"}, Revision: 1, Verifiers: []interface{}{"gh-ver:invalid"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEHOUSE_TEST_KEYCHAIN", "test passphrase")
	err, keyring := keychain.Prepare(ctx, store, []config.Keychain{{
		ID: "test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"},
	}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	err, tokens := Prepare(ctx, store, keyring, "test")
	if err != nil {
		t.Fatal(err)
	}
	err, token := tokens.Mint(ctx, Claims{Principal: "alice", Identity: "gatehouse:alice"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.KeychainsInsert(ctx, []model.Keychain{{
		Ref: model.KeychainRef{Id: "test", Version: 2}, KekKdf: "unavailable", Key: "unavailable", Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	err, claims := tokens.Authenticate(ctx, "Bearer "+token)
	if err != nil {
		t.Fatal(err)
	}
	if claims != (Claims{Principal: "alice", Identity: "gatehouse:alice"}) {
		t.Fatalf("Authenticate() = %#v", claims)
	}
	err, unavailableKeyring := keychain.Prepare(ctx, store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer unavailableKeyring.Close()
	err, unavailableTokens := Prepare(ctx, store, unavailableKeyring, "test")
	if err != nil {
		t.Fatal(err)
	}
	err, _ = unavailableTokens.Authenticate(ctx, "Bearer "+token)
	if err == nil || errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() with unavailable keychain error = %v, want infrastructure error", err)
	}
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_keychains SET enabled = FALSE WHERE id = 'test' AND version = 1`); err != nil {
		t.Fatal(err)
	}
	err, _ = tokens.Authenticate(ctx, "Bearer "+token)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() after disabling version 1 error = %v, want unauthenticated", err)
	}
}

func TestPrepareDoesNotRequireSelectedKeychain(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, keyring := keychain.Prepare(ctx, store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	if err, _ := Prepare(ctx, store, keyring, "missing"); err != nil {
		t.Fatalf("Prepare() error = %v, want lazy keychain resolution", err)
	}
}
