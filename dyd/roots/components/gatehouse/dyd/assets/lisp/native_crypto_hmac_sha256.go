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

var nativeCryptoHMACSHA256VerifyDocumentation = doc(
	"(hmac/verify key message tag) -> Boolean",
	"Returns whether tag is the HMAC-SHA256 of key and message using a constant-time comparison.",
	"(import (hmac @native:crypto/mac/hmac/sha256/v1) (hmac/verify (bytes/utf8/encode \"key\") (bytes/utf8/encode \"abc\") (bytes/hex/decode \"9c196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab\")))",
	"#t",
)

func nativeCryptoHMACSHA256Module() Expr {
	digest := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoHMACSHA256Digest),
	}, nativeCryptoHMACSHA256Documentation.text())
	verify := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoHMACSHA256Verify),
	}, nativeCryptoHMACSHA256VerifyDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("digest"), digest),
		pairValue(symbol("verify"), verify),
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

func nativeCryptoHMACSHA256Verify(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError("hmac/verify requires key, message, and tag Bytes values"), nil
	}
	err, key := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	err, message := requireBytes(arguments[1])
	if err != nil {
		return err, nil
	}
	err, tag := requireBytes(arguments[2])
	if err != nil {
		return err, nil
	}

	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	return nil, boolean(hmac.Equal(mac.Sum(nil), []byte(tag)))
}
