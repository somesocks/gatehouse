package lisp

import (
	_ "embed"
	"strings"
)

const nativeSeqID = "native:seq/v1"

//go:embed native_seq.lisp
var nativeSeqSource string

var nativeSeqExports = compileNativeSeq()

func nativeSeqModule() Expr {
	return list([]Expr{
		symbol("quote"),
		nativeSeqExports,
	})
}

func compileNativeSeq() Expr {
	err, expression := Read(nativeSeqSource)
	if err != nil {
		panic("parse native sequence module: " + err.Error())
	}
	err, result := (&evaluator{}).eval(expression, bootstrap())
	if err != nil {
		panic("bootstrap native sequence module: " + err.Error())
	}
	err, exports := moduleExports(result)
	if err != nil {
		panic("bootstrap native sequence exports: " + err.Error())
	}

	values := make([]Expr, 0, len(exports))
	foundBytesSplit, foundBytesCollect := false, false
	for _, export := range exports {
		value := export.value
		switch export.name {
		case "bytes/split":
			foundBytesSplit = true
			value = withHelp(&builtin{leaky: true, call: pure(nativeSeqBytesSplit)}, helpOf(value))
		case "bytes/collect":
			foundBytesCollect = true
			value = withHelp(&builtin{leaky: true, call: valueCall(nativeSeqBytesCollect)}, helpOf(value))
		}
		values = append(values, pairValue(symbol(export.name), value))
	}
	if !foundBytesSplit || !foundBytesCollect {
		panic("bootstrap native sequence exports are missing byte adapters")
	}
	return list(values)
}

func nativeSeqBytesSplit(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("seq/bytes/split requires Bytes and a positive block size"), nil
	}
	err, blockSize := requireInteger(arguments[1])
	if err != nil {
		return err, nil
	}
	if blockSize <= 0 {
		return expressionError("seq/bytes/split requires a positive block size"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, nativeSeqBytesSplitAt(arguments[0], value, blockSize, 0)
}

func nativeSeqBytesSplitAt(source Expr, value string, blockSize int64, start int64) Expr {
	if start >= int64(len(value)) {
		return null()
	}
	end := start + blockSize
	if end > int64(len(value)) {
		end = int64(len(value))
	}
	block := Expr(bytesValue(value[start:end]))
	if taint := TaintOf(source); taint != TaintNone {
		block = withTaint(block, taint)
	}
	tail := &builtin{call: pure(func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 0 {
			return expressionError("sequence tail requires no arguments"), nil
		}
		return nil, nativeSeqBytesSplitAt(source, value, blockSize, end)
	})}
	return pairValue(block, tail)
}

func nativeSeqBytesCollect(evaluator *evaluator, env *environment, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 && len(arguments) != 2 {
		return expressionError("seq/bytes/collect requires a sequence and optional non-negative maximum bytes"), nil
	}
	maximum := int64(-1)
	if len(arguments) == 2 {
		err, value := requireInteger(arguments[1])
		if err != nil {
			return err, nil
		}
		if value < 0 {
			return expressionError("seq/bytes/collect requires a non-negative maximum bytes"), nil
		}
		maximum = value
	}
	taint := TaintOf(arguments[0])
	var builder strings.Builder
	for sequence := arguments[0]; !isNullValue(sequence); {
		base, _ := unwrap(sequence)
		pair, ok := base.(*pair)
		if !ok {
			return expressionError("seq/bytes/collect requires a sequence"), nil
		}
		err, value := requireBytes(pair.first)
		if err != nil {
			return err, nil
		}
		taint = joinTaint(taint, TaintOf(pair.first))
		if maximum >= 0 && int64(len(value)) > maximum-int64(builder.Len()) {
			return expressionError("seq/bytes/collect exceeds maximum bytes"), nil
		}
		builder.WriteString(value)
		err, sequence = evaluator.call(pair.rest, env, nil)
		if err != nil {
			return err, nil
		}
	}
	result := Expr(bytesValue(builder.String()))
	if taint != TaintNone {
		result = withTaint(result, taint)
	}
	return nil, result
}
