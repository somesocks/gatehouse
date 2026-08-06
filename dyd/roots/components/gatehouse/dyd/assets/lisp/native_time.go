package lisp

import "time"

const nativeTimeID = "native:time/v1"

var nativeTimeDocumentation = doc(
	"(time/now) -> Integer",
	"Returns the current Unix time in milliseconds.",
	"(import (time @native:time/v1) (int? (time/now)))",
	"#t",
)

func nativeTimeModule() Expr {
	now := withHelp(&builtin{call: pure(nativeTimeNow)}, nativeTimeDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("now"), now),
	})
	return list([]Expr{
		symbol("quote"),
		exports,
	})
}

func nativeTimeNow(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 0 {
		return expressionError("time/now requires no arguments"), nil
	}
	return nil, integer(time.Now().UnixMilli())
}
