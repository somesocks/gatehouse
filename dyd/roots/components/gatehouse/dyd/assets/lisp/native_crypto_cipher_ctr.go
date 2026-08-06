package lisp

import _ "embed"

const nativeCryptoCipherCTRID = "native:crypto/cipher/ctr/v1"

//go:embed native_crypto_cipher_ctr.lisp
var nativeCryptoCipherCTRSource string

var nativeCryptoCipherCTRAST = parseNativeCryptoCipherCTRSource()

func nativeCryptoCipherCTRModule() Expr {
	return nativeCryptoCipherCTRAST
}

func parseNativeCryptoCipherCTRSource() Expr {
	err, expression := Read(nativeCryptoCipherCTRSource)
	if err != nil {
		panic("parse native CTR cipher module: " + err.Error())
	}
	return expression
}
