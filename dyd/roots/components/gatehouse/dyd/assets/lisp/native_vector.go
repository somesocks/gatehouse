package lisp

import _ "embed"

const nativeVectorID = "native:vector/v1"

//go:embed native_vector.lisp
var nativeVectorSource string

var nativeVectorExports = compileNativeVector()

func nativeVectorModule() Expr {
	return list([]Expr{symbol("quote"), nativeVectorExports})
}

func compileNativeVector() Expr {
	err, expression := Read(nativeVectorSource)
	if err != nil {
		panic("parse native vector module: " + err.Error())
	}
	err, result := (&evaluator{}).eval(expression, bootstrap())
	if err != nil {
		panic("bootstrap native vector module: " + err.Error())
	}
	err, exports := moduleExports(result)
	if err != nil {
		panic("bootstrap native vector exports: " + err.Error())
	}
	values := make([]Expr, 0, len(exports))
	for _, export := range exports {
		values = append(values, pairValue(symbol(export.name), export.value))
	}
	return list(values)
}
