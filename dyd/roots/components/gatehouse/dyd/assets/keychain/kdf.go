package keychain

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"fmt"
)

const (
	keySize           = 16
	pbkdf2Iterations  = 600000
)

func DeriveKEK(passphrase []byte, kdf KDF) (error, []byte) {
	if !validPassphrase(passphrase) {
		return fmt.Errorf("keychain passphrase must not be empty or contain NUL"), nil
	}
	if len(kdf.Salt) != saltSize {
		return fmt.Errorf("keychain KDF salt must be %d bytes", saltSize), nil
	}
	key, err := pbkdf2.Key(sha256.New, string(passphrase), kdf.Salt, pbkdf2Iterations, keySize)
	if err != nil {
		return fmt.Errorf("derive keychain KEK: %w", err), nil
	}
	return nil, key
}
