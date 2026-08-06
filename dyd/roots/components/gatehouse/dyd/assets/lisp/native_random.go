package lisp

import (
	"crypto/rand"
	"encoding/binary"
)

const nativeRandomID = "native:random/v1"

var nativeRandomBytesDocumentation = doc(
	"(random/bytes length) -> Bytes",
	"Returns cryptographically secure random Bytes of the requested non-negative length.",
	"(import (random @native:random/v1) (bytes/length (random/bytes 4)))",
	"4",
)

var nativeRandomIntegerDocumentation = doc(
	"(random/int bound) -> Integer",
	"Returns a uniformly random integer less than a positive bound.",
	"(import (random @native:random/v1) (let ((value (random/int 10))) (and (<= 0 value) (< value 10))))",
	"#t",
)

var nativeRandomBooleanDocumentation = doc(
	"(random/bool) -> Boolean",
	"Returns an unbiased random Boolean.",
	"(import (random @native:random/v1) (bool? (random/bool)))",
	"#t",
)

func nativeRandomModule() Expr {
	bytes := withHelp(&builtin{call: pure(nativeRandomBytes)}, nativeRandomBytesDocumentation.text())
	integer := withHelp(&builtin{call: pure(nativeRandomInteger)}, nativeRandomIntegerDocumentation.text())
	boolean := withHelp(&builtin{call: pure(nativeRandomBoolean)}, nativeRandomBooleanDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("bytes"), bytes),
		pairValue(symbol("int"), integer),
		pairValue(symbol("bool"), boolean),
	})
	return list([]Expr{
		symbol("quote"),
		exports,
	})
}

func nativeRandomBytes(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("random/bytes requires one non-negative integer"), nil
	}
	err, length := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	if length < 0 {
		return expressionError("random/bytes requires one non-negative integer"), nil
	}
	err, value := nativeRandomRead(length)
	if err != nil {
		return err, nil
	}
	return nil, bytesValue(value)
}

func nativeRandomInteger(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("random/int requires one positive integer"), nil
	}
	err, bound := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	if bound <= 0 {
		return expressionError("random/int requires one positive integer"), nil
	}

	rangeSize := uint64(1) << 63
	limit := rangeSize - rangeSize%uint64(bound)
	for {
		err, value := nativeRandomRead(8)
		if err != nil {
			return err, nil
		}
		candidate := binary.BigEndian.Uint64([]byte(value)) >> 1
		if candidate < limit {
			return nil, integer(int64(candidate % uint64(bound)))
		}
	}
}

func nativeRandomBoolean(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 0 {
		return expressionError("random/bool requires no arguments"), nil
	}
	err, value := nativeRandomRead(1)
	if err != nil {
		return err, nil
	}
	return nil, boolean(value[0]&1 == 1)
}

func nativeRandomRead(length int64) (error, string) {
	if length > int64(^uint(0)>>1) {
		return expressionError("random/bytes length is too large"), ""
	}
	value := make([]byte, int(length))
	if _, err := rand.Read(value); err != nil {
		return expressionError("random source failed"), ""
	}
	return nil, string(value)
}
