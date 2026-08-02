package lisp

import (
	"strconv"
	"strings"
)

type Span struct {
	Start int
	End   int
}

type exprKind uint8

const (
	exprInvalid exprKind = iota
	exprBoolean
	exprInteger
	exprString
	exprSymbol
	exprNull
	exprPair
	exprClosure
	exprBuiltin
)

type Expr struct {
	kind exprKind
	span Span

	boolean bool
	integer int64
	text    string

	pair    *pair
	closure *closure
	builtin *builtin
}

type pair struct {
	first Expr
	rest  Expr
}

type closure struct {
	parameters []string
	body       Expr
	env        *environment
}

type builtin struct {
	helpText     string
	helpKeywords []string
	call         builtinCall
}

type builtinCall func(*evaluator, *environment, []Expr, Span) (error, Expr)

type pureBuiltinCall func(*evaluator, []Expr, Span) (error, Expr)

func pure(call pureBuiltinCall) builtinCall {
	return func(evaluator *evaluator, _ *environment, arguments []Expr, span Span) (error, Expr) {
		return call(evaluator, arguments, span)
	}
}

func boolean(value bool, span Span) Expr {
	return Expr{kind: exprBoolean, boolean: value, span: span}
}

func integer(value int64, span Span) Expr {
	return Expr{kind: exprInteger, integer: value, span: span}
}

func stringValue(value string, span Span) Expr {
	return Expr{kind: exprString, text: value, span: span}
}

func symbol(value string, span Span) Expr {
	return Expr{kind: exprSymbol, text: value, span: span}
}

func null(span Span) Expr {
	return Expr{kind: exprNull, span: span}
}

func pairValue(first Expr, rest Expr, span Span) Expr {
	return Expr{kind: exprPair, pair: &pair{first: first, rest: rest}, span: span}
}

func list(values []Expr, span Span) Expr {
	result := null(span)
	for index := len(values) - 1; index >= 0; index-- {
		result = pairValue(values[index], result, span)
	}
	return result
}

func (expr Expr) String() string {
	switch expr.kind {
	case exprBoolean:
		if expr.boolean {
			return "#t"
		}
		return "#f"
	case exprInteger:
		return strconv.FormatInt(expr.integer, 10)
	case exprString:
		return strconv.Quote(expr.text)
	case exprSymbol:
		return expr.text
	case exprNull:
		return "null"
	case exprPair:
		return formatPair(expr)
	case exprClosure:
		return "#<closure>"
	case exprBuiltin:
		return "#<builtin>"
	default:
		return "#<invalid>"
	}
}

func formatPair(expr Expr) string {
	var builder strings.Builder
	builder.WriteByte('(')
	for {
		builder.WriteString(expr.pair.first.String())
		switch expr.pair.rest.kind {
		case exprNull:
			builder.WriteByte(')')
			return builder.String()
		case exprPair:
			builder.WriteByte(' ')
			expr = expr.pair.rest
		default:
			builder.WriteString(" . ")
			builder.WriteString(expr.pair.rest.String())
			builder.WriteByte(')')
			return builder.String()
		}
	}
}

func isSymbol(expr Expr, value string) bool {
	return expr.kind == exprSymbol && expr.text == value
}

func expressions(expr Expr) (error, []Expr) {
	var values []Expr
	for expr.kind == exprPair {
		values = append(values, expr.pair.first)
		expr = expr.pair.rest
	}
	if expr.kind != exprNull {
		return expressionError(expr.span, "expected a proper list"), nil
	}
	return nil, values
}

func equal(left Expr, right Expr) bool {
	if left.kind != right.kind {
		return false
	}
	switch left.kind {
	case exprBoolean:
		return left.boolean == right.boolean
	case exprInteger:
		return left.integer == right.integer
	case exprString, exprSymbol:
		return left.text == right.text
	case exprNull:
		return true
	case exprPair:
		return equal(left.pair.first, right.pair.first) && equal(left.pair.rest, right.pair.rest)
	default:
		return left == right
	}
}
