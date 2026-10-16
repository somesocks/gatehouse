package providermaintenance

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestRefreshRequiresUsableDestinationKeyBeforeExchange(t *testing.T) {
	for _, unavailable := range []string{"not_configured", "retired", "unreadable_current"} {
		t.Run(unavailable, func(t *testing.T) {
			var calls atomic.Int32
			f := newFixture(t, refreshFunc(func(_ context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
				calls.Add(1)
				old.RefreshToken = "UNEXPECTED_REFRESH_SENTINEL"
				return old, nil
			}))
			provider := f.add(t, "unavailable", time.Time{})
			before := f.record(t, provider.ID)
			switch unavailable {
			case "not_configured":
				err, ring := keychain.NewKeyring(f.store, []config.Keychain{{ID: "maintenance", Sources: []config.KeychainPassphraseSource{"env:MAINTENANCE_TEST_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(ring.Close)
				f.runtime.ring = ring
			case "retired":
				if _, err := f.store.Exec(`UPDATE gatehouse_keychains SET enabled = FALSE WHERE id = 'default'`); err != nil {
					t.Fatal(err)
				}
			case "unreadable_current":
				if err := f.store.KeychainsInsert(context.Background(), []model.Keychain{{Ref: model.KeychainRef{Id: config.SystemKeychainID, Version: 2}, KekKdf: "unreadable", Key: "unreadable", Enabled: true}}); err != nil {
					t.Fatal(err)
				}
			}
			// The old credential remains readable; only the destination is broken.
			plain, err := f.runtime.open(context.Background(), before)
			if err != nil {
				t.Fatal(err)
			}
			clear(plain)
			out := f.check(t, provider, unavailable)
			if calls.Load() != 0 || out.State != "unavailable" || out.Reason != "encryption_key_unavailable" {
				t.Fatalf("exchange was not blocked: %#v, calls=%d", out, calls.Load())
			}
			if after := f.record(t, provider.ID); after.Fingerprint() != before.Fingerprint() || after.Provider.Revision != before.Provider.Revision {
				t.Fatal("unavailable destination changed the credential")
			}
			assertNoSecrets(t, f.store, "ACCESS_unavailable_SENTINEL", "REFRESH_unavailable_SENTINEL", "UNEXPECTED_REFRESH_SENTINEL")
		})
	}
}

func (f *fixture) systemKeyVersion(t *testing.T, version int, enabled bool) model.KeychainRef {
	t.Helper()
	kdf := keychain.KDF{Salt: make([]byte, 16)}
	if _, err := rand.Read(kdf.Salt); err != nil {
		t.Fatal(err)
	}
	err, kek := keychain.DeriveKEK([]byte("test passphrase"), kdf)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(kek)
	dek := make([]byte, 16)
	defer clear(dek)
	if _, err := rand.Read(dek); err != nil {
		t.Fatal(err)
	}
	reference := model.KeychainRef{Id: config.SystemKeychainID, Version: version}
	err, wrapped := keychain.Seal(rand.Reader, kek, []byte(fmt.Sprintf("gh=v1|key=%s|ver=%d", reference.Id, reference.Version)), dek)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.KeychainsInsert(context.Background(), []model.Keychain{{Ref: reference, KekKdf: kdf.String(), Key: wrapped.String(), Enabled: enabled}}); err != nil {
		t.Fatal(err)
	}
	return reference
}

func TestRefreshPinsDestinationAcrossRotationAndRetirement(t *testing.T) {
	for _, change := range []string{"rotate", "retire", "rotate_and_retire"} {
		t.Run(change, func(t *testing.T) {
			verifyRefreshPinsDestination(t, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "rotation.sqlite")}, change)
		})
	}
}

func TestPostgresRefreshPinsDestinationAcrossRotationAndRetirement(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_KEY_ROTATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	verifyRefreshPinsDestination(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_KEY_ROTATION_POSTGRES_URL"}, "rotate_and_retire")
}

func verifyRefreshPinsDestination(t *testing.T, configuration config.DatabaseConfig, change string) {
	t.Helper()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f := newConfiguredFixture(t, configuration, refreshFunc(func(ctx context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return codexlogin.Credential{}, ctx.Err()
		}
		old.RefreshToken = "PINNED_REFRESH_SENTINEL"
		return old, nil
	}))
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	provider := f.add(t, "pinned", time.Time{})
	reference := f.systemKeyVersion(t, 2, true)
	handle, err := f.runtime.CheckNow(provider.ID, "rotation")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("refresh did not start")
	}
	if change != "retire" {
		f.systemKeyVersion(t, 3, true)
	}
	if change != "rotate" {
		if _, err := f.store.Exec(`UPDATE gatehouse_keychains SET enabled = FALSE WHERE id = 'default' AND version <= 2`); err != nil {
			t.Fatal(err)
		}
	}
	unblock()
	out := await(t, handle)
	if out.State != "refreshed" || calls.Load() != 1 {
		t.Fatalf("refresh lost the exchanged bundle: %#v, calls=%d", out, calls.Load())
	}
	after := f.record(t, provider.ID)
	if after.Provider.Keychain == nil || *after.Provider.Keychain != reference || after.Provider.Revision != provider.Revision {
		t.Fatalf("refresh did not retain the selected reference: %#v", after.Provider)
	}
	// Use a fresh keyring to verify the persisted reference/ciphertext pairing,
	// including historical reads after the selected version has been retired.
	err, ring := keychain.NewKeyring(f.store, []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:MAINTENANCE_TEST_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	reader := &Runtime{ring: ring}
	plain, err := reader.open(context.Background(), after)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	var saved codexlogin.Credential
	if err := json.Unmarshal(plain, &saved); err != nil || saved.RefreshToken != "PINNED_REFRESH_SENTINEL" {
		t.Fatalf("saved bundle does not decrypt with its recorded version: %v", err)
	}
	if configuration.Kind != config.DatabaseKindPostgres {
		assertNoSecrets(t, f.store, "PINNED_REFRESH_SENTINEL")
	}
}

func TestRefreshRetriesPinnedBundleAfterKeyRetirement(t *testing.T) {
	var calls atomic.Int32
	f := newFixture(t, refreshFunc(func(_ context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		old.RefreshToken = "RETRIED_PINNED_REFRESH_SENTINEL"
		return old, nil
	}))
	provider := f.add(t, "retired_retry", time.Time{})
	reference := f.systemKeyVersion(t, 2, true)
	f.systemKeyVersion(t, 3, false)
	store := &flakyStore{Store: f.store, beforeRetry: func(selected model.KeychainRef) error {
		if selected != reference {
			return fmt.Errorf("unexpected destination reference: %#v", selected)
		}
		_, err := f.store.Exec(`UPDATE gatehouse_keychains SET enabled = (version = 3) WHERE id = 'default'`)
		return err
	}}
	f.runtime.store = store
	if out := f.check(t, provider, "retired_retry"); out.State != "refreshed" || calls.Load() != 1 {
		t.Fatalf("retry lost the exchanged bundle: %#v, calls=%d", out, calls.Load())
	}
	store.mu.Lock()
	attempts, savedReference := store.calls, store.reference
	store.mu.Unlock()
	if attempts != 2 || savedReference != reference {
		t.Fatalf("retry changed destination: attempts=%d ref=%#v", attempts, savedReference)
	}
	if saved := f.record(t, provider.ID); *saved.Provider.Keychain != reference || f.credential(t, provider.ID).RefreshToken != "RETRIED_PINNED_REFRESH_SENTINEL" {
		t.Fatal("retry corrupted the ciphertext/reference pair")
	}
	assertNoSecrets(t, f.store, "RETRIED_PINNED_REFRESH_SENTINEL")
}
