package lisp

import _ "embed"

const nativeSeqID = "native:seq/v1"

//go:embed native_seq.lisp
var nativeSeqSource string

var nativeSeqAST = parseNativeSeqSource()

func nativeSeqModule() Expr {
	return nativeSeqAST
}

func parseNativeSeqSource() Expr {
	err, expression := Read(nativeSeqSource)
	if err != nil {
		panic("parse native sequence module: " + err.Error())
	}
	return expression
}
