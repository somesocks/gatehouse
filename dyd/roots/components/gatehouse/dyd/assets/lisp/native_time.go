package lisp

import "time"

const nativeTimeID = "native:time/v1"

func nativeTimeModule() Expr {
	now := Expr{
		kind:    exprBuiltin,
		help:    "Returns the current Unix time in milliseconds.",
		builtin: &builtin{call: pure(nativeTimeNow)},
	}
	exports := list([]Expr{
		pairValue(symbol("now", Span{}), now, Span{}),
	}, Span{})
	return list([]Expr{
		symbol("quote", Span{}),
		exports,
	}, Span{})
}

func nativeTimeNow(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 0 {
		return expressionError(span, "time/now requires no arguments"), Expr{}
	}
	return nil, integer(time.Now().UnixMilli(), Span{})
}
