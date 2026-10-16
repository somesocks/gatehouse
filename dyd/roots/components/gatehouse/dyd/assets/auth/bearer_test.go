package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/identity"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestBearerTokensRoundTripWithSystemKeychain(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	configured := []config.Keychain{{
		ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"},
	}}
	t.Setenv("GATEHOUSE_TEST_KEYCHAIN", "test passphrase")
	err, keyring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	err, set := migrations.Build(configuration, config.State{Keychains: configured}, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}
	principalID, err := typed_id.New(typed_id.Principal)
	if err != nil {
		t.Fatal(err)
	}
	verifierErr, passwordVerifier := identity.NewDummyPasswordVerifier()
	if verifierErr != nil {
		t.Fatal(verifierErr)
	}
	identityID, err := typed_id.New(typed_id.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_principals (id, revision, enabled) VALUES (?, 1, TRUE)`, principalID); err != nil {
		t.Fatal(err)
	}
	principal := model.Principal{Ref: model.PrincipalRef{Id: principalID}, Enabled: true}
	if err := store.IdentitiesUpsertRevisions(ctx, []model.Identity{{
		Id: identityID, Key: "gatehouse:alice", Principal: model.PrincipalRef{Id: principalID}, Revision: 1, Verifiers: []interface{}{map[string]any{"kind": "password", "password_verifier": passwordVerifier}}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	err, tokens := Prepare(ctx, store, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("PasswordSetExpirationBoundary", func(t *testing.T) {
		err, encoded, capability := tokens.MintPasswordSet(ctx, principalID, identityID, 1)
		if err != nil {
			t.Fatal(err)
		}
		expires := time.Unix(capability.ExpiresAt, 0)
		if err, _ := tokens.authenticatePasswordSetAt(ctx, "Bearer "+encoded, expires.Add(-time.Nanosecond)); err != nil {
			t.Fatalf("rejected before expiration: %v", err)
		}
		for _, now := range []time.Time{expires, expires.Add(time.Second)} {
			if err, _ := tokens.authenticatePasswordSetAt(ctx, "Bearer "+encoded, now); !errors.Is(err, ErrPasswordLinkExpired) {
				t.Fatalf("accepted expired capability: %v", err)
			}
		}
		for _, capability := range []PasswordSetCapability{
			{Purpose: "identity.set-password", Principal: principalID, Identity: identityID, IdentityRevision: 1},
			{Purpose: "identity.set-password", Principal: principalID, Identity: identityID, IdentityRevision: -1, ExpiresAt: expires.Unix()},
		} {
			err, encoded := tokens.seal(ctx, passwordSetAssociatedData, capability)
			if err != nil {
				t.Fatal(err)
			}
			if err, _ := tokens.authenticatePasswordSetAt(ctx, "Bearer "+encoded, expires.Add(-time.Second)); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("accepted malformed capability: %v", err)
			}
		}
	})
	if loginErr, _ := tokens.Login(ctx, "gatehouse:alice", []byte("gatehouse dummy password verifier")); loginErr != nil {
		t.Fatalf("Login() with typed password verifier error = %v", loginErr)
	}
	if loginErr, _ := tokens.Login(ctx, "gatehouse:alice", []byte("wrong password")); !errors.Is(loginErr, ErrInvalidCredentials) {
		t.Fatalf("Login() with wrong password error = %v, want invalid credentials", loginErr)
	}
	err, token := tokens.Mint(ctx, Claims{Principal: principal, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "key=") || !strings.Contains(token, "ver=1") {
		t.Fatalf("system token did not use implicit default: %s", token)
	}
	legacy := strings.Replace(token, "&ver=1", "&key=default&ver=1", 1)
	if err, _ := tokens.Authenticate(ctx, "Bearer "+legacy); err != nil {
		t.Fatalf("explicit default token rejected: %v", err)
	}
	foreign := strings.Replace(token, "&ver=1", "&key=other&ver=1", 1)
	if err, _ := tokens.Authenticate(ctx, "Bearer "+foreign); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("foreign keychain token accepted: %v", err)
	}
	if err, _ := tokens.Authenticate(ctx, "Bearer "+strings.Replace(token, "&ver=1", "", 1)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("versionless token accepted: %v", err)
	}
	if err := store.KeychainsInsert(ctx, []model.Keychain{{
		Ref: model.KeychainRef{Id: config.SystemKeychainID, Version: 2}, KekKdf: "unavailable", Key: "unavailable", Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	err, claims := tokens.Authenticate(ctx, "Bearer "+token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Principal.Ref != principal.Ref || claims.Identity != identityID {
		t.Fatalf("Authenticate() = %#v", claims)
	}
	err, unavailableKeyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer unavailableKeyring.Close()
	err, unavailableTokens := Prepare(ctx, store, unavailableKeyring)
	if err != nil {
		t.Fatal(err)
	}
	err, _ = unavailableTokens.Authenticate(ctx, "Bearer "+token)
	if err == nil || errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() with unavailable keychain error = %v, want infrastructure error", err)
	}
	if _, err := store.ExecContext(ctx, `UPDATE gatehouse_keychains SET enabled = FALSE WHERE id = 'default' AND version = 1`); err != nil {
		t.Fatal(err)
	}
	err, _ = tokens.Authenticate(ctx, "Bearer "+token)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() after disabling version 1 error = %v, want unauthenticated", err)
	}
}

func TestPrepareResolvesSystemKeychainLazily(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	if err, _ := Prepare(ctx, store, keyring); err != nil {
		t.Fatalf("Prepare() error = %v, want lazy keychain resolution", err)
	}
}
