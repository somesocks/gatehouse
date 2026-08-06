package lisp

import _ "embed"

const nativeCryptoCipherCBCID = "native:crypto/cipher/cbc/v1"

//go:embed native_crypto_cipher_cbc.lisp
var nativeCryptoCipherCBCSource string

var nativeCryptoCipherCBCAST = parseNativeCryptoCipherCBCSource()

func nativeCryptoCipherCBCModule() Expr {
	return nativeCryptoCipherCBCAST
}

func parseNativeCryptoCipherCBCSource() Expr {
	err, expression := Read(nativeCryptoCipherCBCSource)
	if err != nil {
		panic("parse native CBC cipher module: " + err.Error())
	}
	return expression
}
