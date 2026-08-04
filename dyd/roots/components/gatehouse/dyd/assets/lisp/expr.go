package lisp

import (
	"net/url"
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
	exprModuleReference
)

type Expr struct {
	kind   exprKind
	span   Span
	secret bool
	help   string

	boolean bool
	integer int64
	text    string

	pair      *pair
	closure   *closure
	builtin   *builtin
	reference *moduleReference
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
	special bool
	leaky   bool
	call    builtinCall
}

type moduleReferenceScheme uint8

const (
	moduleReferenceFile moduleReferenceScheme = iota
	moduleReferenceFingerprint
	moduleReferenceNative
)

type moduleReference struct {
	scheme      moduleReferenceScheme
	uri         string
	fingerprint string
}

func (reference moduleReference) String() string {
	if reference.fingerprint == "" {
		return "@" + reference.uri
	}
	if reference.scheme == moduleReferenceFingerprint && reference.uri == "" {
		return "@#" + reference.fingerprint
	}
	return "@" + reference.uri + "#" + reference.fingerprint
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

func moduleReferenceValue(value string, span Span) (error, Expr) {
	parsed, err := url.Parse(value)
	if err != nil {
		return expressionError(span, "invalid module reference URI"), Expr{}
	}
	hasFragment := strings.Contains(value, "#")
	fingerprint := parsed.Fragment
	parsed.Fragment = ""
	parsed.RawFragment = ""
	reference := moduleReference{uri: parsed.String()}
	switch parsed.Scheme {
	case "":
		if !hasFragment || !isSHA256Fingerprint(fingerprint) {
			return expressionError(span, "module reference without a URI scheme requires a sha256 fingerprint"), Expr{}
		}
		reference.scheme = moduleReferenceFingerprint
		reference.fingerprint = fingerprint
	case "file":
		if parsed.Opaque == "" && parsed.Path == "" {
			return expressionError(span, "file module reference requires a location"), Expr{}
		}
		if hasFragment && !isSHA256Fingerprint(fingerprint) {
			return expressionError(span, "file module reference has an invalid sha256 fingerprint"), Expr{}
		}
		reference.scheme = moduleReferenceFile
		reference.fingerprint = fingerprint
	case "native":
		if reference.uri == "native:" {
			return expressionError(span, "native module reference requires a name"), Expr{}
		}
		if hasFragment {
			return expressionError(span, "native module reference must not have a sha256 fingerprint"), Expr{}
		}
		reference.scheme = moduleReferenceNative
	default:
		return expressionError(span, "unsupported module reference URI scheme %q", parsed.Scheme), Expr{}
	}
	return nil, Expr{kind: exprModuleReference, span: span, reference: &reference}
}

func isSHA256Fingerprint(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if !('0' <= character && character <= '9') && !('a' <= character && character <= 'f') {
			return false
		}
	}
	return true
}

func pairValue(first Expr, rest Expr, span Span) Expr {
	return Expr{kind: exprPair, pair: &pair{first: first, rest: rest}, span: span, secret: first.secret || rest.secret}
}

func list(values []Expr, span Span) Expr {
	result := null(span)
	for index := len(values) - 1; index >= 0; index-- {
		result = pairValue(values[index], result, span)
	}
	return result
}

func (expr Expr) String() string {
	if expr.secret {
		return "#<secret>"
	}
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
	case exprModuleReference:
		return expr.reference.String()
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
	case exprClosure:
		return left.closure == right.closure
	case exprBuiltin:
		return left.builtin == right.builtin
	case exprModuleReference:
		return *left.reference == *right.reference
	default:
		return false
	}
}
