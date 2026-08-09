package keychain

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"strconv"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type Keyring struct {
	keys map[model.KeychainRef][]byte
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

	keyring := &Keyring{keys: make(map[model.KeychainRef][]byte, len(references))}
	for _, reference := range references {
		keychain, ok := stored[reference]
		if !ok {
			keyring.Close()
			return fmt.Errorf("get keychain %q version %d: not found after insert", reference.Id, reference.Version), nil
		}
		err, kdf := ParseKDF(keychain.KekKdf)
		if err != nil {
			keyring.Close()
			return fmt.Errorf("parse KDF for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
		}
		err, kek := DeriveKEK(passphrases[reference], kdf)
		if err != nil {
			keyring.Close()
			return fmt.Errorf("derive KEK for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
		}
		err, encrypted := ParseKey(keychain.Key)
		if err != nil {
			clear(kek)
			keyring.Close()
			return fmt.Errorf("parse encrypted DEK for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
		}
		err, dek := Open(kek, keychainAssociatedData(reference), encrypted)
		clear(kek)
		if err != nil {
			keyring.Close()
			return fmt.Errorf("decrypt DEK for keychain %q version %d: %w", reference.Id, reference.Version, err), nil
		}
		if len(dek) != keySize {
			clear(dek)
			keyring.Close()
			return fmt.Errorf("decrypt DEK for keychain %q version %d: expected a %d-byte key", reference.Id, reference.Version, keySize), nil
		}
		keyring.keys[reference] = dek
	}
	return nil, keyring
}

func (keyring *Keyring) Close() {
	for _, key := range keyring.keys {
		clear(key)
	}
	clear(keyring.keys)
}

func keychainAssociatedData(reference model.KeychainRef) []byte {
	return []byte("gh=v1|key=" + reference.Id + "|ver=" + strconv.Itoa(reference.Version))
}
