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
	store      *database.Store
	configured map[string]config.Keychain
	resolver   *PassphraseSourceResolver
	keys       map[model.KeychainRef][]byte
	mutex      sync.Mutex
}

func Prepare(ctx context.Context, store *database.Store, configured []config.Keychain, resolver *PassphraseSourceResolver) (error, *Keyring) {
	return prepare(ctx, store, configured, resolver, rand.Reader)
}

func prepare(ctx context.Context, store *database.Store, configured []config.Keychain, resolver *PassphraseSourceResolver, random io.Reader) (error, *Keyring) {
	references := make([]model.KeychainRef, 0, len(configured))
	candidates := make([]model.Keychain, 0, len(configured))
	passphrases := make(map[model.KeychainRef][]byte, len(configured))
	defer func() {
		for _, passphrase := range passphrases {
			clear(passphrase)
		}
	}()

	for _, configuredKeychain := range configured {
		reference := model.KeychainRef{Id: configuredKeychain.ID, Version: 1}
		err, passphrase := resolver.Resolve(configuredKeychain)
		if err != nil {
			return err, nil
		}
		passphrases[reference] = passphrase

		salt := make([]byte, saltSize)
		if _, err := io.ReadFull(random, salt); err != nil {
			return fmt.Errorf("generate keychain KDF salt for %q: %w", reference.Id, err), nil
		}
		kdf := KDF{Salt: salt}
		err, kek := DeriveKEK(passphrase, kdf)
		if err != nil {
			return fmt.Errorf("derive KEK for keychain %q: %w", reference.Id, err), nil
		}

		dek := make([]byte, keySize)
		if _, err := io.ReadFull(random, dek); err != nil {
			clear(kek)
			return fmt.Errorf("generate keychain DEK for %q: %w", reference.Id, err), nil
		}
		err, encrypted := Seal(random, kek, keychainAssociatedData(reference), dek)
		clear(kek)
		clear(dek)
		if err != nil {
			return fmt.Errorf("encrypt DEK for keychain %q: %w", reference.Id, err), nil
		}

		references = append(references, reference)
		candidates = append(candidates, model.Keychain{
			Ref:     reference,
			KekKdf:  kdf.String(),
			Key:     encrypted.String(),
			Enabled: true,
		})
	}

	if err := store.KeychainsInsert(ctx, candidates); err != nil {
		return err, nil
	}
	err, persisted := store.KeychainsGet(ctx, references)
	if err != nil {
		return err, nil
	}
	stored := make(map[model.KeychainRef]model.Keychain, len(persisted))
	for _, keychain := range persisted {
		stored[keychain.Ref] = keychain
	}

	keyring := newKeyring(store, configured, resolver)
	for _, reference := range references {
		keychain, ok := stored[reference]
		if !ok {
			keyring.Close()
			return fmt.Errorf("get keychain %q version %d: not found after insert", reference.Id, reference.Version), nil
		}
		err, dek := decryptKeychain(reference, keychain, passphrases[reference])
		if err != nil {
			keyring.Close()
			return err, nil
		}
		keyring.keys[reference] = dek
	}
	return nil, keyring
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
	passphrases := make(map[string][]byte)
	defer func() {
		for _, passphrase := range passphrases {
			clear(passphrase)
		}
	}()
	for _, reference := range missing {
		keychain, ok := keychains[reference]
		if !ok {
			return fmt.Errorf("get keychain %q version %d: not found", reference.Id, reference.Version), nil
		}
		passphrase, ok := passphrases[reference.Id]
		if !ok {
			configured, ok := keyring.configured[reference.Id]
			if !ok {
				return fmt.Errorf("get keychain %q version %d: no configured passphrase source", reference.Id, reference.Version), nil
			}
			err, passphrase = keyring.resolver.Resolve(configured)
			if err != nil {
				return err, nil
			}
			passphrases[reference.Id] = passphrase
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

func (keyring *Keyring) Close() {
	keyring.mutex.Lock()
	defer keyring.mutex.Unlock()
	for _, key := range keyring.keys {
		clear(key)
	}
	clear(keyring.keys)
}

func newKeyring(store *database.Store, configured []config.Keychain, resolver *PassphraseSourceResolver) *Keyring {
	sources := make(map[string]config.Keychain, len(configured))
	for _, keychain := range configured {
		sources[keychain.ID] = keychain
	}
	return &Keyring{
		store:      store,
		configured: sources,
		resolver:   resolver,
		keys:       make(map[model.KeychainRef][]byte, len(configured)),
	}
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
