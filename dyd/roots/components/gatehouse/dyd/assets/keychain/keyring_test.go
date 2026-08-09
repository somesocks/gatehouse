package keychain

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestPrepareBootstrapsAndUnlocksKeychains(t *testing.T) {
	store := openKeyringTestStore(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	reference := model.KeychainRef{Id: "default", Version: 1}
	firstDEK := bytes.Repeat([]byte{2}, keySize)
	err, keyring := prepare(context.Background(), store, configured, testResolver(map[string]string{"DEFAULT": "passphrase"}), testRandom(1, 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	if got := keyring.keys[reference]; !bytes.Equal(got, firstDEK) {
		t.Fatalf("prepared DEK = %x, want %x", got, firstDEK)
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

	err, repeat := prepare(context.Background(), store, configured, testResolver(map[string]string{"DEFAULT": "passphrase"}), testRandom(4, 5, 6))
	if err != nil {
		t.Fatal(err)
	}
	defer repeat.Close()
	if got := repeat.keys[reference]; !bytes.Equal(got, firstDEK) {
		t.Fatalf("repeated prepared DEK = %x, want persisted %x", got, firstDEK)
	}
}

func TestPrepareRejectsWrongPassphrase(t *testing.T) {
	store := openKeyringTestStore(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	err, keyring := prepare(context.Background(), store, configured, testResolver(map[string]string{"DEFAULT": "correct"}), testRandom(1, 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	keyring.Close()

	err, _ = prepare(context.Background(), store, configured, testResolver(map[string]string{"DEFAULT": "wrong"}), testRandom(4, 5, 6))
	if err == nil || !strings.Contains(err.Error(), "decrypt DEK") {
		t.Fatalf("Prepare() error = %v, want decryption failure", err)
	}
}

func TestKeyringGetLoadsExactVersion(t *testing.T) {
	store := openKeyringTestStore(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	err, keyring := prepare(context.Background(), store, configured, testResolver(map[string]string{"DEFAULT": "passphrase"}), testRandom(1, 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()

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

func TestPrepareConcurrentReplicas(t *testing.T) {
	configuration := config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: filepath.Join(t.TempDir(), "gatehouse.db"),
	}
	initial := openKeyringTestStore(t, configuration)
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}

	first := openKeyringTestStore(t, configuration)
	defer first.Close()
	second := openKeyringTestStore(t, configuration)
	defer second.Close()
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}
	type result struct {
		err     error
		keyring *Keyring
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for index, store := range []*database.Store{first, second} {
		wait.Add(1)
		go func(index int, store *database.Store) {
			defer wait.Done()
			<-start
			err, keyring := prepare(
				context.Background(),
				store,
				configured,
				testResolver(map[string]string{"DEFAULT": "passphrase"}),
				testRandom(byte(index+1), byte(index+11), byte(index+21)),
			)
			results <- result{err: err, keyring: keyring}
		}(index, store)
	}
	close(start)
	wait.Wait()
	close(results)

	var keyrings []*Keyring
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		keyrings = append(keyrings, result.keyring)
	}
	defer keyrings[0].Close()
	defer keyrings[1].Close()
	reference := model.KeychainRef{Id: "default", Version: 1}
	if !bytes.Equal(keyrings[0].keys[reference], keyrings[1].keys[reference]) {
		t.Fatalf("replicas unlocked different DEKs: %x and %x", keyrings[0].keys[reference], keyrings[1].keys[reference])
	}
	err, stored := first.KeychainsGet(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored keychains = %#v, want one keychain", stored)
	}
}

func TestKeychainAssociatedData(t *testing.T) {
	if got, want := string(keychainAssociatedData(model.KeychainRef{Id: "default", Version: 1})), "gh=v1|key=default|ver=1"; got != want {
		t.Fatalf("keychainAssociatedData() = %q, want %q", got, want)
	}
}

func openKeyringTestStore(t *testing.T, configuration config.DatabaseConfig) *database.Store {
	t.Helper()
	err, store := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	err, migrations := database.BuildMigrations(configuration, config.State{})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background(), store, migrations); err != nil {
		store.Close()
		t.Fatal(err)
	}
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
