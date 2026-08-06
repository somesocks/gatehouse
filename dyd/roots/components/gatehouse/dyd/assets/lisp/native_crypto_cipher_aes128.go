package lisp

import "crypto/aes"

const nativeCryptoCipherAES128ID = "native:crypto/cipher/aes/128/v1"

var nativeCryptoCipherAES128EncryptDocumentation = doc(
	"(aes/encrypt key block) -> Bytes",
	"Encrypts one 16-byte block with a 16-byte AES-128 key.",
	"(import (aes @native:crypto/cipher/aes/128/v1) (bytes/hex/encode (aes/encrypt (bytes/hex/decode \"000102030405060708090a0b0c0d0e0f\") (bytes/hex/decode \"00112233445566778899aabbccddeeff\"))))",
	"\"69c4e0d86a7b0430d8cdb78070b4c55a\"",
)

var nativeCryptoCipherAES128DecryptDocumentation = doc(
	"(aes/decrypt key block) -> Bytes",
	"Decrypts one 16-byte block with a 16-byte AES-128 key.",
	"(import (aes @native:crypto/cipher/aes/128/v1) (bytes/hex/encode (aes/decrypt (bytes/hex/decode \"000102030405060708090a0b0c0d0e0f\") (bytes/hex/decode \"69c4e0d86a7b0430d8cdb78070b4c55a\"))))",
	"\"00112233445566778899aabbccddeeff\"",
)

func nativeCryptoCipherAES128Module() Expr {
	encrypt := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoCipherAES128Encrypt),
	}, nativeCryptoCipherAES128EncryptDocumentation.text())
	decrypt := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoCipherAES128Decrypt),
	}, nativeCryptoCipherAES128DecryptDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("encrypt"), encrypt),
		pairValue(symbol("decrypt"), decrypt),
	})
	return list([]Expr{
		symbol("quote"),
		exports,
	})
}

func nativeCryptoCipherAES128Encrypt(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("aes/encrypt requires key and block Bytes values"), nil
	}
	err, key := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	err, block := requireBytes(arguments[1])
	if err != nil {
		return err, nil
	}
	if len(key) != aes.BlockSize {
		return expressionError("aes/encrypt requires a 16-byte key"), nil
	}
	if len(block) != aes.BlockSize {
		return expressionError("aes/encrypt requires a 16-byte block"), nil
	}
	cipher, cipherErr := aes.NewCipher([]byte(key))
	if cipherErr != nil {
		return expressionError("aes/encrypt could not initialize AES-128"), nil
	}
	result := make([]byte, aes.BlockSize)
	cipher.Encrypt(result, []byte(block))
	return nil, bytesValue(string(result))
}

func nativeCryptoCipherAES128Decrypt(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("aes/decrypt requires key and block Bytes values"), nil
	}
	err, key := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	err, block := requireBytes(arguments[1])
	if err != nil {
		return err, nil
	}
	if len(key) != aes.BlockSize {
		return expressionError("aes/decrypt requires a 16-byte key"), nil
	}
	if len(block) != aes.BlockSize {
		return expressionError("aes/decrypt requires a 16-byte block"), nil
	}
	cipher, cipherErr := aes.NewCipher([]byte(key))
	if cipherErr != nil {
		return expressionError("aes/decrypt could not initialize AES-128"), nil
	}
	result := make([]byte, aes.BlockSize)
	cipher.Decrypt(result, []byte(block))
	return nil, bytesValue(string(result))
}
