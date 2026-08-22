package lisp

import _ "embed"

const nativeCompressDeflateID = "native:compress/deflate/v1"

//go:embed native_compress_deflate.lisp
var nativeCompressDeflateSource string

var nativeCompressDeflateAST = parseNativeCompressDeflateSource()

func nativeCompressDeflateModule() Expr {
	return nativeCompressDeflateAST
}

func parseNativeCompressDeflateSource() Expr {
	err, expression := Read(nativeCompressDeflateSource)
	if err != nil {
		panic("parse native DEFLATE module: " + err.Error())
	}
	return expression
}
