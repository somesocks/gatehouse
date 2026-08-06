package lisp

import _ "embed"

const nativeCryptoPaddingPKCS7ID = "native:crypto/padding/pkcs7/v1"

//go:embed native_crypto_padding_pkcs7.lisp
var nativeCryptoPaddingPKCS7Source string

var nativeCryptoPaddingPKCS7AST = parseNativeCryptoPaddingPKCS7Source()

func nativeCryptoPaddingPKCS7Module() Expr {
	return nativeCryptoPaddingPKCS7AST
}

func parseNativeCryptoPaddingPKCS7Source() Expr {
	err, expression := Read(nativeCryptoPaddingPKCS7Source)
	if err != nil {
		panic("parse native PKCS#7 padding module: " + err.Error())
	}
	return expression
}
