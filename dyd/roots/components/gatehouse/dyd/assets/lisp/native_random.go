package lisp

import (
	"crypto/rand"
	"encoding/binary"
)

const nativeRandomID = "native:random/v1"

func nativeRandomModule() Expr {
	bytes := Expr{
		kind:    exprBuiltin,
		help:    "Returns cryptographically secure random Bytes of the requested length.",
		builtin: &builtin{call: pure(nativeRandomBytes)},
	}
	integer := Expr{
		kind:    exprBuiltin,
		help:    "Returns a uniformly random integer less than a positive bound.",
		builtin: &builtin{call: pure(nativeRandomInteger)},
	}
	boolean := Expr{
		kind:    exprBuiltin,
		help:    "Returns an unbiased random Boolean.",
		builtin: &builtin{call: pure(nativeRandomBoolean)},
	}
	exports := list([]Expr{
		pairValue(symbol("bytes", Span{}), bytes, Span{}),
		pairValue(symbol("int", Span{}), integer, Span{}),
		pairValue(symbol("bool", Span{}), boolean, Span{}),
	}, Span{})
	return list([]Expr{
		symbol("quote", Span{}),
		exports,
	}, Span{})
}

func nativeRandomBytes(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "random/bytes requires one non-negative integer"), Expr{}
	}
	err, length := requireInteger(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	if length < 0 {
		return expressionError(span, "random/bytes requires one non-negative integer"), Expr{}
	}
	err, value := nativeRandomRead(length, span)
	if err != nil {
		return err, Expr{}
	}
	return nil, bytesValue(value, Span{})
}

func nativeRandomInteger(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "random/int requires one positive integer"), Expr{}
	}
	err, bound := requireInteger(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	if bound <= 0 {
		return expressionError(span, "random/int requires one positive integer"), Expr{}
	}

	rangeSize := uint64(1) << 63
	limit := rangeSize - rangeSize%uint64(bound)
	for {
		err, value := nativeRandomRead(8, span)
		if err != nil {
			return err, Expr{}
		}
		candidate := binary.BigEndian.Uint64([]byte(value)) >> 1
		if candidate < limit {
			return nil, integer(int64(candidate%uint64(bound)), Span{})
		}
	}
}

func nativeRandomBoolean(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 0 {
		return expressionError(span, "random/bool requires no arguments"), Expr{}
	}
	err, value := nativeRandomRead(1, span)
	if err != nil {
		return err, Expr{}
	}
	return nil, boolean(value[0]&1 == 1, Span{})
}

func nativeRandomRead(length int64, span Span) (error, string) {
	if length > int64(^uint(0)>>1) {
		return expressionError(span, "random/bytes length is too large"), ""
	}
	value := make([]byte, int(length))
	if _, err := rand.Read(value); err != nil {
		return expressionError(span, "random source failed"), ""
	}
	return nil, string(value)
}
