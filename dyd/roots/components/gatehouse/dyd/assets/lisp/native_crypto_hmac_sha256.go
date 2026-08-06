package lisp

import (
	"crypto/hmac"
	"crypto/sha256"
)

const nativeCryptoHMACSHA256ID = "native:crypto/mac/hmac/sha256/v1"

var nativeCryptoHMACSHA256Documentation = doc(
	"(hmac/digest key message) -> Bytes",
	"Returns the HMAC-SHA256 digest of key and message Bytes.",
	"(import (hmac @native:crypto/mac/hmac/sha256/v1) (bytes/hex/encode (hmac/digest (bytes/utf8/encode \"key\") (bytes/utf8/encode \"abc\"))))",
	"\"9c196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab\"",
)

func nativeCryptoHMACSHA256Module() Expr {
	digest := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoHMACSHA256Digest),
	}, nativeCryptoHMACSHA256Documentation.text())
	exports := list([]Expr{
		pairValue(symbol("digest"), digest),
	})
	return list([]Expr{
		symbol("quote"),
		exports,
	})
}

func nativeCryptoHMACSHA256Digest(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("hmac/digest requires key and message Bytes values"), nil
	}
	err, key := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	err, message := requireBytes(arguments[1])
	if err != nil {
		return err, nil
	}

	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	return nil, bytesValue(string(mac.Sum(nil)))
}
