package keychain

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"strconv"
	"sync"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type Keyring struct {
	store       *database.Store
	ids         []string
	passphrases map[string][]byte
	keys        map[model.KeychainRef][]byte
	random      io.Reader
	mutex       sync.Mutex
}

func NewKeyring(store *database.Store, configured []config.Keychain, resolver *PassphraseSourceResolver) (error, *Keyring) {
	return newKeyring(store, configured, resolver, rand.Reader)
}

func newKeyring(store *database.Store, configured []config.Keychain, resolver *PassphraseSourceResolver, random io.Reader) (error, *Keyring) {
	keyring := &Keyring{
		store:       store,
		ids:         make([]string, 0, len(configured)),
		passphrases: make(map[string][]byte, len(configured)),
		keys:        make(map[model.KeychainRef][]byte, len(configured)),
		random:      random,
	}
	for _, configuredKeychain := range configured {
		err, passphrase := resolver.Resolve(configuredKeychain)
		if err != nil {
			keyring.Close()
			return err, nil
		}
		keyring.ids = append(keyring.ids, configuredKeychain.ID)
		keyring.passphrases[configuredKeychain.ID] = passphrase
	}
	return nil, keyring
}

func (keyring *Keyring) Candidates() (error, []model.Keychain) {
	keyring.mutex.Lock()
	defer keyring.mutex.Unlock()

	candidates := make([]model.Keychain, 0, len(keyring.ids))
	for _, id := range keyring.ids {
		reference := model.KeychainRef{Id: id, Version: 1}
		passphrase, ok := keyring.passphrases[id]
		if !ok {
			return fmt.Errorf("generate keychain %q: no cached passphrase", id), nil
		}

		salt := make([]byte, saltSize)
		if _, err := io.ReadFull(keyring.random, salt); err != nil {
			return fmt.Errorf("generate keychain KDF salt for %q: %w", reference.Id, err), nil
		}
		kdf := KDF{Salt: salt}
		err, kek := DeriveKEK(passphrase, kdf)
		if err != nil {
			return fmt.Errorf("derive KEK for keychain %q: %w", reference.Id, err), nil
		}

		dek := make([]byte, keySize)
		if _, err := io.ReadFull(keyring.random, dek); err != nil {
			clear(kek)
			return fmt.Errorf("generate keychain DEK for %q: %w", reference.Id, err), nil
		}
		err, encrypted := Seal(keyring.random, kek, keychainAssociatedData(reference), dek)
		clear(kek)
		clear(dek)
		if err != nil {
			return fmt.Errorf("encrypt DEK for keychain %q: %w", reference.Id, err), nil
		}
		candidates = append(candidates, model.Keychain{Ref: reference, KekKdf: kdf.String(), Key: encrypted.String(), Enabled: true})
	}
	return nil, candidates
}

func (keyring *Keyring) Get(ctx context.Context, references []model.KeychainRef) (error, map[model.KeychainRef][]byte) {
	keyring.mutex.Lock()
	defer keyring.mutex.Unlock()

	keys := make(map[model.KeychainRef][]byte, len(references))
	missing := make([]model.KeychainRef, 0, len(references))
	seen := make(map[model.KeychainRef]struct{}, len(references))
	for _, reference := range references {
		if _, exists := seen[reference]; exists {
			continue
		}
		seen[reference] = struct{}{}
		if key, ok := keyring.keys[reference]; ok {
			keys[reference] = append([]byte(nil), key...)
			continue
		}
		missing = append(missing, reference)
	}
	if len(missing) == 0 {
		return nil, keys
	}

	err, stored := keyring.store.KeychainsGet(ctx, missing)
	if err != nil {
		return err, nil
	}
	keychains := make(map[model.KeychainRef]model.Keychain, len(stored))
	for _, keychain := range stored {
		keychains[keychain.Ref] = keychain
	}
	for _, reference := range missing {
		keychain, ok := keychains[reference]
		if !ok {
			return fmt.Errorf("get keychain %q version %d: not found", reference.Id, reference.Version), nil
		}
		passphrase, ok := keyring.passphrases[reference.Id]
		if !ok {
			return fmt.Errorf("get keychain %q version %d: no cached passphrase", reference.Id, reference.Version), nil
		}
		err, dek := decryptKeychain(reference, keychain, passphrase)
		if err != nil {
			return err, nil
		}
		keyring.keys[reference] = dek
		keys[reference] = append([]byte(nil), dek...)
	}
	return nil, keys
}

func (keyring *Keyring) Current(ctx context.Context) (error, []model.KeychainRef) {
	keyring.mutex.Lock()
	ids := append([]string(nil), keyring.ids...)
	keyring.mutex.Unlock()
	err, keychains := keyring.store.KeychainsGetCurrent(ctx, ids)
	if err != nil {
		return err, nil
	}
	references := make([]model.KeychainRef, 0, len(keychains))
	for _, keychain := range keychains {
		references = append(references, keychain.Ref)
	}
	return nil, references
}

func (keyring *Keyring) Close() {
	keyring.mutex.Lock()
	defer keyring.mutex.Unlock()
	for _, passphrase := range keyring.passphrases {
		clear(passphrase)
	}
	clear(keyring.passphrases)
	for _, key := range keyring.keys {
		clear(key)
	}
	clear(keyring.keys)
}

func decryptKeychain(reference model.KeychainRef, keychain model.Keychain, passphrase []byte) (error, []byte) {
	err, kdf := ParseKDF(keychain.KekKdf)
	if err != nil {
		return fmt.Errorf("parse KDF for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
	}
	err, kek := DeriveKEK(passphrase, kdf)
	if err != nil {
		return fmt.Errorf("derive KEK for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
	}
	err, encrypted := ParseKey(keychain.Key)
	if err != nil {
		clear(kek)
		return fmt.Errorf("parse encrypted DEK for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
	}
	err, dek := Open(kek, keychainAssociatedData(reference), encrypted)
	clear(kek)
	if err != nil {
		return fmt.Errorf("decrypt DEK for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
	}
	if len(dek) != keySize {
		clear(dek)
		return fmt.Errorf("decrypt DEK for keychain %q version %d: expected a %d-byte key", reference.Id, reference.Version, keySize), nil
	}
	return nil, dek
}

func keychainAssociatedData(reference model.KeychainRef) []byte {
	return []byte("gh=v1|key=" + reference.Id + "|ver=" + strconv.Itoa(reference.Version))
}
