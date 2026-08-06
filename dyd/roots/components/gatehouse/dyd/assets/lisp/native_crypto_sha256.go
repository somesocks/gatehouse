package lisp

import "crypto/sha256"

const nativeCryptoSHA256ID = "native:crypto/digest/sha256/v1"

var nativeCryptoSHA256Documentation = doc(
	"(sha256/digest bytes) -> Bytes",
	"Returns the SHA-256 digest of Bytes.",
	"(import (sha256 @native:crypto/digest/sha256/v1) (bytes/hex/encode (sha256/digest (bytes/utf8/encode \"abc\"))))",
	"\"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\"",
)

func nativeCryptoSHA256Module() Expr {
	digest := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoSHA256Digest),
	}, nativeCryptoSHA256Documentation.text())
	exports := list([]Expr{
		pairValue(symbol("digest"), digest),
	})
	return list([]Expr{
		symbol("quote"),
		exports,
	})
}

func nativeCryptoSHA256Digest(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("sha256/digest requires one Bytes value"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	digest := sha256.Sum256([]byte(value))
	return nil, bytesValue(string(digest[:]))
}
