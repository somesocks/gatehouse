package lisp

import (
	"net/url"
	"strconv"
	"strings"
)

// Expr is a sealed runtime value. Source locations are intentionally not retained.
type Expr interface {
	expr()
	String() string
}

type annotations struct {
	taint Taint
	help  string
}

// Taint controls how a value may flow through the runtime.
type Taint uint8

const (
	TaintNone Taint = iota
	TaintSensitive
	TaintSecret
)

type annotatedExpr struct {
	value Expr
	annotations
}

type booleanExpr struct{ value bool }
type integerExpr struct{ value int64 }
type stringExpr struct{ value string }
type bytesExpr struct{ value string }
type symbolExpr struct{ value string }
type nullExpr struct{}

type pair struct {
	first Expr
	rest  Expr
}

type closure struct {
	parameters    []string
	restParameter *string
	body          []Expr
	env           *environment
}

type builtin struct {
	special bool
	leaky   bool
	call    builtinCall
}

type callOutcome interface{ callOutcome() }

type callResult struct{ value Expr }

type callTailState struct {
	expression  Expr
	environment *environment
	taint       Taint
}

type errorValue struct {
	value Expr
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

func (*annotatedExpr) expr()   {}
func (*booleanExpr) expr()     {}
func (*integerExpr) expr()     {}
func (*stringExpr) expr()      {}
func (*bytesExpr) expr()       {}
func (*symbolExpr) expr()      {}
func (*nullExpr) expr()        {}
func (*pair) expr()            {}
func (*closure) expr()         {}
func (*builtin) expr()         {}
func (*errorValue) expr()      {}
func (*moduleReference) expr() {}

func (reference *moduleReference) String() string {
	if reference.fingerprint == "" {
		return "@" + reference.uri
	}
	if reference.scheme == moduleReferenceFingerprint && reference.uri == "" {
		return "@#" + reference.fingerprint
	}
	return "@" + reference.uri + "#" + reference.fingerprint
}

func (expr *annotatedExpr) String() string { return render(expr) }
func (expr *booleanExpr) String() string   { return render(expr) }
func (expr *integerExpr) String() string   { return render(expr) }
func (expr *stringExpr) String() string    { return render(expr) }
func (expr *bytesExpr) String() string     { return render(expr) }
func (expr *symbolExpr) String() string    { return render(expr) }
func (expr *nullExpr) String() string      { return render(expr) }
func (expr *pair) String() string          { return render(expr) }
func (expr *closure) String() string       { return render(expr) }
func (expr *builtin) String() string       { return render(expr) }
func (expr *errorValue) String() string    { return render(expr) }

var (
	trueValue  Expr = &booleanExpr{value: true}
	falseValue Expr = &booleanExpr{value: false}
	nullValue  Expr = &nullExpr{}
)

type builtinCall func(*evaluator, *environment, []Expr) (error, callOutcome)
type valueBuiltinCall func(*evaluator, *environment, []Expr) (error, Expr)
type pureBuiltinCall func(*evaluator, []Expr) (error, Expr)

func (callResult) callOutcome()    {}
func (callTailState) callOutcome() {}

func valueCall(call valueBuiltinCall) builtinCall {
	return func(evaluator *evaluator, environment *environment, arguments []Expr) (error, callOutcome) {
		err, result := call(evaluator, environment, arguments)
		if err != nil {
			return err, nil
		}
		return nil, callResult{value: result}
	}
}

func pure(call pureBuiltinCall) builtinCall {
	return valueCall(func(evaluator *evaluator, _ *environment, arguments []Expr) (error, Expr) {
		return call(evaluator, arguments)
	})
}

func boolean(value bool) Expr {
	if value {
		return trueValue
	}
	return falseValue
}

func integer(value int64) Expr      { return &integerExpr{value: value} }
func stringValue(value string) Expr { return &stringExpr{value: value} }
func bytesValue(value string) Expr  { return &bytesExpr{value: value} }
func symbol(value string) Expr      { return &symbolExpr{value: value} }
func null() Expr                    { return nullValue }

func moduleReferenceValue(value string) (error, Expr) {
	parsed, err := url.Parse(value)
	if err != nil {
		return expressionError("invalid module reference URI"), nil
	}
	hasFragment := strings.Contains(value, "#")
	fingerprint := parsed.Fragment
	parsed.Fragment = ""
	parsed.RawFragment = ""
	reference := moduleReference{uri: parsed.String()}
	switch parsed.Scheme {
	case "":
		if !hasFragment || !isSHA256Fingerprint(fingerprint) {
			return expressionError("module reference without a URI scheme requires a sha256 fingerprint"), nil
		}
		reference.scheme = moduleReferenceFingerprint
		reference.fingerprint = fingerprint
	case "file":
		if parsed.Opaque == "" && parsed.Path == "" {
			return expressionError("file module reference requires a location"), nil
		}
		if hasFragment && !isSHA256Fingerprint(fingerprint) {
			return expressionError("file module reference has an invalid sha256 fingerprint"), nil
		}
		reference.scheme = moduleReferenceFile
		reference.fingerprint = fingerprint
	case "native":
		if reference.uri == "native:" {
			return expressionError("native module reference requires a name"), nil
		}
		if hasFragment {
			return expressionError("native module reference must not have a sha256 fingerprint"), nil
		}
		reference.scheme = moduleReferenceNative
	default:
		return expressionError("unsupported module reference URI scheme %q", parsed.Scheme), nil
	}
	return nil, &reference
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

func unwrap(expr Expr) (Expr, annotations) {
	for {
		annotated, ok := expr.(*annotatedExpr)
		if !ok {
			return expr, annotations{}
		}
		return annotated.value, annotated.annotations
	}
}

func TaintOf(expr Expr) Taint {
	_, annotations := unwrap(expr)
	return annotations.taint
}

func helpOf(expr Expr) string {
	_, annotations := unwrap(expr)
	return annotations.help
}

func joinTaint(left, right Taint) Taint {
	if left > right {
		return left
	}
	return right
}

func withTaint(expr Expr, taint Taint) Expr {
	if taint == TaintNone {
		return expr
	}
	base, annotations := unwrap(expr)
	if annotations.taint >= taint {
		return expr
	}
	annotations.taint = taint
	return &annotatedExpr{value: base, annotations: annotations}
}

func withHelp(expr Expr, help string) Expr {
	base, annotations := unwrap(expr)
	annotations.help = help
	if annotations.taint == TaintNone && annotations.help == "" {
		return base
	}
	return &annotatedExpr{value: base, annotations: annotations}
}

func appendHelp(expr Expr, help string) Expr {
	if help == "" {
		return expr
	}
	current := helpOf(expr)
	if current != "" {
		help = current + "\n" + help
	}
	return withHelp(expr, help)
}

func pairValue(first Expr, rest Expr) Expr {
	result := Expr(&pair{first: first, rest: rest})
	if taint := joinTaint(TaintOf(first), TaintOf(rest)); taint != TaintNone {
		return withTaint(result, taint)
	}
	return result
}

func list(values []Expr) Expr {
	result := null()
	for index := len(values) - 1; index >= 0; index-- {
		result = pairValue(values[index], result)
	}
	return result
}

func render(expr Expr) string {
	base, annotations := unwrap(expr)
	switch annotations.taint {
	case TaintSensitive:
		return "#<sensitive>"
	case TaintSecret:
		return "#<secret>"
	}
	switch value := base.(type) {
	case *booleanExpr:
		if value.value {
			return "#t"
		}
		return "#f"
	case *integerExpr:
		return strconv.FormatInt(value.value, 10)
	case *stringExpr:
		return strconv.Quote(value.value)
	case *bytesExpr:
		return "#<bytes " + strconv.Itoa(len(value.value)) + ">"
	case *errorValue:
		return "#<error>"
	case *symbolExpr:
		return value.value
	case *nullExpr:
		return "null"
	case *pair:
		return formatPair(value)
	case *closure:
		return "#<closure>"
	case *builtin:
		return "#<builtin>"
	case *moduleReference:
		return value.String()
	default:
		return "#<invalid>"
	}
}

func formatPair(value *pair) string {
	var builder strings.Builder
	builder.WriteByte('(')
	for {
		builder.WriteString(value.first.String())
		rest, _ := unwrap(value.rest)
		switch rest := rest.(type) {
		case *nullExpr:
			builder.WriteByte(')')
			return builder.String()
		case *pair:
			builder.WriteByte(' ')
			value = rest
		default:
			builder.WriteString(" . ")
			builder.WriteString(value.rest.String())
			builder.WriteByte(')')
			return builder.String()
		}
	}
}

func isSymbol(expr Expr, value string) bool {
	base, _ := unwrap(expr)
	symbol, ok := base.(*symbolExpr)
	return ok && symbol.value == value
}

func isNullValue(expr Expr) bool {
	base, _ := unwrap(expr)
	_, ok := base.(*nullExpr)
	return ok
}

func expressions(expr Expr) (error, []Expr) {
	var values []Expr
	for {
		base, _ := unwrap(expr)
		pair, ok := base.(*pair)
		if !ok {
			if _, ok := base.(*nullExpr); ok {
				return nil, values
			}
			return expressionError("expected a proper list"), nil
		}
		values = append(values, pair.first)
		expr = pair.rest
	}
}

func equal(left Expr, right Expr) bool {
	left, _ = unwrap(left)
	right, _ = unwrap(right)
	switch left := left.(type) {
	case *booleanExpr:
		right, ok := right.(*booleanExpr)
		return ok && left.value == right.value
	case *integerExpr:
		right, ok := right.(*integerExpr)
		return ok && left.value == right.value
	case *stringExpr:
		right, ok := right.(*stringExpr)
		return ok && left.value == right.value
	case *bytesExpr:
		right, ok := right.(*bytesExpr)
		return ok && left.value == right.value
	case *errorValue:
		right, ok := right.(*errorValue)
		return ok && left == right
	case *symbolExpr:
		right, ok := right.(*symbolExpr)
		return ok && left.value == right.value
	case *nullExpr:
		_, ok := right.(*nullExpr)
		return ok
	case *pair:
		right, ok := right.(*pair)
		return ok && equal(left.first, right.first) && equal(left.rest, right.rest)
	case *closure:
		right, ok := right.(*closure)
		return ok && left == right
	case *builtin:
		right, ok := right.(*builtin)
		return ok && left == right
	case *moduleReference:
		right, ok := right.(*moduleReference)
		return ok && *left == *right
	default:
		return false
	}
}
