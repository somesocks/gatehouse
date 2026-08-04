package lisp

import "crypto/sha256"

const nativeCryptoSHA256ID = "native:crypto/sha256/v1"

func nativeCryptoSHA256Module() Expr {
	digest := Expr{
		kind: exprBuiltin,
		help: "Returns the SHA-256 digest of Bytes.",
		builtin: &builtin{
			leaky: true,
			call:  pure(nativeCryptoSHA256Digest),
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

func nativeCryptoSHA256Digest(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "sha256/digest requires one Bytes value"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	digest := sha256.Sum256([]byte(value))
	return nil, bytesValue(string(digest[:]), Span{})
}
