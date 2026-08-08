package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
)

const (
	encryptionNonceSize = 12
	encryptionTagSize   = 16
	encryptionOverhead  = encryptionNonceSize + encryptionTagSize
)

func Seal(random io.Reader, key, associatedData, plaintext []byte) (error, Encrypted) {
	err, cipher := gcm(key)
	if err != nil {
		return err, Encrypted{}
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return fmt.Errorf("generate encryption nonce: %w", err), Encrypted{}
	}
	payload := append(nonce, cipher.Seal(nil, nonce, plaintext, associatedData)...)
	return nil, Encrypted{Payload: payload}
}

func Open(key, associatedData []byte, encrypted Encrypted) (error, []byte) {
	err, cipher := gcm(key)
	if err != nil {
		return err, nil
	}
	if len(encrypted.Payload) < cipher.NonceSize()+cipher.Overhead() {
		return fmt.Errorf("encrypted payload must contain a nonce and authentication tag"), nil
	}
	nonce := encrypted.Payload[:cipher.NonceSize()]
	plaintext, err := cipher.Open(nil, nonce, encrypted.Payload[cipher.NonceSize():], associatedData)
	if err != nil {
		return fmt.Errorf("decrypt encrypted payload: %w", err), nil
	}
	return nil, plaintext
}

func gcm(key []byte) (error, cipher.AEAD) {
	if len(key) != keySize {
		return fmt.Errorf("AES-128-GCM requires a %d-byte key", keySize), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("initialize AES-128-GCM: %w", err), nil
	}
	cipher, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("initialize AES-128-GCM: %w", err), nil
	}
	if cipher.NonceSize() != encryptionNonceSize || cipher.Overhead() != encryptionTagSize {
		return fmt.Errorf("unexpected AES-128-GCM format"), nil
	}
	return nil, cipher
}
