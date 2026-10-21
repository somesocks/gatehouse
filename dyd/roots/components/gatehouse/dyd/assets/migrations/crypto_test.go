package migrations

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/identity"
	"gatehouse/kdf"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestCryptoDefaultChangesPreserveStoredRecords(t *testing.T) {
	for _, initial := range []string{kdf.Default, kdf.InsecureV0} {
		t.Run(initial, func(t *testing.T) {
			ctx := context.Background()
			t.Setenv("CRYPTO_TEST_KEYCHAIN", "test passphrase")
			t.Setenv("CRYPTO_TEST_PASSWORD", "test password")
			configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "crypto.sqlite")}
			configured := []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:CRYPTO_TEST_KEYCHAIN"}}}
			state := config.State{Keychains: configured, Principals: []config.Principal{{
				Alias: "alice", Enabled: true, Identities: []config.Identity{{
					Alias: "alice-login", Key: "gatehouse:alice", Enabled: true, Revision: 1,
					PasswordSources: []config.PasswordSource{"env:CRYPTO_TEST_PASSWORD"},
				}},
			}}}
			err, store := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: initial})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { store.Close() })
			err, ring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ring.Close)
			run := func(store *database.Store, ring *keychain.Keyring) {
				t.Helper()
				err, set := Build(configuration, state, ring)
				if err != nil {
					t.Fatal(err)
				}
				if err := Run(ctx, store, set); err != nil {
					t.Fatal(err)
				}
			}
			readVerifier := func(store *database.Store, key, password, algorithm string) *database.ActiveIdentity {
				t.Helper()
				err, active := store.ActiveIdentityGetByKey(ctx, key)
				if err != nil || active == nil || len(active.Verifiers) != 1 {
					t.Fatalf("read identity %q: %#v, %v", key, active, err)
				}
				value, ok := storedPasswordVerifier(active.Verifiers[0])
				if !ok || !strings.HasSuffix(value, "?alg="+algorithm) {
					t.Fatalf("identity %q verifier = %q, want %s", key, value, algorithm)
				}
				if err, valid := identity.VerifyPassword(value, []byte(password)); err != nil || !valid {
					t.Fatalf("verify %q: %v, %t", key, err, valid)
				}
				return active
			}
			run(store, ring)
			original := readVerifier(store, "gatehouse:alice", "test password", initial)
			originalVerifier, _ := storedPasswordVerifier(original.Verifiers[0])
			reference := model.KeychainRef{Id: config.SystemKeychainID, Version: 1}
			err, originals := store.KeychainsGet(ctx, []model.KeychainRef{reference})
			if err != nil || len(originals) != 1 || !strings.HasSuffix(originals[0].KekKdf, "?alg="+initial) {
				t.Fatalf("initial keychain = %#v, %v", originals, err)
			}
			err, originalKeys := ring.Get(ctx, []model.KeychainRef{reference})
			if err != nil {
				t.Fatal(err)
			}
			ring.Close()
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}

			next := kdf.InsecureV0
			if initial == next {
				next = kdf.Default
			}
			err, reopened := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: next})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { reopened.Close() })
			err, nextRing := keychain.NewKeyring(reopened, configured, keychain.NewPassphraseSourceResolver())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(nextRing.Close)
			// Unchanged identities must not even resolve their password source.
			t.Setenv("CRYPTO_TEST_PASSWORD", "")
			run(reopened, nextRing)
			preserved := readVerifier(reopened, "gatehouse:alice", "test password", initial)
			preservedVerifier, _ := storedPasswordVerifier(preserved.Verifiers[0])
			if preservedVerifier != originalVerifier {
				t.Fatal("changing the default rewrote a password verifier")
			}
			err, preservedChains := reopened.KeychainsGet(ctx, []model.KeychainRef{reference})
			if err != nil || len(preservedChains) != 1 || preservedChains[0] != originals[0] {
				t.Fatalf("changing the default rewrote a keychain: %#v, %v", preservedChains, err)
			}
			err, keys := nextRing.Get(ctx, []model.KeychainRef{reference})
			if err != nil || !bytes.Equal(keys[reference], originalKeys[reference]) {
				t.Fatalf("stored-algorithm decryption failed: %v", err)
			}
			err, tokens := auth.Prepare(ctx, reopened, nextRing)
			if err != nil {
				t.Fatal(err)
			}
			if err, _ := tokens.Login(ctx, "gatehouse:alice", []byte("test password")); err != nil {
				t.Fatalf("login across default change: %v", err)
			}

			// New source-based identities and keychains use the new default.
			t.Setenv("CRYPTO_TEST_PASSWORD", "new password")
			state.Principals[0].Identities = append(state.Principals[0].Identities, config.Identity{
				Alias: "new-login", Key: "gatehouse:new", Enabled: true, Revision: 1,
				PasswordSources: []config.PasswordSource{"env:CRYPTO_TEST_PASSWORD"},
			})
			configured = append(configured, config.Keychain{ID: "new", Sources: []config.KeychainPassphraseSource{"env:CRYPTO_TEST_KEYCHAIN"}})
			state.Keychains = configured
			err, newRing := keychain.NewKeyring(reopened, configured, keychain.NewPassphraseSourceResolver())
			if err != nil {
				t.Fatal(err)
			}
			defer newRing.Close()
			run(reopened, newRing)
			readVerifier(reopened, "gatehouse:new", "new password", next)
			err, newChains := reopened.KeychainsGet(ctx, []model.KeychainRef{{Id: "new", Version: 1}})
			if err != nil || len(newChains) != 1 || !strings.HasSuffix(newChains[0].KekKdf, "?alg="+next) {
				t.Fatalf("new keychain = %#v, %v", newChains, err)
			}

			// Administrative creation, update and capability reset share the policy.
			username, password := "admin-created", "admin password"
			if err, _ := reopened.SystemPrincipalIdentityCreate(ctx, original.Principal.Ref.Id, database.SystemPrincipalIdentityInput{Username: &username, Password: &password}); err != nil {
				t.Fatal(err)
			}
			created := readVerifier(reopened, "gatehouse:"+username, password, next)
			err, login := reopened.PasswordSetIdentityGet(ctx, original.Principal.Ref.Id, created.ID)
			if err != nil || login == nil {
				t.Fatalf("get new login: %#v, %v", login, err)
			}
			password = "updated password"
			if err, _ := reopened.SystemPrincipalIdentityUpdate(ctx, original.Principal.Ref.Id, created.ID, login.Revision, database.SystemPrincipalIdentityInput{Password: &password}); err != nil {
				t.Fatal(err)
			}
			readVerifier(reopened, "gatehouse:"+username, password, next)
			err, login = reopened.PasswordSetIdentityGet(ctx, original.Principal.Ref.Id, created.ID)
			if err != nil || login == nil {
				t.Fatalf("get updated login: %#v, %v", login, err)
			}
			password = "reset password"
			if err := reopened.PasswordSet(ctx, original.Principal.Ref.Id, created.ID, login.Revision, time.Now().Add(time.Minute), []byte(password)); err != nil {
				t.Fatal(err)
			}
			readVerifier(reopened, "gatehouse:"+username, password, next)
		})
	}
}
