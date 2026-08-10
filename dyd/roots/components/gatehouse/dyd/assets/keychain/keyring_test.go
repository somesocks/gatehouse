package keychain

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestKeyringGeneratesAndLoadsKeychainsLazily(t *testing.T) {
	store := openKeyringTestStore(t)
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	reference := model.KeychainRef{Id: "default", Version: 1}
	keyring := newTestKeyring(t, store, configured, testRandom(1, 2, 3))
	defer keyring.Close()
	if len(keyring.keys) != 0 {
		t.Fatalf("initial DEK cache = %#v, want empty", keyring.keys)
	}
	insertCandidates(t, store, keyring)

	err, keys := keyring.Get(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys[reference], bytes.Repeat([]byte{2}, keySize); !bytes.Equal(got, want) {
		t.Fatalf("loaded DEK = %x, want %x", got, want)
	}

	err, stored := store.KeychainsGet(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || !stored[0].Enabled {
		t.Fatalf("stored keychains = %#v, want one enabled keychain", stored)
	}
	err, kdf := ParseKDF(stored[0].KekKdf)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kdf.Salt, bytes.Repeat([]byte{1}, saltSize); !bytes.Equal(got, want) {
		t.Fatalf("stored KDF salt = %x, want %x", got, want)
	}

	repeat := newTestKeyring(t, store, configured, testRandom(4, 5, 6))
	defer repeat.Close()
	insertCandidates(t, store, repeat)
	err, keys = repeat.Get(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys[reference], bytes.Repeat([]byte{2}, keySize); !bytes.Equal(got, want) {
		t.Fatalf("repeated loaded DEK = %x, want persisted %x", got, want)
	}
}

func TestKeyringRejectsWrongPassphrase(t *testing.T) {
	store := openKeyringTestStore(t)
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	reference := model.KeychainRef{Id: "default", Version: 1}
	first := newTestKeyring(t, store, configured, testRandom(1, 2, 3))
	insertCandidates(t, store, first)
	first.Close()

	wrong := newTestKeyringWithPassphrase(t, store, configured, "wrong", testRandom(4, 5, 6))
	defer wrong.Close()
	err, _ := wrong.Get(context.Background(), []model.KeychainRef{reference})
	if err == nil || !strings.Contains(err.Error(), "decrypt DEK") {
		t.Fatalf("Keyring.Get() error = %v, want decryption failure", err)
	}
}

func TestKeyringGetLoadsExactVersion(t *testing.T) {
	store := openKeyringTestStore(t)
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	keyring := newTestKeyring(t, store, configured, testRandom(1, 2, 3))
	defer keyring.Close()
	insertCandidates(t, store, keyring)

	reference := model.KeychainRef{Id: "default", Version: 2}
	passphrase := []byte("passphrase")
	kdf := KDF{Salt: bytes.Repeat([]byte{4}, saltSize)}
	err, kek := DeriveKEK(passphrase, kdf)
	if err != nil {
		t.Fatal(err)
	}
	dek := bytes.Repeat([]byte{5}, keySize)
	err, encrypted := Seal(bytes.NewReader(bytes.Repeat([]byte{6}, encryptionNonceSize)), kek, keychainAssociatedData(reference), dek)
	clear(kek)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.KeychainsInsert(context.Background(), []model.Keychain{{
		Ref:     reference,
		KekKdf:  kdf.String(),
		Key:     encrypted.String(),
		Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}

	err, keys := keyring.Get(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys[reference]; !bytes.Equal(got, dek) {
		t.Fatalf("Keyring.Get() = %x, want %x", got, dek)
	}
}

func TestKeychainAssociatedData(t *testing.T) {
	if got, want := string(keychainAssociatedData(model.KeychainRef{Id: "default", Version: 1})), "gh=v1|key=default|ver=1"; got != want {
		t.Fatalf("keychainAssociatedData() = %q, want %q", got, want)
	}
}

func newTestKeyring(t *testing.T, store *database.Store, configured []config.Keychain, random io.Reader) *Keyring {
	t.Helper()
	return newTestKeyringWithPassphrase(t, store, configured, "passphrase", random)
}

func newTestKeyringWithPassphrase(t *testing.T, store *database.Store, configured []config.Keychain, passphrase string, random io.Reader) *Keyring {
	t.Helper()
	err, keyring := newKeyring(store, configured, testResolver(map[string]string{"DEFAULT": passphrase}), random)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func insertCandidates(t *testing.T, store *database.Store, keyring *Keyring) {
	t.Helper()
	err, candidates := keyring.Candidates()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.KeychainsInsert(context.Background(), candidates); err != nil {
		t.Fatal(err)
	}
}

func openKeyringTestStore(t *testing.T) *database.Store {
	t.Helper()
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(context.Background(), `
		CREATE TABLE gatehouse_keychains (
			id TEXT NOT NULL,
			version INTEGER NOT NULL,
			kek_kdf TEXT NOT NULL,
			key TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			PRIMARY KEY (id, version)
		) STRICT
	`); err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testResolver(values map[string]string) *PassphraseSourceResolver {
	return newPassphraseSourceResolver(strings.NewReader(""), io.Discard, func(name string) string {
		return values[name]
	}, nil)
}

func testRandom(salt, dek, nonce byte) io.Reader {
	value := make([]byte, 0, saltSize+keySize+encryptionNonceSize)
	value = append(value, bytes.Repeat([]byte{salt}, saltSize)...)
	value = append(value, bytes.Repeat([]byte{dek}, keySize)...)
	value = append(value, bytes.Repeat([]byte{nonce}, encryptionNonceSize)...)
	return bytes.NewReader(value)
}
