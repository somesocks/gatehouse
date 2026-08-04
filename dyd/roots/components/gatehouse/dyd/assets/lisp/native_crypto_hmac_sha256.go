package lisp

import (
	"crypto/hmac"
	"crypto/sha256"
)

const nativeCryptoHMACSHA256ID = "native:crypto/hmac/sha256/v1"

func nativeCryptoHMACSHA256Module() Expr {
	digest := Expr{
		kind: exprBuiltin,
		help: "Returns the HMAC-SHA256 digest of key and message Bytes.",
		builtin: &builtin{
			leaky: true,
			call:  pure(nativeCryptoHMACSHA256Digest),
		},
	}
	exports := list([]Expr{
		pairValue(symbol("digest", Span{}), digest, Span{}),
	}, Span{})
	return list([]Expr{
		symbol("quote", Span{}),
		exports,
	}, Span{})
}

func nativeCryptoHMACSHA256Digest(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "hmac/digest requires key and message Bytes values"), Expr{}
	}
	err, key := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	err, message := requireBytes(arguments[1], span)
	if err != nil {
		return err, Expr{}
	}

	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	return nil, bytesValue(string(mac.Sum(nil)), Span{})
}
