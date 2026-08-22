package lisp

import _ "embed"

const nativeArchiveZIPID = "native:archive/zip/v1"

//go:embed native_archive_zip.lisp
var nativeArchiveZIPSource string

var nativeArchiveZIPAST = parseNativeArchiveZIPSource()

func nativeArchiveZIPModule() Expr {
	return nativeArchiveZIPAST
}

func parseNativeArchiveZIPSource() Expr {
	err, expression := Read(nativeArchiveZIPSource)
	if err != nil {
		panic("parse native ZIP module: " + err.Error())
	}
	return expression
}
